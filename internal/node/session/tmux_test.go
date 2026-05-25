package session

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
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

func TestParseWindowActivities(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []SessionActivity
	}{
		{
			name:  "empty output",
			input: "",
			want:  nil,
		},
		{
			name:  "single session single window",
			input: "mysess\t1709300000\n",
			want:  []SessionActivity{{Name: "mysess", Timestamp: 1709300000}},
		},
		{
			name:  "multi-window takes max timestamp",
			input: "mysess\t1709300000\nmysess\t1709300500\nmysess\t1709300100\n",
			want:  []SessionActivity{{Name: "mysess", Timestamp: 1709300500}},
		},
		{
			name:  "multiple sessions",
			input: "sess-a\t1709300000\nsess-b\t1709300100\n",
			want: []SessionActivity{
				{Name: "sess-a", Timestamp: 1709300000},
				{Name: "sess-b", Timestamp: 1709300100},
			},
		},
		{
			name:  "malformed line skipped",
			input: "mysess\t1709300000\nbadline\nmysess2\t1709300100\n",
			want: []SessionActivity{
				{Name: "mysess", Timestamp: 1709300000},
				{Name: "mysess2", Timestamp: 1709300100},
			},
		},
		{
			name:  "non-numeric timestamp skipped",
			input: "mysess\tnotanumber\ngood\t1709300000\n",
			want:  []SessionActivity{{Name: "good", Timestamp: 1709300000}},
		},
		{
			name:  "zero timestamp is preserved",
			input: "mysess\t0\n",
			want:  []SessionActivity{{Name: "mysess", Timestamp: 0}},
		},
		{
			name:  "empty session name skipped",
			input: "\t1709300000\ngood\t1709300100\n",
			want:  []SessionActivity{{Name: "good", Timestamp: 1709300100}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWindowActivities(tt.input)

			// Sort both slices by name for deterministic comparison.
			sort.Slice(got, func(i, j int) bool { return got[i].Name < got[j].Name })
			sort.Slice(tt.want, func(i, j int) bool { return tt.want[i].Name < tt.want[j].Name })

			if len(got) != len(tt.want) {
				t.Fatalf("got %d activities, want %d\ngot:  %+v\nwant: %+v", len(got), len(tt.want), got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("activity[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetSessionActivitiesContext_CanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	activities, err := GetSessionActivitiesContext(ctx)
	if activities != nil {
		t.Errorf("expected nil activities, got %v", activities)
	}
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestBuildStatusRight(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		dir        string
		branch     string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStatusRight(tt.sessionID, tt.dir, tt.branch, tt.home)
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

func TestNewSession_UsesDedicatedSocket(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)

	name := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())
	// Register cleanup before creating, so a session is torn down even if a
	// later assertion fails.
	t.Cleanup(func() { killTestSession(name) })
	if err := NewSession(name, "", ""); err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// Visible on the dedicated socket.
	if !HasSession(name) {
		t.Errorf("session %q not found on dedicated socket", name)
	}

	// NOT visible on the user's default tmux server.
	if exec.Command("tmux", "has-session", "-t", name).Run() == nil {
		t.Errorf("session %q leaked onto the default tmux server", name)
	}
}

func TestCapturePaneContext_JoinsWrappedLines(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	requireDedicatedSocketUnder(t, dir)

	// SeedTmuxConfig's side-effect is creating the tmux dir so tmux can place
	// the socket there; the seeded config itself is not used by this test.
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("seed config: %v", err)
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
