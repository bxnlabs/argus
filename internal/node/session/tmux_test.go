package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/shared"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// shortTempDir returns a throwaway directory to point ARGUS_HOME at, and
// registers its removal.
//
// t.TempDir() is unusable for anything that starts a tmux server: it names the
// directory after the test function, and on macOS the resulting path plus
// "/tmux/server" overruns sockaddr_un's 104-byte capacity, which tmux reports
// as an opaque "File name too long". The name here is a fixed length instead,
// so the socket path no longer depends on how a test is spelled and renaming
// one can't break it. shared.TmuxSocketPath enforces the limit itself, so if
// even this path is too long (a long TMPDIR) the guard below fails naming it.
//
// The path is symlink-resolved because macOS hands out /var/... paths that are
// really /private/var/...; tests that compare against resolved worktree paths
// need the real one.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "argus")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	return resolved
}

// requireDedicatedSocketUnder is a hard safety guard for tmux integration
// tests. It fails the test unless Argus's resolved tmux socket lives under dir
// (a shortTempDir). This guarantees a test can only ever create or kill sessions
// on a throwaway dedicated server — never the user's real default or ~/.argus
// tmux server. Call it immediately after t.Setenv("ARGUS_HOME", dir) and
// before any tmux command.
func requireDedicatedSocketUnder(t *testing.T, dir string) {
	t.Helper()
	sock, err := shared.TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath: %v", err)
	}
	if !strings.HasPrefix(sock, filepath.Clean(dir)+string(filepath.Separator)) {
		t.Fatalf("refusing to run: tmux socket %q is not under temp dir %q", sock, dir)
	}
}

// killTestSession removes a single named session from Argus's dedicated
// (throwaway) socket. It deliberately targets one session by name rather than
// running kill-server, so an integration test can never tear down a whole tmux
// server — the server exits on its own once its last session is gone.
func killTestSession(name string) {
	if cmd, err := shared.TmuxCommand("kill-session", "-t", name); err == nil {
		cmd.Run()
	}
}

