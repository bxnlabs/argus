# Dedicated, Isolated tmux Server Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Route every Argus `tmux` interaction through a dedicated, isolated tmux server (its own socket + seeded config under `$ARGUS_HOME/tmux/`) so Argus never touches the user's personal tmux server, and vice versa.

**Architecture:** A single command-builder in `internal/shared` threads `-S <socket>` through every `tmux` invocation, shared by the node and CLI. The socket and a user-editable `tmux.conf` live under `$ARGUS_HOME/tmux/` via the existing `shared.StateDir()` conventions. The config is seeded once on first session create (never overwritten); static status-bar styling moves from per-session `set-option` calls into that config, leaving only the dynamic `status-right` applied per session.

**Tech Stack:** Go, `os/exec`, tmux. Existing test pattern: standard `testing` package, `t.Setenv("ARGUS_HOME", ...)`, integration tests gated on `hasTmux()`.

---

## Background for the implementer

- `internal/shared/paths.go` already has `StateDir()` → `$ARGUS_HOME` or `~/.argus`. All path helpers join onto it.
- `internal/node/session/tmux.go` is the central wrapper: ~10 functions each do `exec.Command("tmux", ...)` against the **default** tmux server. We add `-S <socket>` to every one.
- Two attach call sites also shell out to tmux: the web bridge at `internal/node/terminal/handler.go:326`, and the CLI's `attachTmux` in `cmd/argus/cli/session_attach.go` (used by both `argus attach` and `argus new`).
- `internal/node/session/initscript.go` has a `tmux capture-pane` that runs **inside** a session — it auto-targets the dedicated server via `$TMUX`, so it is intentionally left unchanged.
- `tmux`'s `-S <path>` flag selects the server socket; `-f <conf>` is read only when the server first starts (ignored against a running server). tmux creates the socket file but **not** its parent directory, so we must `MkdirAll` first.
- Migration is intentionally code-free: after the switch, sessions Argus created on the default server read as dead and `EnsureSession` revives them on the dedicated server on next access. No task implements migration.

## File Structure

| File | Responsibility |
|------|----------------|
| `internal/shared/tmux.go` (new) | Socket/config path resolution, the `TmuxCommand`/`TmuxCommandContext` builder, the base `tmux.conf` template, and `SeedTmuxConfig` (seed-once writer). |
| `internal/shared/tmux_test.go` (new) | Unit tests for path resolution, the builder arg vector, and seed/no-overwrite behavior. |
| `internal/node/session/tmux.go` (modify) | Route every call through `shared.TmuxCommand`; `NewSession` seeds config + passes `-f`; `ConfigureSession` sets only the dynamic `status-right`. |
| `internal/node/session/tmux_test.go` (modify) | Integration tests asserting sessions land on the dedicated socket and not the default server. |
| `internal/node/terminal/handler.go` (modify) | Web attach via `shared.TmuxCommand`. |
| `cmd/argus/cli/session_attach.go` (modify) | CLI attach via `shared.TmuxCommand`. |

---

## Task 1: Shared socket path + command builder

**Files:**
- Create: `internal/shared/tmux.go`
- Test: `internal/shared/tmux_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/shared/tmux_test.go`:

