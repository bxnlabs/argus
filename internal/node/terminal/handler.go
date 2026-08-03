package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	agentsession "github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/shared"
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
	// exitPaneMode cancels any tmux pane mode (copy-mode, choose-tree, ...)
	// before compose text is injected. nil for the raw-shell route, which has
	// no tmux session.
	exitPaneMode func() error
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
					// Submit defaults to true for backwards compatibility
					// (old clients don't send the field).
					submit := msg.Submit == nil || *msg.Submit
					if err := handleTextMessage(s.ptmx, time.Sleep, s.exitPaneMode, msg.Text, submit); err != nil {
						return
					}
				}
			}
		}
	}
}

// submitReturnDelay is how long to wait after writing a compose paste block
// before writing the submit Return. The Return must land in a SEPARATE PTY read
// from the paste so a bracketed-paste-aware TUI (e.g. Claude Code) finishes
// applying the pasted text to its input before it processes the Return.
//
// Otherwise the Return coalesces into the same PTY read — and the same parser
// feed — as the closing marker: the TUI emits the paste and the Enter in one
// batch, applies the paste asynchronously, and runs the Enter's submit handler
// in the same tick against stale (empty) input, so nothing submits. That is the
// intermittent "Enter didn't send" bug — intermittent because network/load
// jitter only occasionally splits the Return into its own read by luck.
//
// Measured through Argus's PTY -> tmux -> pane path: an immediate Return is
// always coalesced with the close marker, while any separating delay reliably
// splits it into its own read. The margin here covers the extra tmux hops plus
// node load. Claude Code's own programmatic submit uses the same
// paste-then-delayed-Return shape (it delays 10ms for a single, direct PTY hop).
const submitReturnDelay = 40 * time.Millisecond

// composePasteBlock wraps compose text in bracketed-paste markers
// (ESC[200~ ... ESC[201~) so a TUI that enabled bracketed paste mode (e.g.
// Claude Code) receives multiline input as a SINGLE paste event — newlines
// inserted literally — instead of one Enter keypress per line, which would
// submit each line as its own prompt (BXN-110). tmux forwards the markers to
// such panes and strips them for panes whose app did not enable the mode (e.g.
// a plain shell), so embedded line breaks remain plain CRs there and behavior
// is unchanged.
//
// Line endings are normalized to CR (the PTY's Enter). The block NEVER includes
// the submit Return — injectCompose delivers that as a separate, delayed write.
func composePasteBlock(text string) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\r")
	text = strings.ReplaceAll(text, "\n", "\r")

	var b strings.Builder
	b.WriteString("\x1b[200~")
	b.WriteString(text)
	b.WriteString("\x1b[201~")
	return []byte(b.String())
}

// injectCompose writes a compose "text" message into the PTY: the bracketed
// paste block, then — when submit is set — a lone Return written as a SEPARATE
// call after submitReturnDelay. The separation (not the marker alone) is what
// makes the Return register as a distinct submit instead of being swallowed
// into the paste; see submitReturnDelay. The sleep is injectable for testing.
func injectCompose(w io.Writer, sleep func(time.Duration), text string, submit bool) error {
	if _, err := w.Write(composePasteBlock(text)); err != nil {
		return err
	}
	if submit {
		sleep(submitReturnDelay)
		if _, err := w.Write([]byte("\r")); err != nil {
			return err
		}
	}
	return nil
}

// exitPaneModeTimeout bounds the tmux call so a wedged tmux cannot stall the
// WebSocket read loop, which is what calls this.
const exitPaneModeTimeout = 2 * time.Second

// newExitPaneMode returns a func that cancels any mode the session's active
// pane is in, or nil when there is no tmux session (the raw-shell route).
//
// This matters because the PTY we write compose text to is a tmux *client*: a
// pane sitting in copy-mode routes those bytes through the copy-mode key table
// instead of to the pane's process, so the paste never reaches the agent and
// its characters fire copy-mode bindings instead ('q' cancels, '/' opens
// search, digits become repeat counts). Cancelling first also snaps the pane
// back to live output, which is the feedback the user wants after sending.
func newExitPaneMode(sessionName string) func() error {
	if sessionName == "" {
		return nil
	}
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), exitPaneModeTimeout)
		defer cancel()
		cmd, err := shared.TmuxCommandContext(ctx, "send-keys", "-t", sessionName, "-X", "cancel")
		if err != nil {
			return err
		}
		// Errors here are expected and benign: tmux fails this when the pane is
		// not in a mode, which is the common case.
		return cmd.Run()
	}
}

