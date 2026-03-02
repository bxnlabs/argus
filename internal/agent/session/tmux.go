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

// ConfigureSession applies the standard Argus tmux status bar styling to a session.
func ConfigureSession(name string) {
	options := []struct{ key, val string }{
		{"status-style", "bg=#1e1e2e,fg=#cdd6f4"},
		{"status-left", "#[fg=#cba6f7,bold] Argus #[fg=#6c7086]| "},
		{"status-left-length", "20"},
		{"status-right", "#[fg=#6c7086]| #[fg=#89b4fa]#S #[fg=#6c7086]| #[fg=#a6adc8]%H:%M "},
		{"status-right-length", "40"},
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