```go
package shared

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxSocketPathHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath: %v", err)
	}
	want := filepath.Join("/custom/home", "tmux", "server")
	if got != want {
		t.Errorf("TmuxSocketPath() = %q, want %q", got, want)
	}
}

func TestTmuxConfigPathHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := TmuxConfigPath()
	if err != nil {
		t.Fatalf("TmuxConfigPath: %v", err)
	}
	want := filepath.Join("/custom/home", "tmux", "tmux.conf")
	if got != want {
		t.Errorf("TmuxConfigPath() = %q, want %q", got, want)
	}
}

func TestTmuxCommandThreadsSocket(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	cmd, err := TmuxCommand("has-session", "-t", "x")
	if err != nil {
		t.Fatalf("TmuxCommand: %v", err)
	}
	sock := filepath.Join("/custom/home", "tmux", "server")
	want := []string{"tmux", "-S", sock, "has-session", "-t", "x"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestTmuxCommandContextThreadsSocket(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	cmd, err := TmuxCommandContext(context.Background(), "list-sessions")
	if err != nil {
		t.Fatalf("TmuxCommandContext: %v", err)
	}
	sock := filepath.Join("/custom/home", "tmux", "server")
	want := []string{"tmux", "-S", sock, "list-sessions"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestSeedTmuxConfig_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	got, err := SeedTmuxConfig()
	if err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	want := filepath.Join(dir, "tmux", "tmux.conf")
	if got != want {
		t.Fatalf("SeedTmuxConfig() = %q, want %q", got, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, directive := range []string{
		`default-terminal "tmux-256color"`,
		"terminal-overrides",
		"mouse on",
		"status-position bottom",
	} {
		if !strings.Contains(string(data), directive) {
			t.Errorf("config missing %q\ngot:\n%s", directive, data)
		}
	}
}

func TestSeedTmuxConfig_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	confPath := filepath.Join(dir, "tmux", "tmux.conf")
	custom := "# my custom config\nset -g mouse off\n"
	if err := os.WriteFile(confPath, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("SeedTmuxConfig overwrote user config\ngot:\n%s\nwant:\n%s", data, custom)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/shared/ -run 'Tmux|SeedTmux' -v`
Expected: FAIL — `undefined: TmuxSocketPath` (and the other helpers).

- [ ] **Step 3: Implement the helpers**

Create `internal/shared/tmux.go`:

```go
package shared

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// baseTmuxConfig is the default config seeded for Argus's dedicated tmux
// server. It is written once (see SeedTmuxConfig) and then owned by the user;
// Argus never overwrites it. It restores the rendering and styling Argus
// previously inherited from the user's ~/.tmux.conf on the shared server.
const baseTmuxConfig = `# Argus tmux defaults — seeded once. Edit to customize; Argus won't overwrite.
set -g default-terminal "tmux-256color"
set -ga terminal-overrides ",*:Tc"
set -g mouse on
set -g status-style "bg=#1e1e2e,fg=#cdd6f4"
set -g status-left "#[fg=#cba6f7,bold] Argus #[fg=#6c7086]| "
set -g status-left-length 20
set -g status-right-length 110
set -g status-position bottom
`

// TmuxSocketPath returns the path to Argus's dedicated tmux server socket:
// <StateDir>/tmux/server. Honors ARGUS_HOME so the dev stack is isolated.
func TmuxSocketPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmux", "server"), nil
}

// TmuxConfigPath returns the path to Argus's tmux config: <StateDir>/tmux/tmux.conf.
func TmuxConfigPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmux", "tmux.conf"), nil
}

// TmuxCommand builds an *exec.Cmd that targets Argus's dedicated tmux server
// socket. It is the single place the -S flag is threaded; all callers (node
// and CLI) build their tmux commands through it.
func TmuxCommand(args ...string) (*exec.Cmd, error) {
	sock, err := TmuxSocketPath()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-S", sock}, args...)
	return exec.Command("tmux", full...), nil
}

// TmuxCommandContext is TmuxCommand with a context for cancellation/timeout.
func TmuxCommandContext(ctx context.Context, args ...string) (*exec.Cmd, error) {
	sock, err := TmuxSocketPath()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-S", sock}, args...)
	return exec.CommandContext(ctx, "tmux", full...), nil
}

// SeedTmuxConfig ensures <StateDir>/tmux exists and writes the default
// tmux.conf only when it is missing, so a user-edited config is never
// overwritten. Returns the config path; the directory creation it performs is
// also what lets tmux create the socket there on first new-session.
func SeedTmuxConfig() (string, error) {
	confPath, err := TmuxConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0700); err != nil {
		return "", fmt.Errorf("create tmux dir: %w", err)
	}
	f, err := os.OpenFile(confPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	switch {
	case err == nil:
		defer f.Close()
		if _, werr := f.WriteString(baseTmuxConfig); werr != nil {
			return "", fmt.Errorf("write tmux config: %w", werr)
		}
	case errors.Is(err, os.ErrExist):
		// Existing file (possibly user-edited) — leave it untouched.
	default:
		return "", fmt.Errorf("open tmux config: %w", err)
	}
	return confPath, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/shared/ -run 'Tmux|SeedTmux' -v`
