package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bxnlabs/argus/internal/shared"
	"github.com/spf13/cobra"
)

// submitReturnDelay mirrors the server's compose submit delay: the Return must
// land in a separate write from the paste so the agent TUI registers it as a
// submit instead of coalescing it into the pasted text (see BXN-110 and
// internal/node/terminal/handler.go).
const submitReturnDelay = 40 * time.Millisecond

// resolveSendInput picks the single input source for `session send`: the inline
// text arg, the --file path, or stdin. Exactly one may be used. When no arg and
// no file are given, stdin is read — unless stdin is a terminal (stdinIsTTY),
// which is treated as "no input" and returns a guiding error.
func resolveSendInput(text string, hasText bool, filePath string, stdin io.Reader, stdinIsTTY bool) ([]byte, error) {
	if hasText && filePath != "" {
		return nil, fmt.Errorf("provide input as either text or --file, not both")
	}
	if hasText {
		return []byte(text), nil
	}
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read input file: %w", err)
		}
		return data, nil
	}
	if stdinIsTTY {
		return nil, fmt.Errorf("no input: pass text, --file <path>, or pipe to stdin")
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return data, nil
}

// loadBufferArgs loads a tmux paste buffer from the command's stdin.
func loadBufferArgs(bufName string) []string {
	return []string{"load-buffer", "-b", bufName, "-"}
}

// pasteBufferArgs pastes bufName into the session's pane using bracketed paste
// (-p) and deletes the buffer afterward (-d).
func pasteBufferArgs(bufName, tmuxName string) []string {
	return []string{"paste-buffer", "-d", "-p", "-b", bufName, "-t", tmuxName}
}

// sendKeysArgs sends the given tmux key names to the session's pane.
func sendKeysArgs(tmuxName string, keys []string) []string {
	return append([]string{"send-keys", "-t", tmuxName}, keys...)
}

// runTmux runs a tmux command through the dedicated socket, surfacing tmux's own
// error text (e.g. "can't find session") on failure.
func runTmux(args []string) error {
	c, err := shared.TmuxCommand(args...)
	if err != nil {
		return fmt.Errorf("build tmux command: %w", err)
	}
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}

// runTmuxStdin is runTmux with data fed to the command's stdin.
func runTmuxStdin(args []string, data []byte) error {
	c, err := shared.TmuxCommand(args...)
	if err != nil {
		return fmt.Errorf("build tmux command: %w", err)
	}
	c.Stdin = bytes.NewReader(data)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux %s: %s", args[0], strings.TrimSpace(string(out)))
	}
	return nil
}

func newSendCmd() *cobra.Command {
	var (
		file     string
		enter    bool
		keysMode bool
	)
	cmd := &cobra.Command{
		Use:   "send <name-or-id> [text]",
		Short: "Send input or keys to a session's tmux",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			query := args[0]
			hasText := len(args) == 2
			text := ""
			if hasText {
				text = args[1]
			}

			fi, _ := os.Stdin.Stat()
			stdinIsTTY := fi != nil && (fi.Mode()&os.ModeCharDevice) != 0
			input, err := resolveSendInput(text, hasText, file, os.Stdin, stdinIsTTY)
			if err != nil {
				return err
			}

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}
			s, err := fetchAndResolve(c, query)
			if err != nil {
				return err
			}

			// Revive the tmux session if it died, mirroring `session attach`:
			// GET /sessions/{id} runs EnsureSession on the node.
			if _, err := c.get("/sessions/" + s.ID); err != nil {
				return fmt.Errorf("ensure session: %w", err)
			}

			if keysMode {
				keys := strings.Fields(string(input))
				if len(keys) == 0 {
					return fmt.Errorf("no keys to send")
				}
				return runTmux(sendKeysArgs(s.TmuxName, keys))
			}

			// Bracketed-paste the input via a per-invocation tmux buffer loaded
			// from stdin. A unique name avoids cross-wiring concurrent sends that
			// share the node's single tmux server; the deferred delete-buffer
			// cleans up input left behind if paste-buffer fails before its own
			// -d removes the buffer.
			bufName := fmt.Sprintf("argus-send-%d-%d", os.Getpid(), time.Now().UnixNano())
			if err := runTmuxStdin(loadBufferArgs(bufName), input); err != nil {
				return err
			}
			defer func() { _ = runTmux([]string{"delete-buffer", "-b", bufName}) }()
			if err := runTmux(pasteBufferArgs(bufName, s.TmuxName)); err != nil {
				return err
			}
			if enter {
				// Deliver Return as a SEPARATE, delayed write so it registers as a
				// submit instead of coalescing into the paste (see BXN-110).
				time.Sleep(submitReturnDelay)
				return runTmux(sendKeysArgs(s.TmuxName, []string{"Enter"}))
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&file, "file", "f", "", "Read input from this file instead of an arg or stdin")
	cmd.Flags().BoolVar(&enter, "enter", false, "Press Enter after sending (submits the input)")
	cmd.Flags().BoolVar(&keysMode, "keys", false, "Interpret input as tmux key names (e.g. Escape, C-c, Enter)")
	return cmd
}
