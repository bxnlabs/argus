package status

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	agentsession "github.com/bxnlabs/argus/internal/node/session"
)

// mockTmux implements TmuxQuerier for tests.
type mockTmux struct {
	activities    []agentsession.SessionActivity
	activitiesErr error
	panes         map[string]string // session name → pane content
	paneErr       error
	captureCount  atomic.Int64 // tracks how many times CapturePaneContent was called
}

func (m *mockTmux) GetSessionActivities(_ context.Context) ([]agentsession.SessionActivity, error) {
	if m.activitiesErr != nil {
		return nil, m.activitiesErr
	}
	return m.activities, nil
}

func (m *mockTmux) CapturePaneContent(_ context.Context, name string) (string, error) {
	m.captureCount.Add(1)
	if m.paneErr != nil {
		return "", m.paneErr
	}
	return m.panes[name], nil
}

// --- GetStatus tests ---

func TestGetStatus_IdleContent(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "$ "},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	got := d.GetStatus(context.Background(), "s1")
	if got != StatusIdle {
		t.Errorf("expected idle for idle content, got %s", got)
	}
	if mock.captureCount.Load() != 1 {
		t.Errorf("expected 1 capture, got %d", mock.captureCount.Load())
	}
}

func TestGetStatus_BusyContent(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "working...\nesc to interrupt\n"},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	got := d.GetStatus(context.Background(), "s1")
	if got != StatusRunning {
		t.Errorf("expected running for busy content, got %s", got)
	}
}

func TestGetStatus_WaitingContent(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "Continue? [Y/n]"},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	got := d.GetStatus(context.Background(), "s1")
	if got != StatusWaiting {
		t.Errorf("expected waiting for waiting content, got %s", got)
	}
}

func TestGetStatus_SessionNotInTmux(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{}, // s1 not listed
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	got := d.GetStatus(context.Background(), "s1")
	if got != StatusDead {
		t.Errorf("expected dead for missing session, got %s", got)
	}
	if mock.captureCount.Load() != 0 {
		t.Error("pane should NOT be captured for dead session")
	}
}

func TestGetStatus_PaneCaptureError_FallsBackToIdle(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		paneErr:    fmt.Errorf("tmux capture-pane failed"),
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	got := d.GetStatus(context.Background(), "s1")
	if got != StatusIdle {
		t.Errorf("expected idle on pane capture error, got %s", got)
	}
}

func TestGetStatus_ActivityQueryError_UsesStaleCache(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "$ "},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	// First call succeeds and populates cache
	got := d.GetStatus(context.Background(), "s1")
	if got != StatusIdle {
		t.Fatalf("expected idle on first call, got %s", got)
	}

	// Force cache to be stale, then make activity query fail
	d.cache.updatedAt = 0
	mock.activitiesErr = fmt.Errorf("tmux list-sessions failed")

	// refreshCache fails → stale cache still has s1 → session classified normally
	got = d.GetStatus(context.Background(), "s1")
	if got != StatusIdle {
		t.Errorf("expected idle from stale cache when activity query fails, got %s", got)
	}
}

func TestGetStatus_ConsecutivePolls_StableIdle(t *testing.T) {
	// Repeated polls on an idle session should consistently return idle
	// without any spurious running blips.
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "$ "},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	for i := 0; i < 5; i++ {
		got := d.GetStatus(context.Background(), "s1")
		if got != StatusIdle {
			t.Errorf("poll %d: expected idle, got %s", i, got)
		}
	}
}

func TestGetStatus_TransitionRunningToIdle(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "working...\nesc to interrupt\n"},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	// Session is running
	got := d.GetStatus(context.Background(), "s1")
	if got != StatusRunning {
		t.Fatalf("expected running, got %s", got)
	}

	// Agent finishes — content changes to idle
	mock.panes["s1"] = "$ "
	d.cache.updatedAt = 0

	got = d.GetStatus(context.Background(), "s1")
	if got != StatusIdle {
		t.Errorf("expected idle after transition, got %s", got)
	}
}

func TestGetStatus_BusyAndWaitingOverlap_PrefersRunning(t *testing.T) {
	// Documents precedence: if both busy and waiting indicators are present
	// in the last 10 lines, busy wins (checkBusyIndicators runs first).
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		panes:      map[string]string{"s1": "esc to interrupt\nContinue? [Y/n]"},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	got := d.GetStatus(context.Background(), "s1")
	if got != StatusRunning {
		t.Errorf("expected running when both busy and waiting present, got %s", got)
	}
}

func TestGetStatus_CaptureErrorThenSessionMissing_SelfCorrectsToDead(t *testing.T) {
	// Verifies idle→dead self-correction: capture error → idle on first poll,
	// then session removed → dead on second poll.
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{{Name: "s1", Timestamp: 100}},
		paneErr:    fmt.Errorf("tmux capture-pane failed"),
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	// First poll: session exists but capture fails → idle
	got := d.GetStatus(context.Background(), "s1")
	if got != StatusIdle {
		t.Fatalf("expected idle on capture error, got %s", got)
	}

	// Second poll: session no longer in tmux → dead
	mock.activities = []agentsession.SessionActivity{} // s1 removed
	mock.paneErr = nil
	d.cache.updatedAt = 0

	got = d.GetStatus(context.Background(), "s1")
	if got != StatusDead {
		t.Errorf("expected dead after session removed, got %s", got)
	}
}