Expected: PASS (all 6 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/shared/tmux.go internal/shared/tmux_test.go
git add internal/shared/tmux.go internal/shared/tmux_test.go
git commit -m "feat(tmux): dedicated socket path + command builder"
```

---

## Task 2: Route the session tmux wrapper through the dedicated socket

**Files:**
- Modify: `internal/node/session/tmux.go` (full new content below)
- Test: `internal/node/session/tmux_test.go` (add one test, rewrite one test)

This task changes every `exec.Command("tmux", ...)` in `tmux.go` to `shared.TmuxCommand(...)`, makes `NewSession` seed the config and pass `-f`, and slims `ConfigureSession` to the dynamic `status-right` (the static styling now lives in the seeded config from Task 1).

- [ ] **Step 1: Write the failing integration test**

Add to `internal/node/session/tmux_test.go` (and add the shared import — see Step 2):

```go
func TestNewSession_UsesDedicatedSocket(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	t.Cleanup(func() {
		if cmd, err := shared.TmuxCommand("kill-server"); err == nil {
			cmd.Run()
		}
	})

	// Visible on the dedicated socket.
	if !HasSession(name) {
		t.Errorf("session %q not found on dedicated socket", name)
	}

	// NOT visible on the user's default tmux server.
	if exec.Command("tmux", "has-session", "-t", name).Run() == nil {
		t.Errorf("session %q leaked onto the default tmux server", name)
	}
}
```

- [ ] **Step 2: Update the test file imports and rewrite the capture test**

In `internal/node/session/tmux_test.go`, change the import block to add the shared package:

```go
import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/shared"
)
```

Then replace the existing `TestCapturePaneContext_JoinsWrappedLines` with a version that creates its session on the dedicated socket (so the now socket-scoped `CapturePaneContext` can see it):

```go
func TestCapturePaneContext_JoinsWrappedLines(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)

	// Ensure the dedicated socket directory exists before tmux creates the socket.
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	sess := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())

	// Create a narrow (40-column) session on the dedicated socket.
	newCmd, err := shared.TmuxCommand("new-session", "-d", "-s", sess, "-x", "40", "-y", "10")
	if err != nil {
		t.Fatalf("build new-session: %v", err)
	}
	if out, err := newCmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		if cmd, err := shared.TmuxCommand("kill-server"); err == nil {
			cmd.Run()
		}
	})

	// Send a line longer than the pane width so tmux wraps it.
	longLine := "background tasks still running indicator test line"
	sendCmd, err := shared.TmuxCommand("send-keys", "-t", sess, fmt.Sprintf("echo '%s'", longLine), "Enter")
	if err != nil {
		t.Fatalf("build send-keys: %v", err)
	}
	if out, err := sendCmd.CombinedOutput(); err != nil {
		t.Fatalf("send-keys: %v: %s", err, out)
	}

	// Give tmux a moment to render.
	time.Sleep(500 * time.Millisecond)

	content, err := CapturePaneContext(context.Background(), sess)
	if err != nil {
		t.Fatalf("CapturePaneContext: %v", err)
	}

	if !strings.Contains(content, longLine) {
		t.Errorf("expected captured content to contain %q as a single logical line\ngot:\n%s", longLine, content)
	}
}
```

- [ ] **Step 3: Run the new test to verify it fails**

Run: `go test ./internal/node/session/ -run TestNewSession_UsesDedicatedSocket -v`
Expected: FAIL — the current `NewSession`/`HasSession` ignore `ARGUS_HOME` and use the default server, so the session leaks onto the default server (the "leaked onto the default tmux server" assertion fails).

- [ ] **Step 4: Rewrite `internal/node/session/tmux.go`**

Replace the entire file with:

```go
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
		return nil, err
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
		return "", err
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
		return "", err
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
		return PaneDimensions{}, err
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
		return false, err
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
		return nil, err
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
```

- [ ] **Step 5: Run the session package tests to verify they pass**

Run: `go test ./internal/node/session/ -run 'TestNewSession_UsesDedicatedSocket|TestCapturePaneContext_JoinsWrappedLines|TestBuildStatusRight|TestParseWindowActivities|TestParsePaneDimensions' -v`
Expected: PASS. (`TestNewSession_UsesDedicatedSocket` and the capture test require tmux; they skip if absent.)

- [ ] **Step 6: Commit**

```bash
gofmt -w internal/node/session/tmux.go internal/node/session/tmux_test.go
git add internal/node/session/tmux.go internal/node/session/tmux_test.go
git commit -m "feat(tmux): route session wrapper through dedicated socket"
```

---

## Task 3: Web attach via the dedicated socket

**Files:**
- Modify: `internal/node/terminal/handler.go` (imports + `HandleSessionWebSocket`, around line 326)

The web bridge currently does `exec.Command("tmux", "attach-session", "-t", tmuxName)`, which would attach to the default server and never see the dedicated session.

- [ ] **Step 1: Add the shared import**

In `internal/node/terminal/handler.go`, the import block currently includes:

```go
	agentsession "github.com/bxnlabs/argus/internal/node/session"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