// handleTextMessage injects a compose "text" message, cancelling any tmux pane
// mode first. A failing cancel never blocks the send — see newExitPaneMode.
func handleTextMessage(w io.Writer, sleep func(time.Duration), exitMode func() error, text string, submit bool) error {
	if exitMode != nil {
		_ = exitMode()
	}
	return injectCompose(w, sleep, text, submit)
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

// readHello reads the mandatory opening "hello" frame and returns the terminal
// dimensions it carries and ok=true. Returns ok=false on any failure:
// timeout, read error, non-text frame, bad JSON, or wrong message type.
// Callers must close the WebSocket and not start the PTY when ok=false.
//
// The client sends hello synchronously in ws.onopen before any PTY data is
// needed, so this normally returns in milliseconds. Requiring a well-formed
// hello before starting the PTY means tmux always attaches at the correct
// size — the root cause of the A→B→A garbled-output bug (BXN-54).
//
// Note: Gorilla permanently marks a connection corrupt after a read deadline
// fires, so there is no recovery path — only close.
func readHello(ws *websocket.Conn) (cols, rows uint16, ok bool) {
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	defer ws.SetReadDeadline(time.Time{})

	msgType, data, err := ws.ReadMessage()
	if err != nil || msgType != websocket.TextMessage {
		return 0, 0, false
	}
	var msg controlMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.Type != "hello" {
		return 0, 0, false
	}
	// Both dimensions must be valid together (mirrors readWS hello handling).
	// If only one is valid the hello is malformed; fall back to defaults but
	// still treat the connection as usable.
	cols, rows = 80, 24
	if msg.Cols >= 1 && msg.Cols <= 500 && msg.Rows >= 1 && msg.Rows <= 200 {
		cols, rows = uint16(msg.Cols), uint16(msg.Rows)
	}
	return cols, rows, true
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

	// Block until the client sends its initial hello with actual terminal
	// dimensions. This guarantees tmux always attaches at the correct size —
	// no spurious 80×24 phase, no garbled scrollback.
	// Fail closed on any handshake error: the connection is either corrupt
	// (timeout) or the client sent an unexpected first frame (protocol error).
	initCols, initRows, ok := readHello(ws)
	if !ok {
		ws.Close()
		return
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: initCols, Rows: initRows})
	if err != nil {
		log.Printf("pty start: %v", err)
		ws.WriteJSON(controlMessage{Version: 1, Type: "error", Message: err.Error()})
		ws.Close()
		return
	}

	s := &session{
		ws:           ws,
		ptmx:         ptmx,
		cmd:          cmd,
		done:         make(chan struct{}),
		exitPaneMode: newExitPaneMode(sessionName),
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

// SessionReadyFunc is an optional callback invoked after EnsureSession
// succeeds. It receives (sessionID, tmuxName) so callers can start
// ancillary work such as ensuring a status watcher is running.
type SessionReadyFunc func(sessionID, tmuxName string)

// HandleSessionWebSocket returns an http.HandlerFunc that resolves
// the {id} path value to a tmux session (creating it if needed) and
// attaches via WebSocket. An optional onReady callback is invoked
// after the session is ensured.
func HandleSessionWebSocket(sf sessionFactory, onReady SessionReadyFunc) http.HandlerFunc {
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
		if onReady != nil {
			onReady(id, tmuxName)
		}
		cmd, err := shared.TmuxCommand("attach-session", "-t", tmuxName)
		if err != nil {
			log.Printf("build tmux command for %s: %v", id, err)
			http.Error(w, "session not available", http.StatusInternalServerError)
			return
		}
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
