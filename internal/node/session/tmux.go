package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bxnlabs/argus/internal/shared"
)

// HasSession checks if a tmux session exists on Argus's dedicated server.
func HasSession(name string) bool {
	cmd, err := shared.TmuxCommand("has-session", "-t", name)
	if err != nil {
		// A build error (e.g. misconfigured ARGUS_HOME) is treated as "not found".
		return false
	}
	return cmd.Run() == nil
}

// NewSession creates a new tmux session running the given command on Argus's
// dedicated server. If command is empty, starts a default shell. The dedicated
// server's directory and config are bootstrapped (fatally) at node startup
// (see shared.EnsureTmuxStateDir / SeedTmuxConfig), so the config is guaranteed
// present; the first session create boots the server with it via -f.
func NewSession(name, cwd, command string) error {
	confPath, err := shared.TmuxConfigPath()
	if err != nil {
		return fmt.Errorf("build tmux command: %w", err)
	}
	args := []string{"-f", confPath, "new-session", "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if command != "" {
		args = append(args, command)
	}

	cmd, err := shared.TmuxCommand(args...)
	if err != nil {
		return fmt.Errorf("build tmux command: %w", err)
	}
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, string(out))
	}
	hideStatusBar(name)
	return nil
}

// hideStatusBar turns off the tmux status bar for a single session. The web UI
// renders the session's identity — ID, profile, branch, directory — in its own
// status bar, so tmux's would only repeat it at the cost of a row in every
// pane. Setting it per session (rather than globally in the seeded tmux.conf)
// keeps it out of reach of a user-edited config, which Argus never overwrites,
// and applies to installs seeded before the bar went away.
//
// Best-effort: a session that renders one row too few is not worth failing a
// create over, so a failure is logged and the session still comes up. The
// option is set while the session is still detached, so no client has rendered
// the bar yet and there is nothing to flash.
func hideStatusBar(name string) {
	cmd, err := shared.TmuxCommand("set-option", "-t", name, "status", "off")
	if err != nil {
		log.Printf("tmux set-option status off: %v", err)
		return
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("tmux set-option status off: %v: %s", err, out)
	}
}

// KillSession kills a tmux session.
func KillSession(name string) error {
	cmd, err := shared.TmuxCommand("kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("build tmux command: %w", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, string(out))
	}
	return nil
}

// CapturePane captures the visible pane content of a tmux session.
func CapturePane(name string) (string, error) {
	return CapturePaneContext(context.Background(), name)
}

// CapturePaneContext captures pane content with context for cancellation/timeout.
func CapturePaneContext(ctx context.Context, name string) (string, error) {
	cmd, err := shared.TmuxCommandContext(ctx, "capture-pane", "-t", name, "-p", "-J")
	if err != nil {
		return "", fmt.Errorf("build tmux command: %w", err)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GetPaneCwd returns the current working directory of a tmux pane.
func GetPaneCwd(name string) (string, error) {
	cmd, err := shared.TmuxCommand("display-message", "-t", name, "-p", "#{pane_current_path}")
	if err != nil {
		return "", fmt.Errorf("build tmux command: %w", err)
	}
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// PaneDimensions holds the width and height of a tmux pane.
type PaneDimensions struct {
	Width  int
	Height int
}

// parsePaneDimensions parses a "WxH" string into width and height integers.
func parsePaneDimensions(s string) (width, height int, ok bool) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return w, h, true
}

// GetPaneDimensionsContext returns the dimensions of the named tmux pane.
func GetPaneDimensionsContext(ctx context.Context, name string) (PaneDimensions, error) {
	cmd, err := shared.TmuxCommandContext(ctx, "display-message", "-t", name, "-p", "#{pane_width}x#{pane_height}")
	if err != nil {
		return PaneDimensions{}, fmt.Errorf("build tmux command: %w", err)
	}
	out, err := cmd.Output()
	if err != nil {
		return PaneDimensions{}, fmt.Errorf("tmux display-message: %w", err)
	}
	w, h, ok := parsePaneDimensions(string(out))
	if !ok {
		return PaneDimensions{}, fmt.Errorf("invalid pane dimensions: %q", string(out))
	}
	return PaneDimensions{Width: w, Height: h}, nil
}

// HasSessionContext checks if a tmux session exists, with context for cancellation/timeout.
func HasSessionContext(ctx context.Context, name string) (bool, error) {
	cmd, err := shared.TmuxCommandContext(ctx, "has-session", "-t", name)
	if err != nil {
		return false, fmt.Errorf("build tmux command: %w", err)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return true, nil
	}
	// Context cancellation kills the process, producing an ExitError.
	// Return the context error so callers can distinguish cancellation from "not found".
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	// Distinguish connection/permission errors from "session not found".
	// tmux exits non-zero for both, but connection errors should propagate
	// so the caller can skip the cycle rather than falsely marking dead.
	if msg := strings.TrimSpace(string(out)); strings.Contains(msg, "error connecting") {
		// A missing socket means no dedicated server is running (e.g. the last
		// session exited and the server shut down), which is "not alive" — not
		// a transient connection error to retry.
		if strings.Contains(msg, "No such file or directory") {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session: %s", msg)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("tmux has-session: %w", err)
}