```

Add the shared import so the block reads:

```go
	agentsession "github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/creack/pty"
	"github.com/gorilla/websocket"
```

- [ ] **Step 2: Build the attach command via the shared builder**

In `HandleSessionWebSocket`, replace:

```go
		if onReady != nil {
			onReady(id, tmuxName)
		}
		cmd := exec.Command("tmux", "attach-session", "-t", tmuxName)
		handleConnection(w, r, cmd, tmuxName)
```

with:

```go
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
```

Note: `exec`, `log`, and `http` are already imported and still used elsewhere in the file (`exec.Cmd` in the `session` struct, `exec.Command` in `HandleTerminalWebSocket`), so no import removal is needed.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/node/terminal/`
Expected: builds with no errors.

- [ ] **Step 4: Commit**

```bash
gofmt -w internal/node/terminal/handler.go
git add internal/node/terminal/handler.go
git commit -m "feat(tmux): web attach targets dedicated socket"
```

---

## Task 4: CLI attach via the dedicated socket

**Files:**
- Modify: `cmd/argus/cli/session_attach.go` (imports + `attachTmux`)

`attachTmux` is the single CLI tmux entry point — both `argus attach` and `argus new` (via `session_create.go`) call it. Changing it covers both.

- [ ] **Step 1: Add the shared import**

In `cmd/argus/cli/session_attach.go`, the import block is currently:

```go
import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)
```

Change it to add the shared package:

```go
import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/bxnlabs/argus/internal/shared"
)
```

(`os/exec` stays — the function still uses `*exec.ExitError` in its exit-code handling.)

- [ ] **Step 2: Build the attach command via the shared builder**

Replace the start of `attachTmux`:

```go
func attachTmux(sessionID, tmuxName, baseURL string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	tmuxCmd := exec.Command(tmuxPath, "attach-session", "-t", tmuxName)
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	if err := tmuxCmd.Start(); err != nil {
		return fmt.Errorf("start tmux: %w", err)
	}
```

with:

```go
func attachTmux(sessionID, tmuxName, baseURL string) error {
	tmuxCmd, err := shared.TmuxCommand("attach-session", "-t", tmuxName)
	if err != nil {
		return fmt.Errorf("build tmux command: %w", err)
	}
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	if err := tmuxCmd.Start(); err != nil {
		return fmt.Errorf("start tmux: %w", err)
	}
```

(The rest of `attachTmux` — the heartbeat goroutine and `tmuxCmd.Wait()` exit handling — is unchanged.)

- [ ] **Step 3: Verify it compiles**

Run: `go build ./cmd/argus/...`
Expected: builds with no errors. If the compiler reports `"os/exec" imported and not used`, confirm the `*exec.ExitError` branch later in `attachTmux` is intact — it is the only remaining `exec` reference and must stay.

- [ ] **Step 4: Commit**