// --- Pattern unit tests ---

func TestCheckBusyIndicators(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"normal output", "hello world\nfoo bar", false},
		{"esc to interrupt", "some output\nesc to interrupt\n", true},
		{"parenthesized", "line1\n(esc to interrupt)\n", true},
		{"dot prefix", "line1\n\u00b7 esc to interrupt\n", true},
		{"spinner char", "line1\nline2\n\u280b processing\n", true},
		{"status line thinking", "line1\n\u2726 cogitating\u2026 (2m 31s \u00b7 thinking)\n", true},
		{"status line tokens", "line1\n\u2736 tinkering\u2026 (1m 5s \u00b7 \u2193 3.4k tokens)\n", true},
		{"status line novel word", "line1\n* xyzfracking\u2026 (5s \u00b7 thinking)\n", true},
		{"word without ellipsis", "line1\ncogitating something\n", false},
		{"truncated text", "line1\n\u23f5\u23f5 bypass permissions on (shift+tab to cycl\u2026\n", false},
		{"running ellipsis", "line1\n  \u23bf  running\u2026\n", true},
		{"background tasks running plural", "output\n\u203b Brewed for 53s \u00b7 3 background tasks still running (\u2193 to manage)\n", true},
		{"background task running singular", "output\n\u203b Brewed for 10s \u00b7 1 background task still running (\u2193 to manage)\n", true},
		{"old scrollback ignored", "esc to interrupt\n" + repeat("safe line\n", 15), false},
		// Mobile scenarios: -J rejoins wrapped lines into logical lines
		{"mobile: background tasks on long logical line", "output\n※ Brewed for 53s · 3 background tasks still running (↓ to manage)\n", true},
		{"mobile: status line on long logical line", "output\n✶ Tinkering… (2m 31s · thinking)\n", true},
		{"mobile: height-clipped no indicators present", "some truncated output\npartial content here\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkBusyIndicators(tt.content); got != tt.want {
				t.Errorf("checkBusyIndicators() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckWaitingPatterns(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"empty", "", false},
		{"normal", "hello world", false},
		{"Y/n prompt", "Continue? [Y/n]", true},
		{"allow", "Allow? (y/n)", true},
		{"press enter", "Press Enter to continue", true},
		{"yes allow all", "  1. Yes, allow all", true},
		{"plan exit full", " Claude has written up a plan and is ready to execute. Would you like to proceed?\n\n \u276f 1. Yes, clear context (13% used) and bypass permissions\n   2. Yes, and bypass permissions\n   3. Yes, manually approve edits\n   4. Type here to tell Claude what to change\n\n ctrl-g to edit in Vim \u00b7 ~/.claude/plans/temporal-finding-meteor.md", true},
		{"old scrollback", "[Y/n]\n" + repeat("safe\n", 20), false},
		// Mobile scenario: -J rejoins wrapped plan prompt into logical lines
		{"mobile: plan exit prompt on long logical line", " Claude has written up a plan and is ready to execute. Would you like to proceed?\n\n ❯ 1. Yes, clear context (13% used) and bypass permissions\n", true},
		{"narrow terminal: plan prompt wrapped across 12+ lines", "\n Claude has written up a plan and is ready to\n execute. Would you like to proceed?\n\n \u276f 1. Yes, clear context (36% used) and bypass\n      permissions\n   2. Yes, and bypass permissions\n   3. Yes, manually approve edits\n   4. Type here to tell Claude what to change\n\n ctrl-g to   Vim \u00b7 ~/.claude/plans/gentle-drift\n edit in        ing-candy.md\n\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkWaitingPatterns(tt.content); got != tt.want {
				t.Errorf("checkWaitingPatterns() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- GetAllStatuses tests ---

func TestGetAllStatuses_MixedStatuses(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{
			{Name: "s1", Timestamp: 100},
			{Name: "s2", Timestamp: 100},
			{Name: "s3", Timestamp: 100},
		},
		panes: map[string]string{
			"s1": "working...\nesc to interrupt\n",
			"s2": "Continue? [Y/n]",
			"s3": "$ ",
		},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	results := d.GetAllStatuses(context.Background(), []string{"s1", "s2", "s3", "s4"})

	if results["s1"] != StatusRunning {
		t.Errorf("s1: expected running, got %s", results["s1"])
	}
	if results["s2"] != StatusWaiting {
		t.Errorf("s2: expected waiting, got %s", results["s2"])
	}
	if results["s3"] != StatusIdle {
		t.Errorf("s3: expected idle, got %s", results["s3"])
	}
	if results["s4"] != StatusDead {
		t.Errorf("s4: expected dead, got %s", results["s4"])
	}
}

func TestGetAllStatuses_CancelledContext_OmitsSessions(t *testing.T) {
	mock := &mockTmux{
		activities: []agentsession.SessionActivity{
			{Name: "s1", Timestamp: 100},
		},
		panes: map[string]string{"s1": "$ "},
	}
	d := newDetector(mock)
	d.cache.updatedAt = 0

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	results := d.GetAllStatuses(ctx, []string{"s1"})
	// Sessions should be omitted (not marked dead) when context is cancelled
	if _, ok := results["s1"]; ok {
		t.Error("expected s1 to be omitted when context is cancelled")
	}
}

func repeat(s string, n int) string {
	result := ""
	for range n {
		result += s
	}
	return result
}
