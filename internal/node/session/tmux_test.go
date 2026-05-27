package session

import (
	"context"
	"fmt"
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

// requireDedicatedSocketUnder is a hard safety guard for tmux integration
// tests. It fails the test unless Argus's resolved tmux socket lives under dir
// (a t.TempDir). This guarantees a test can only ever create or kill sessions
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

func TestBuildStatusRight(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		dir        string
		branch     string
		profile    string
		home       string
		wantExact  string   // if set, assert exact equality
		wantParts  []string // substrings that must all appear
		wantAbsent []string // substrings that must NOT appear
	}{
		{
			name:      "git session with all fields",
			sessionID: "sess_m2abc12_xyz789",
			dir:       "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			branch:    "main",
			home:      "/Users/jeevb",
			wantParts: []string{"sess_m2abc12_xyz789", "main", "bxnlabs/argus"},
		},
		{
			name:      "exact format for short git session",
			sessionID: "sess_abc",
			dir:       "/Users/jeevb/project",
			branch:    "main",
			home:      "/Users/jeevb",
			wantExact: "#[fg=#a6adc8]sess_abc #[fg=#6c7086]| #[fg=#cba6f7] main #[fg=#6c7086]| #[fg=#89b4fa]~/project ",
		},
		{
			name:       "non-git session omits branch segment",
			sessionID:  "sess_m2abc12_xyz789",
			dir:        "/Users/jeevb/projects/myapp",
			branch:     "",
			home:       "/Users/jeevb",
			wantParts:  []string{"sess_m2abc12_xyz789", "~/projects/myapp"},
			wantAbsent: []string{"#[fg=#cba6f7]"}, // branch color absent
		},
		{
			name:      "long branch is truncated",
			sessionID: "sess_abc",
			dir:       "/tmp",
			branch:    "feat/some-really-long-branch-name-that-keeps-going",
			home:      "/Users/jeevb",
			wantParts: []string{"feat/some-really-long-branch-name-…"},
		},
		{
			name:       "branch with hash is escaped",
			sessionID:  "sess_abc",
			dir:        "/tmp",
			branch:     "feat#(echo hacked)",
			home:       "/Users/jeevb",
			wantParts:  []string{"feat##(echo hacked)"},
			wantAbsent: []string{"feat#(echo hacked)"},
		},
		{
			name:       "dir with percent is escaped",
			sessionID:  "sess_abc",
			dir:        "/tmp/100%done",
			branch:     "main",
			home:       "/Users/jeevb",
			wantParts:  []string{"100%%done"},
			wantAbsent: []string{"100%d"},
		},
		{
			name:      "profile segment present when set",
			sessionID: "sess_abc",
			dir:       "/Users/jeevb/project",
			branch:    "main",
			profile:   "work",
			home:      "/Users/jeevb",
			wantParts: []string{"sess_abc", "#[fg=#a6e3a1]work ", "main", "~/project"},
		},
		{
			name:       "no profile segment when empty",
			sessionID:  "sess_abc",
			dir:        "/Users/jeevb/project",
			branch:     "main",
			profile:    "",
			home:       "/Users/jeevb",
			wantAbsent: []string{"#[fg=#a6e3a1]"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStatusRight(tt.sessionID, tt.dir, tt.branch, tt.profile, tt.home)
			if tt.wantExact != "" {
				if got != tt.wantExact {
					t.Errorf("buildStatusRight() =\n  %q\nwant:\n  %q", got, tt.wantExact)
				}
				return
			}
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("buildStatusRight() = %q, want it to contain %q", got, part)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("buildStatusRight() = %q, should NOT contain %q", got, absent)
				}
			}
		})
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

func TestNewSession_AppliesSeededConfig(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)

	// Bootstrap dir + config as the node does at startup.
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(name) })
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// The seeded config sets `mouse on` (tmux defaults to off), so its effect on
	// the running server proves NewSession booted it with -f <seeded config>.
	cmd, err := shared.TmuxCommand("show-options", "-g", "mouse")
	if err != nil {
		t.Fatalf("build show-options: %v", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("show-options: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "mouse on" {
		t.Errorf("seeded config not applied: show-options -g mouse = %q, want %q", got, "mouse on")
	}
}

func TestCapturePaneContext_JoinsWrappedLines(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)

	// Create the tmux dir so tmux can place the socket there, as the node does
	// at startup.
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}

	sess := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { killTestSession(sess) })

	// Create a narrow (40-column) session on the dedicated socket.
	newCmd, err := shared.TmuxCommand("new-session", "-d", "-s", sess, "-x", "40", "-y", "10")
	if err != nil {
		t.Fatalf("build new-session: %v", err)
	}
	if out, err := newCmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}

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