```bash
gofmt -w cmd/argus/cli/session_attach.go
git add cmd/argus/cli/session_attach.go
git commit -m "feat(tmux): CLI attach targets dedicated socket"
```

---

## Task 5: Full verification

**Files:** none (build + test + manual smoke test)

- [ ] **Step 1: Build everything**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 2: Vet**

Run: `go vet ./...`
Expected: no errors.

- [ ] **Step 3: Run the full test suite**

Run: `go test ./...`
Expected: PASS. tmux-dependent integration tests run if `tmux` is on PATH, otherwise skip.

- [ ] **Step 4: Manual smoke test — isolation from the default server**

Confirm that creating a session does not touch the user's default tmux server, and lands on the dedicated socket.

```bash
# Build the binary.
go build -o /tmp/argus ./cmd/argus

# Note the default server's sessions before (may print "no server running").
tmux ls || echo "(no default server)"

# Start the node + create a session per the repo's normal dev flow, e.g.:
#   /tmp/argus node      (in one shell)
#   /tmp/argus new smoke-test --provider shell   (in another)
# Then verify:

# 1) The session is on Argus's dedicated socket:
tmux -S ~/.argus/tmux/server ls
# Expected: lists the "shell-sess_..." session.

# 2) The default server is unchanged (no Argus session leaked in):
tmux ls || echo "(no default server)"
# Expected: same as before — no new Argus session.

# 3) The seeded config exists and is the editable default:
cat ~/.argus/tmux/tmux.conf
# Expected: the "Argus tmux defaults — seeded once" file.
```

Expected: the session appears only under `tmux -S ~/.argus/tmux/server ls`, never in plain `tmux ls`; `~/.argus/tmux/tmux.conf` exists.

- [ ] **Step 5: Manual smoke test — config is not overwritten**

```bash
# Append a marker to the seeded config, then create another session.
echo "# user marker" >> ~/.argus/tmux/tmux.conf
/tmp/argus new smoke-test-2 --provider shell   # or your normal create flow
grep "user marker" ~/.argus/tmux/tmux.conf
```

Expected: the `# user marker` line is still present — `SeedTmuxConfig` did not overwrite the user-edited file.

- [ ] **Step 6: Final commit (if any formatting/cleanup remains)**

```bash
git status
# If gofmt or cleanup produced changes:
git add -A
git commit -m "chore(tmux): formatting and cleanup"
```

---

## Self-Review

**Spec coverage:**
- Socket + config under `$ARGUS_HOME/tmux/` via shared helpers → Task 1 (`TmuxSocketPath`, `TmuxConfigPath`). ✓
- Single command-builder threaded through every call site → Task 1 (`TmuxCommand`), consumed in Tasks 2/3/4. ✓
- Bootstrap: seed-once via `O_CREATE|O_EXCL`, `-f` on `new-session`, degrade on failure → Task 1 (`SeedTmuxConfig`) + Task 2 (`NewSession`). ✓
- Base `tmux.conf` content (truecolor, default-terminal, mouse, static status styling) → Task 1 (`baseTmuxConfig`). ✓
- `ConfigureSession` slimmed to dynamic `status-right` → Task 2. ✓
- `initscript.go` `tmux capture-pane` unchanged (uses `$TMUX`) → no task, documented in Background. ✓
- Web attach (`handler.go`) → Task 3. ✓
- CLI attach (`session_attach.go`, covers `session_create.go`) → Task 4. ✓
- Migration: auto-revive, no code → no task, documented in Background. ✓
- Testing: shared unit (paths/builder/seed), integration isolation, capture test on dedicated socket → Tasks 1, 2. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code; every command lists expected output. ✓

**Type consistency:** `TmuxCommand`/`TmuxCommandContext`/`TmuxSocketPath`/`TmuxConfigPath`/`SeedTmuxConfig` signatures are defined in Task 1 and called identically in Tasks 2–4 (`shared.TmuxCommand(args...) (*exec.Cmd, error)`, `shared.SeedTmuxConfig() (string, error)`). `ConfigureSession`'s signature is unchanged. ✓
