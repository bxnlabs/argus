package status

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
)

// --- fakes ---

type fakeLister struct {
	mu       sync.Mutex
	sessions []*db.Session
	err      error
	ch       chan struct{} // optional: signals after each List call
}

func (f *fakeLister) List(_ context.Context) ([]*db.Session, error) {
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

func (f *fakeToucher) TouchSession(_ context.Context, id string, unixTS int64) error {
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
	mu       sync.Mutex
	statuses map[string]SessionStatus
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
// after timeout. Use this when the lister signal fires before the snapshot
// is committed.
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
			{ID: "s1", TmuxName: "claude-s1", ProviderType: "claude"},
			{ID: "s2", TmuxName: "shell-s2", ProviderType: "shell"},
		},
	}
	toucher := &fakeToucher{ch: make(chan struct{}, 2)}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{
			"claude-s1": StatusRunning,
			"shell-s2":  StatusIdle,
		},
	}

	before := time.Now().Unix()
	mon := NewMonitor(lister, toucher, detector)
	mon.Start(context.Background())
	// Wait for the active session's touch to complete before closing.
	// On first refresh only active sessions (running/waiting) are touched;
	// idle sessions seen for the first time are NOT touched to avoid
	// inflating updated_at on every agent restart.
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
	if s1.ProviderType != "claude" {
		t.Errorf("s1 ProviderType = %q, want %q", s1.ProviderType, "claude")
	}

	s2 := snap.Statuses["s2"]
	if s2.Status != StatusIdle {
		t.Errorf("s2 Status = %q, want %q", s2.Status, StatusIdle)
	}

	// Verify TouchSession was called for s1 (active) but NOT s2 (idle, first seen).
	touched := toucher.getTouched()
	if touched["s1"] < before {
		t.Errorf("touched[s1] = %d, want >= %d", touched["s1"], before)
	}
	if _, ok := touched["s2"]; ok {
		t.Errorf("idle session s2 should not be touched on first refresh, got %d", touched["s2"])
	}

	// Verify LastRefreshedAt is set.
	if snap.LastRefreshedAt.IsZero() {
		t.Error("expected LastRefreshedAt to be non-zero after successful refresh")
	}
}

func TestMonitorSnapshotEmpty(t *testing.T) {
	lister := &fakeLister{sessions: nil, ch: make(chan struct{}, 1)}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}

	mon := NewMonitor(lister, toucher, detector)
	mon.Start(context.Background())
	waitForRefresh(t, lister.ch)
	mon.Close()

	snap := mon.Snapshot()
	if len(snap.Statuses) != 0 {
		t.Errorf("expected 0 statuses, got %d", len(snap.Statuses))
	}

	// Empty list is still a successful refresh.
	if snap.LastRefreshedAt.IsZero() {
		t.Error("expected LastRefreshedAt to be non-zero after successful refresh with empty list")
	}
}

func TestMonitorCloseWithoutStart(t *testing.T) {
	lister := &fakeLister{}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}
	mon := NewMonitor(lister, toucher, detector)
	mon.Close() // must not panic
}

func TestMonitorDoubleStartIsSafe(t *testing.T) {
	lister := &fakeLister{sessions: nil, ch: make(chan struct{}, 1)}
	toucher := &fakeToucher{}
	detector := &fakeDetector{statuses: map[string]SessionStatus{}}

	mon := NewMonitor(lister, toucher, detector)
	ctx := context.Background()
	mon.Start(ctx)
	mon.Start(ctx) // second call must be a no-op
	waitForRefresh(t, lister.ch)
	mon.Close()

	// The test intent is that double-Start doesn't create two goroutines.
	// If it did, we'd see panics or races under -race.
}

func TestMonitorSnapshotIsDefensiveCopy(t *testing.T) {
	lister := &fakeLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", ProviderType: "claude"},
		},
		ch: make(chan struct{}, 1),
	}
	toucher := &fakeToucher{}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{"claude-s1": StatusRunning},
	}

	mon := NewMonitor(lister, toucher, detector)
	mon.Start(context.Background())
	waitForSnapshot(t, mon, 5*time.Second, func(s Snapshot) bool {
		return len(s.Statuses) == 1
	})
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

	mon := NewMonitor(lister, toucher, detector)
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

	// LastRefreshedAt should be zero — refresh exits early on List error.
	if !snap.LastRefreshedAt.IsZero() {
		t.Error("expected LastRefreshedAt to be zero after List error")
	}
}

func TestMonitorCarriesForwardStatusOnDetectorOmission(t *testing.T) {
	// Use a counting lister that clears detector statuses on the second call.
	sessions := []*db.Session{
		{ID: "s1", TmuxName: "claude-s1", ProviderType: "claude"},
	}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{"claude-s1": StatusRunning},
	}

	callCount := 0
	var countMu sync.Mutex
	listCh := make(chan struct{}, 2)
	lister := &countingLister{
		sessions: sessions,
		onList: func(n int) {
			if n == 2 {
				// On the second refresh, clear detector statuses to simulate
				// timeout/omission.
				detector.mu.Lock()
				detector.statuses = map[string]SessionStatus{}
				detector.mu.Unlock()
			}
		},
		countMu:  &countMu,
		count:    &callCount,
		ch:       listCh,
	}
	toucher := &fakeToucher{}

	mon := NewMonitor(lister, toucher, detector)
	mon.Start(context.Background())

	// Wait for first refresh, then poll until snapshot is committed.
	waitForRefresh(t, listCh)
	waitForSnapshot(t, mon, 5*time.Second, func(s Snapshot) bool {
		return s.Statuses["s1"].Status == StatusRunning
	})

	// Wait for second refresh → detector omits s1, should carry forward Running.
	waitForRefresh(t, listCh)
	mon.Close()

	snap2 := mon.Snapshot()
	if snap2.Statuses["s1"].Status != StatusRunning {
		t.Errorf("after detector omission: status = %q, want %q (should carry forward)", snap2.Statuses["s1"].Status, StatusRunning)
	}
}

// countingLister is a SessionLister that counts calls and runs a callback.
type countingLister struct {
	sessions []*db.Session
	onList   func(n int)
	countMu  *sync.Mutex
	count    *int
	ch       chan struct{}
}

func (c *countingLister) List(_ context.Context) ([]*db.Session, error) {
	c.countMu.Lock()
	*c.count++
	n := *c.count
	c.countMu.Unlock()
	if c.onList != nil {
		c.onList(n)
	}
	if c.ch != nil {
		select {
		case c.ch <- struct{}{}:
		default:
		}
	}
	return c.sessions, nil
}
