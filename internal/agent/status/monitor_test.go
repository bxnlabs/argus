package status

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/agent/db"
)

// --- fakes ---

type fakeLister struct {
	mu       sync.Mutex
	sessions []*db.Session
	err      error
	ch       chan struct{} // optional: signals after each List call
}

func (f *fakeLister) List() ([]*db.Session, error) {
	f.mu.Lock()
	sessions, err := f.sessions, f.err
	f.mu.Unlock()
	if f.ch != nil {
		select {
		case f.ch <- struct{}{}:
		default:
		}
	}
	return sessions, err
}

type fakeToucher struct {
	mu      sync.Mutex
	touched map[string]int64
	ch      chan struct{} // optional: signals after each touch
}

func (f *fakeToucher) TouchSession(id string, unixTS int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.touched == nil {
		f.touched = make(map[string]int64)
	}
	f.touched[id] = unixTS
	if f.ch != nil {
		select {
		case f.ch <- struct{}{}:
		default:
		}
	}
	return nil
}

func (f *fakeToucher) getTouched() map[string]int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make(map[string]int64, len(f.touched))
	for k, v := range f.touched {
		cp[k] = v
	}
	return cp
}

type fakeDetector struct {
	mu           sync.Mutex
	statuses     map[string]SessionStatus
	cleanupCalls int
}

func (f *fakeDetector) GetAllStatuses(_ context.Context, names []string) map[string]SessionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make(map[string]SessionStatus, len(names))
	for _, n := range names {
		if s, ok := f.statuses[n]; ok {
			result[n] = s
		}
	}
	return result
}

func (f *fakeDetector) Cleanup() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanupCalls++
}

func (f *fakeDetector) getCleanupCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cleanupCalls
}

// notifyingFetcher wraps a fetcher and signals a channel after each call.
// Tests wait on this channel instead of using time.Sleep.
func notifyingFetcher(fn ActivityFetcher) (ActivityFetcher, <-chan struct{}) {
	ch := make(chan struct{}, 1)
	return func(ctx context.Context) ([]ActivityEntry, error) {
		result, err := fn(ctx)
		select {
		case ch <- struct{}{}:
		default:
		}
		return result, err
	}, ch
}

// waitForRefresh waits for the channel to signal or fails the test after 5s.
func waitForRefresh(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for refresh")
	}
}

// waitForSnapshot polls mon.Snapshot() until check returns true, or fails
// after timeout. Use this when the fetcher/lister signal fires before the
// snapshot is committed.
func waitForSnapshot(t *testing.T, mon *Monitor, timeout time.Duration, check func(Snapshot) bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check(mon.Snapshot()) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for snapshot condition")
}

// --- tests ---

func TestMonitorSnapshot(t *testing.T) {
	lister := &fakeLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", AgentType: "claude"},
			{ID: "s2", TmuxName: "shell-s2", AgentType: "shell"},
		},
	}
	toucher := &fakeToucher{ch: make(chan struct{}, 2)}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{
			"claude-s1": StatusRunning,
			"shell-s2":  StatusIdle,
		},
	}
	baseFetcher := func(_ context.Context) ([]ActivityEntry, error) {
		return []ActivityEntry{
			{Name: "claude-s1", Timestamp: 1000},
			{Name: "shell-s2", Timestamp: 2000},
		}, nil
	}

	fetcher, refreshed := notifyingFetcher(baseFetcher)
	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	waitForRefresh(t, refreshed)
	// Wait for both touches to complete before closing, since Close()
	// cancels the context and the ctx.Err() guard skips remaining touches.
	waitForRefresh(t, toucher.ch)
	waitForRefresh(t, toucher.ch)
	mon.Close()

	snap := mon.Snapshot()
	if len(snap.Statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(snap.Statuses))
	}

	s1 := snap.Statuses["s1"]
	if s1.SessionName != "claude-s1" {
		t.Errorf("s1 SessionName = %q, want %q", s1.SessionName, "claude-s1")
	}
	if s1.Status != StatusRunning {
		t.Errorf("s1 Status = %q, want %q", s1.Status, StatusRunning)
	}
	if s1.AgentType != "claude" {
		t.Errorf("s1 AgentType = %q, want %q", s1.AgentType, "claude")
	}

	s2 := snap.Statuses["s2"]
	if s2.Status != StatusIdle {
		t.Errorf("s2 Status = %q, want %q", s2.Status, StatusIdle)
	}

	// Verify TouchSession was called with activity timestamps.
	touched := toucher.getTouched()
	if touched["s1"] != 1000 {
		t.Errorf("touched[s1] = %d, want 1000", touched["s1"])
	}
	if touched["s2"] != 2000 {
		t.Errorf("touched[s2] = %d, want 2000", touched["s2"])
	}

	// Verify Cleanup was called.
	if detector.getCleanupCalls() == 0 {
		t.Error("expected Cleanup to be called at least once")
	}
}

func TestMonitorSnapshotEmpty(t *testing.T) {
	lister := &fakeLister{sessions: nil}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}
	baseFetcher := func(_ context.Context) ([]ActivityEntry, error) {
		return nil, nil
	}

	fetcher, refreshed := notifyingFetcher(baseFetcher)
	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	waitForRefresh(t, refreshed)
	mon.Close()

	snap := mon.Snapshot()
	if len(snap.Statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(snap.Statuses))
	}
}

