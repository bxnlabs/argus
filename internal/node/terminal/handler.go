package terminal

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	agentsession "github.com/bxnlabs/argus/internal/node/session"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// session encapsulates a single WebSocket-to-PTY bridge.
type session struct {
	ws        *websocket.Conn
	ptmx      *os.File
	cmd       *exec.Cmd
	wsMu      sync.Mutex
	done      chan struct{}
	closeOnce sync.Once
}

func (s *session) close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.ws.Close()

		if s.cmd.Process != nil {
			// SIGHUP detaches tmux attach cleanly without killing the session.
			_ = s.cmd.Process.Signal(syscall.SIGHUP)

			waitDone := make(chan struct{})
			go func() {
				_ = s.cmd.Wait()
				close(waitDone)
			}()

			select {
			case <-waitDone:
			case <-time.After(5 * time.Second):
				_ = s.cmd.Process.Kill()
			}
		}

		if s.ptmx != nil {
			s.ptmx.Close()
		}
	})
}

func (s *session) writeJSON(msg controlMessage) error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return s.ws.WriteJSON(msg)
}

func (s *session) writeBinary(data []byte) error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return s.ws.WriteMessage(websocket.BinaryMessage, data)
}

// readPTY pumps data from PTY to WebSocket as binary frames.
func (s *session) readPTY() {
	defer s.close()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-s.done:
			return
		default:
		}

		n, err := s.ptmx.Read(buf)
		if err != nil {
			// PTY closed (tmux session ended or shell exited).
			// Send exit notification before closing the WebSocket.
			_ = s.writeJSON(controlMessage{Version: 1, Type: "exit"})
			return
		}

		if err := s.writeBinary(buf[:n]); err != nil {
			return
		}
	}
}

// readWS pumps data from WebSocket to PTY. Binary frames are raw input,
// text frames are JSON control messages (hello, resize).
func (s *session) readWS() {
	defer s.close()

	s.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	s.ws.SetPongHandler(func(string) error {
		s.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		msgType, data, err := s.ws.ReadMessage()
		if err != nil {
			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			if _, err := s.ptmx.Write(data); err != nil {
				return
			}

		case websocket.TextMessage:
			var msg controlMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("bad control message: %v", err)
				continue
			}

			switch msg.Type {
			case "hello":
				if msg.Cols > 0 && msg.Rows > 0 {
					s.resize(msg.Cols, msg.Rows)
				}
			case "resize":
				s.resize(msg.Cols, msg.Rows)
			case "text":
				if msg.Text != "" {
					// Normalize line endings: CRLF -> CR, then LF -> CR
					// (PTY expects CR for Enter / line submission)
					text := strings.ReplaceAll(msg.Text, "\r\n", "\r")
					text = strings.ReplaceAll(text, "\n", "\r")

					if _, err := s.ptmx.Write([]byte(text)); err != nil {
						return
					}

					// Submit defaults to true for backwards compatibility
					// (old clients don't send the field).
					// Write \r as a separate PTY write so the TUI sees
					// Enter as a distinct input event (not part of a
					// pasted text chunk). The 1ms sleep ensures the
					// kernel delivers them as separate read()s.
					submit := msg.Submit == nil || *msg.Submit
					if submit {
						time.Sleep(time.Millisecond)
						if _, err := s.ptmx.Write([]byte("\r")); err != nil {
							return
						}
					}
				}
			}
		}
	}
}

// ping sends periodic WebSocket pings to detect dead connections.
func (s *session) ping() {
	defer s.close()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.wsMu.Lock()
			s.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
			err := s.ws.WriteMessage(websocket.PingMessage, nil)
			s.wsMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

func (s *session) resize(cols, rows int) {
	if cols < 1 || cols > 500 || rows < 1 || rows > 200 {
		return
	}
	_ = pty.Setsize(s.ptmx, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
}

// handleConnection is the shared WebSocket-to-PTY bridge logic.
func handleConnection(w http.ResponseWriter, r *http.Request, cmd *exec.Cmd, sessionName string) {
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
	)

	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 80, Rows: 24})
	if err != nil {
		log.Printf("pty start: %v", err)
		ws.WriteJSON(controlMessage{Version: 1, Type: "error", Message: err.Error()})
		ws.Close()
		return
	}

	s := &session{
		ws:   ws,
		ptmx: ptmx,
		cmd:  cmd,
		done: make(chan struct{}),
	}

	if err := s.writeJSON(controlMessage{
		Version: 1,
		Type:    "connected",
		Session: sessionName,
	}); err != nil {
		s.close()
		return
	}

	go s.readPTY()
	go s.ping()
	s.readWS() // blocks until disconnect
}

// sessionFactory resolves a session ID to a live tmux session name,
// recreating the tmux session if it was killed.
type sessionFactory interface {
	EnsureSession(id string) (tmuxName string, err error)
}

// HandleSessionWebSocket returns an http.HandlerFunc that resolves
// the {id} path value to a tmux session (creating it if needed) and
// attaches via WebSocket.
func HandleSessionWebSocket(sf sessionFactory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing session id", http.StatusBadRequest)
			return
		}
		tmuxName, err := sf.EnsureSession(id)
		if err != nil {
			if errors.Is(err, agentsession.ErrNotFound) {
				http.Error(w, "session not found", http.StatusNotFound)
			} else {
				log.Printf("EnsureSession %s: %v", id, err)
				http.Error(w, "session not available", http.StatusInternalServerError)
			}
			return
		}
		cmd := exec.Command("tmux", "attach-session", "-t", tmuxName)
		handleConnection(w, r, cmd, tmuxName)
	}
}

// HandleTerminalWebSocket spawns a raw interactive shell.
// Route: /ws/terminal
func HandleTerminalWebSocket(w http.ResponseWriter, r *http.Request) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell)
	// Explicitly start in the user's home directory so the shell has a
	// valid CWD regardless of where the agent process is running.
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	handleConnection(w, r, cmd, "")
}