func TestParsePaneDimensions(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantW     int
		wantH     int
		wantValid bool
	}{
		{"normal", "80x24", 80, 24, true},
		{"large", "200x50", 200, 50, true},
		{"empty", "", 0, 0, false},
		{"no separator", "8024", 0, 0, false},
		{"non-numeric width", "abcx24", 0, 0, false},
		{"non-numeric height", "80xabc", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, ok := parsePaneDimensions(tt.input)
			if ok != tt.wantValid {
				t.Errorf("parsePaneDimensions(%q) valid=%v, want %v", tt.input, ok, tt.wantValid)
			}
			if ok && (w != tt.wantW || h != tt.wantH) {
				t.Errorf("parsePaneDimensions(%q) = (%d,%d), want (%d,%d)", tt.input, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

// bootstrapTmuxConfigs runs the config half of the node's startup (see
// node.Setup) against a throwaway ARGUS_HOME, so a session created afterwards
// boots a server the same way production does.
func bootstrapTmuxConfigs(t *testing.T) {
	t.Helper()
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	if _, err := shared.WriteManagedTmuxConfig(); err != nil {
		t.Fatalf("WriteManagedTmuxConfig: %v", err)
	}
}

// showOption reads one option back off the throwaway server, e.g.
// showOption(t, "-g", "mouse") -> "mouse on".
func showOption(t *testing.T, args ...string) string {
	t.Helper()
	cmd, err := shared.TmuxCommand(append([]string{"show-options"}, args...)...)
	if err != nil {
		t.Fatalf("build show-options: %v", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show-options %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestNewSession_AppliesManagedConfig(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := shortTempDir(t)
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)
	bootstrapTmuxConfigs(t)

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(name) })
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Each of these differs from tmux's own default, so seeing it on the running
	// server proves NewSession booted with -f <managed config>.
	for _, want := range []string{"mouse on", "history-limit 20000"} {
		option := strings.SplitN(want, " ", 2)[0]
		if got := showOption(t, "-g", option); got != want {
			t.Errorf("managed config not applied: show-options -g %s = %q, want %q", option, got, want)
		}
	}
	if got := showOption(t, "-s", "focus-events"); got != "focus-events on" {
		t.Errorf("managed config not applied: show-options -s focus-events = %q, want %q", got, "focus-events on")
	}
}

// The managed config sources the user's *after* its own defaults, which is the
// only thing making those defaults overridable rather than imposed. Nothing
// about the file layout says so on its own, so prove it against real tmux.
func TestNewSession_UserConfigOverridesDefaults(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := shortTempDir(t)
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)
	bootstrapTmuxConfigs(t)

	userPath, err := shared.TmuxUserConfigPath()
	if err != nil {
		t.Fatalf("TmuxUserConfigPath: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("set -g history-limit 4321\n"), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(name) })
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if got := showOption(t, "-g", "history-limit"); got != "history-limit 4321" {
		t.Errorf("user config did not override the default: show-options -g history-limit = %q, want %q", got, "history-limit 4321")
	}
}

// The web UI carries the session's identity in its own status bar, so tmux's
// bar is redundant chrome eating a row of every pane. The managed config turns
// it off after sourcing the user's, so even a user config asking for the bar
// back leaves it off — tmux's own default is on, so "off" here proves it.
func TestNewSession_DisablesStatusBar(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := shortTempDir(t)
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)
	bootstrapTmuxConfigs(t)

	userPath, err := shared.TmuxUserConfigPath()
	if err != nil {
		t.Fatalf("TmuxUserConfigPath: %v", err)
	}
	if err := os.WriteFile(userPath, []byte("set -g status on\n"), 0o600); err != nil {
		t.Fatalf("write user config: %v", err)
	}

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(name) })
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Ask what the session actually renders, not what is set on it: the value
	// now arrives by inheriting the global, so `show-options -t <name> status`
	// reports nothing at all. display-message resolves the inheritance.
	cmd, err := shared.TmuxCommand("display-message", "-t", name, "-p", "#{status}")
	if err != nil {
		t.Fatalf("build display-message: %v", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("display-message: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "off" {
		t.Errorf("session %s renders status = %q, want %q", name, got, "off")
	}
}

// A settings change has to reach a server that is already up, or it waits on
// every live session exiting. Booting with -f cannot do that; SourceManagedTmuxConfig
// is what closes the gap, and node.Setup calls it on every start.
func TestSourceManagedTmuxConfig_ReachesRunningServer(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := shortTempDir(t)
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)
	bootstrapTmuxConfigs(t)

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(name) })
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Stand in for an upgrade changing a default while the server is up.
	managedPath, err := shared.TmuxManagedConfigPath()
	if err != nil {
		t.Fatalf("TmuxManagedConfigPath: %v", err)
	}
	if err := os.WriteFile(managedPath, []byte("set -g history-limit 9876\n"), 0o600); err != nil {
		t.Fatalf("write managed config: %v", err)
	}
	if err := shared.SourceManagedTmuxConfig(); err != nil {
		t.Fatalf("SourceManagedTmuxConfig: %v", err)
	}

	if got := showOption(t, "-g", "history-limit"); got != "history-limit 9876" {
		t.Errorf("show-options -g history-limit = %q, want %q (config never reached the running server)", got, "history-limit 9876")
	}
}

func TestCapturePaneContext_JoinsWrappedLines(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := shortTempDir(t)
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)

	// Create the tmux dir so tmux can place the socket there, as the node does
	// at startup.
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}

	sess := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(sess) })

	// A line longer than the 40-column pane, so tmux wraps it across two rows
	// and capture-pane -J has something to join.
	longLine := "background tasks still running indicator test line"

	// Print it from a fixed command rather than typing it into an interactive
	// shell. A login shell renders whatever prompt the developer has
	// configured, and a tall one (a two-line prompt wraps to seven rows at 40
	// columns) scrolls the line off the top of a 10-row pane before it can be
	// captured — making the test pass or fail on the reader's dotfiles. The
	// sleep just holds the pane open; t.Cleanup kills the session.
	newCmd, err := shared.TmuxCommand("new-session", "-d", "-s", sess, "-x", "40", "-y", "10",
		fmt.Sprintf("printf '%%s\\n' '%s'; sleep 60", longLine))
	if err != nil {
		t.Fatalf("build new-session: %v", err)
	}
	if out, err := newCmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}

	// Poll until the pane has rendered rather than sleeping a guessed interval.
	deadline := time.Now().Add(10 * time.Second)
	for {
		content, err := CapturePaneContext(context.Background(), sess)
		if err != nil {
			t.Fatalf("CapturePaneContext: %v", err)
		}
		if strings.Contains(content, longLine) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected captured content to contain %q as a single logical line\ngot:\n%s", longLine, content)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