func TestMonitorCloseWithoutStart(t *testing.T) {
	lister := &fakeLister{}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}
	mon := NewMonitor(lister, toucher, detector, nil)
	mon.Close() // must not panic
}

func TestMonitorDoubleStartIsSafe(t *testing.T) {
	calls := 0
	var mu sync.Mutex
	lister := &fakeLister{sessions: nil}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}

	ch := make(chan struct{}, 1)
	fetcher := func(_ context.Context) ([]ActivityEntry, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		select {
		case ch <- struct{}{}:
		default:
		}
		return nil, nil
	}

	mon := NewMonitor(lister, toucher, detector, fetcher)
	ctx := context.Background()
	mon.Start(ctx)
	mon.Start(ctx) // second call must be a no-op
	waitForRefresh(t, ch)
	mon.Close()

	mu.Lock()
	c := calls
	mu.Unlock()
	// Assert at least 1 call (the initial refresh). We don't assert exactly 1
	// because the ticker could fire on a very slow machine before Close().
	// The test intent is that double-Start doesn't create two goroutines.
	if c < 1 {
		t.Errorf("expected fetchAct called at least once, got %d", c)
	}
}

func TestMonitorSnapshotIsDefensiveCopy(t *testing.T) {
	lister := &fakeLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", AgentType: "claude"},
		},
	}
	toucher := &fakeToucher{}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{"claude-s1": StatusRunning},
	}
	baseFetcher := func(_ context.Context) ([]ActivityEntry, error) { return nil, nil }

	fetcher, refreshed := notifyingFetcher(baseFetcher)
	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	waitForRefresh(t, refreshed)
	mon.Close()

	snap1 := mon.Snapshot()
	snap1.Statuses["s1"] = SnapshotEntry{SessionName: "mutated", Status: StatusDead}

	snap2 := mon.Snapshot()
	if snap2.Statuses["s1"].SessionName != "claude-s1" {
		t.Errorf("internal state was mutated: SessionName = %q, want %q", snap2.Statuses["s1"].SessionName, "claude-s1")
	}
}

func TestMonitorListError(t *testing.T) {
	lister := &fakeLister{
		err: errors.New("db unavailable"),
		ch:  make(chan struct{}, 1),
	}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}
	fetcher := func(_ context.Context) ([]ActivityEntry, error) { return nil, nil }

	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	waitForRefresh(t, lister.ch)
	mon.Close()

	// List error → refresh exits early → snapshot stays empty.
	snap := mon.Snapshot()
	if len(snap.Statuses) != 0 {
		t.Errorf("expected 0 statuses after List error, got %d", len(snap.Statuses))
	}

	// No sessions should have been touched.
	if len(toucher.getTouched()) != 0 {
		t.Error("expected no touches after List error")
	}
}

func TestMonitorFetchActivityError(t *testing.T) {
	lister := &fakeLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", AgentType: "claude"},
		},
	}
	toucher := &fakeToucher{}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{"claude-s1": StatusRunning},
	}
	baseFetcher := func(_ context.Context) ([]ActivityEntry, error) {
		return nil, errors.New("tmux unavailable")
	}

	fetcher, refreshed := notifyingFetcher(baseFetcher)
	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	waitForRefresh(t, refreshed)
	mon.Close()

	// Snapshot should still be built (statuses come from detector).
	snap := mon.Snapshot()
	if len(snap.Statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(snap.Statuses))
	}
	if snap.Statuses["s1"].Status != StatusRunning {
		t.Errorf("status = %q, want %q", snap.Statuses["s1"].Status, StatusRunning)
	}

	// No sessions should have been touched (no activity data).
	if len(toucher.getTouched()) != 0 {
		t.Error("expected no touches when activity fetch fails")
	}
}

func TestMonitorCarriesForwardStatusOnDetectorOmission(t *testing.T) {
	lister := &fakeLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", AgentType: "claude"},
		},
	}
	toucher := &fakeToucher{}

	// First refresh: detector returns Running.
	// Second refresh: detector omits the session (simulates timeout).
	callCount := 0
	var detMu sync.Mutex
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{"claude-s1": StatusRunning},
	}

	refreshDone := make(chan struct{}, 2)
	fetcher := func(_ context.Context) ([]ActivityEntry, error) {
		detMu.Lock()
		callCount++
		n := callCount
		detMu.Unlock()
		if n == 2 {
			// On the second refresh, clear detector statuses to simulate
			// timeout/omission.
			detector.mu.Lock()
			detector.statuses = map[string]SessionStatus{}
			detector.mu.Unlock()
		}
		select {
		case refreshDone <- struct{}{}:
		default:
		}
		return nil, nil
	}

	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())

	// Wait for first refresh signal, then poll until snapshot is committed.
	waitForRefresh(t, refreshDone)
	waitForSnapshot(t, mon, 5*time.Second, func(s Snapshot) bool {
		return s.Statuses["s1"].Status == StatusRunning
	})

	// Wait for second refresh → detector omits s1, should carry forward Running.
	waitForRefresh(t, refreshDone)
	// Close to ensure the second refresh has fully completed.
	mon.Close()

	snap2 := mon.Snapshot()
	if snap2.Statuses["s1"].Status != StatusRunning {
		t.Errorf("after detector omission: status = %q, want %q (should carry forward)", snap2.Statuses["s1"].Status, StatusRunning)
	}
}
