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

// HasSession checks if a tmux session exists.
func HasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// NewSession creates a new tmux session running the given command.
// If command is empty, starts a default shell.
func NewSession(name, cwd, command string) error {
	args := []string{"new-session", "-d", "-s", name}
	if cwd != "" {
		args = append(args, "-c", cwd)
	}
	if command != "" {
		args = append(args, command)
	}

	cmd := exec.Command("tmux", args...)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, string(out))
	}
	return nil
}

const (
	maxDirWidth    = 50
	maxBranchWidth = 35
)

// escapeTmuxLiteral escapes characters that tmux interprets in format strings:
// # -> ## (prevents #(...) command execution, #{...} variable expansion, #[...] style changes)
// % -> %% (prevents strftime expansion like %H, %M)
// Control characters are normalized to spaces to prevent malformed rendering.
var tmuxEscaper = strings.NewReplacer("#", "##", "%", "%%", "\n", " ", "\r", " ", "\t", " ")

func escapeTmuxLiteral(s string) string {
	return tmuxEscaper.Replace(s)
}

// buildStatusRight formats the right side of the tmux status bar.
// Layout with branch:    "{sessionID} | {branch} | {dir} "
// Layout without branch: "{sessionID} | {dir} "
func buildStatusRight(sessionID, dir, branch, home string) string {
	displayDir := escapeTmuxLiteral(shared.CompressPath(dir, home, maxDirWidth))
	displayID := escapeTmuxLiteral(sessionID)

	if branch == "" {
		return fmt.Sprintf("#[fg=#a6adc8]%s #[fg=#6c7086]| #[fg=#89b4fa]%s ", displayID, displayDir)
	}
	displayBranch := escapeTmuxLiteral(shared.TruncateRight(branch, maxBranchWidth))
	return fmt.Sprintf("#[fg=#a6adc8]%s #[fg=#6c7086]| #[fg=#cba6f7] %s #[fg=#6c7086]| #[fg=#89b4fa]%s ", displayID, displayBranch, displayDir)
}

// ConfigureSession applies the standard Argus tmux status bar styling to a session.
func ConfigureSession(name, sessionID, dir, branch, home string) {
	statusRight := buildStatusRight(sessionID, dir, branch, home)
	options := []struct{ key, val string }{
		{"status-style", "bg=#1e1e2e,fg=#cdd6f4"},
		{"status-left", "#[fg=#cba6f7,bold] Argus #[fg=#6c7086]| "},
		{"status-left-length", "20"},
		{"status-right", statusRight},
		{"status-right-length", "110"},
		{"status-position", "bottom"},
		{"mouse", "on"},
	}
	for _, o := range options {
		if err := exec.Command("tmux", "set-option", "-t", name, o.key, o.val).Run(); err != nil {
			log.Printf("tmux set-option %s: %v", o.key, err)
		}
	}
}

// KillSession kills a tmux session.
func KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, string(out))
	}
	return nil
}

// ListSessions returns all tmux session names.
func ListSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// tmux exits non-zero when no server is running — expected.
		// Log and propagate unexpected errors (e.g. binary not found).
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			log.Printf("tmux list-sessions: %v", err)
			return nil, err
		}
		return nil, nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var names []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

// CapturePane captures the visible pane content of a tmux session.
func CapturePane(name string) (string, error) {
	return CapturePaneContext(context.Background(), name)
}

// CapturePaneContext captures pane content with context for cancellation/timeout.
func CapturePaneContext(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", name, "-p", "-J")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GetPaneCwd returns the current working directory of a tmux pane.
func GetPaneCwd(name string) (string, error) {
	cmd := exec.Command("tmux", "display-message", "-t", name, "-p", "#{pane_current_path}")
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
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-t", name, "-p", "#{pane_width}x#{pane_height}")
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
	cmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", name)
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
		return false, fmt.Errorf("tmux has-session: %s", msg)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, fmt.Errorf("tmux has-session: %w", err)
}

// SessionActivity holds tmux session activity timestamps.
type SessionActivity struct {
	Name      string
	Timestamp int64 // unix timestamp of last activity
}

// GetSessionActivitiesContext returns activity timestamps with context for cancellation/timeout.
// It uses window_activity (last pane output) rather than session_activity
// so that merely attaching to a session does not bump the timestamp.
func GetSessionActivitiesContext(ctx context.Context) ([]SessionActivity, error) {
	cmd := exec.CommandContext(ctx, "tmux", "list-windows", "-a", "-F", "#{session_name}\t#{window_activity}")
	out, err := cmd.Output()
	if err != nil {
		// Context cancellation kills the process, producing an ExitError.
		// Return the context error so callers can preserve stale state.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// tmux exits non-zero when no server is running — expected.
		// Log and propagate unexpected errors (e.g. binary not found).
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			log.Printf("tmux list-windows: %v", err)
			return nil, err
		}
		return nil, nil
	}

	return parseWindowActivities(string(out)), nil
}

// parseWindowActivities parses the tab-separated output of
// `tmux list-windows -a -F "#{session_name}\t#{window_activity}"`.
// A session may have multiple windows; the max timestamp wins.
func parseWindowActivities(output string) []SessionActivity {
	maxTS := make(map[string]int64)
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		ts, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		if cur, ok := maxTS[parts[0]]; !ok || ts > cur {
			maxTS[parts[0]] = ts
		}
	}

	activities := make([]SessionActivity, 0, len(maxTS))
	for name, ts := range maxTS {
		activities = append(activities, SessionActivity{
			Name:      name,
			Timestamp: ts,
		})
	}
	return activities
}
