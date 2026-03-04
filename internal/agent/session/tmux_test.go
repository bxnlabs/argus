package session

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestParseWindowActivities(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   []SessionActivity
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
			branch:    "feat/some-really-long-branch-name",
			home:      "/Users/jeevb",
			wantParts: []string{"feat/some-really-lo…"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStatusRight(tt.sessionID, tt.dir, tt.branch, tt.home)
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

func TestCapturePaneContext_JoinsWrappedLines(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}

	sess := fmt.Sprintf("argus-test-%d", time.Now().UnixNano())

	// Create a narrow (40-column) tmux session.
	cmd := exec.Command("tmux", "new-session", "-d", "-s", sess, "-x", "40", "-y", "10")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, out)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", sess).Run()
	})

	// Send a line longer than the pane width so tmux wraps it.
	longLine := "background tasks still running indicator test line"
	cmd = exec.Command("tmux", "send-keys", "-t", sess, fmt.Sprintf("echo '%s'", longLine), "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("send-keys: %v: %s", err, out)
	}

	// Give tmux a moment to render.
	time.Sleep(500 * time.Millisecond)

	content, err := CapturePaneContext(context.Background(), sess)
	if err != nil {
		t.Fatalf("CapturePaneContext: %v", err)
	}

	// With -J, the long line should appear as a single logical line containing
	// the full string, not split across multiple physical rows.
	if !strings.Contains(content, longLine) {
		t.Errorf("expected captured content to contain %q as a single logical line\ngot:\n%s", longLine, content)
	}
}
