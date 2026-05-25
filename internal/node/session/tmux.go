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
// server's config is seeded (once) and passed via -f so the first session
// create boots the server with Argus's base config.
func NewSession(name, cwd, command string) error {
	var args []string
	if confPath, err := shared.SeedTmuxConfig(); err != nil {
		// Degrade rather than block session creation: the server still starts,
		// just without Argus's base config.
		log.Printf("seed tmux config: %v", err)
	} else {
		args = append(args, "-f", confPath)
	}
	args = append(args, "new-session", "-d", "-s", name)
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

// ConfigureSession applies the per-session dynamic status-right to a session.
// Static styling (status-style, status-left, mouse, position, lengths) lives in
// the dedicated server's seeded tmux.conf, so only the per-session value is
// applied at runtime here.
func ConfigureSession(name, sessionID, dir, branch, home string) {
	statusRight := buildStatusRight(sessionID, dir, branch, home)
	cmd, err := shared.TmuxCommand("set-option", "-t", name, "status-right", statusRight)
	if err != nil {
		log.Printf("tmux set-option status-right: %v", err)
		return
	}
	if err := cmd.Run(); err != nil {
		log.Printf("tmux set-option status-right: %v", err)
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

// ListSessions returns all tmux session names on the dedicated server.
func ListSessions() ([]string, error) {
	cmd, err := shared.TmuxCommand("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("build tmux command: %w", err)
	}
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
	cmd, err := shared.TmuxCommandContext(ctx, "list-windows", "-a", "-F", "#{session_name}\t#{window_activity}")
	if err != nil {
		return nil, fmt.Errorf("build tmux command: %w", err)
	}
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
