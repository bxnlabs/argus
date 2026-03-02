package status

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/agent/db"
)

// --- fakes ---

type fakeLister struct {
	sessions []*db.Session
	err      error
}

func (f *fakeLister) List() ([]*db.Session, error) { return f.sessions, f.err }

type fakeToucher struct {
	mu      sync.Mutex
	touched map[string]int64
}

func (f *fakeToucher) TouchSession(id string, unixTS int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.touched == nil {
		f.touched = make(map[string]int64)
	}
	f.touched[id] = unixTS
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
	mu          sync.Mutex
	statuses    map[string]SessionStatus
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

// --- tests ---

func TestMonitorSnapshot(t *testing.T) {
	lister := &fakeLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", AgentType: "claude"},
			{ID: "s2", TmuxName: "shell-s2", AgentType: "shell"},
		},
	}
	toucher := &fakeToucher{}
	detector := &fakeDetector{
		statuses: map[string]SessionStatus{
			"claude-s1": StatusRunning,
			"shell-s2":  StatusIdle,
		},
	}
	fetcher := func(_ context.Context) ([]ActivityEntry, error) {
		return []ActivityEntry{
			{Name: "claude-s1", Timestamp: 1000},
			{Name: "shell-s2", Timestamp: 2000},
		}, nil
	}

	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
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
	fetcher := func(_ context.Context) ([]ActivityEntry, error) {
		return nil, nil
	}

	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
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
	fetcher := func(_ context.Context) ([]ActivityEntry, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil, nil
	}

	mon := NewMonitor(lister, toucher, detector, fetcher)
	ctx := context.Background()
	mon.Start(ctx)
	mon.Start(ctx) // second call must be a no-op
	time.Sleep(100 * time.Millisecond)
	mon.Close()

	mu.Lock()
	c := calls
	mu.Unlock()
	if c != 1 {
		t.Errorf("expected fetchAct called once, got %d", c)
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
	fetcher := func(_ context.Context) ([]ActivityEntry, error) { return nil, nil }

	mon := NewMonitor(lister, toucher, detector, fetcher)
	mon.Start(context.Background())
	time.Sleep(100 * time.Millisecond)
	mon.Close()

	snap1 := mon.Snapshot()
	snap1.Statuses["s1"] = SnapshotEntry{SessionName: "mutated", Status: StatusDead}

	snap2 := mon.Snapshot()
	if snap2.Statuses["s1"].SessionName != "claude-s1" {
		t.Errorf("internal state was mutated: SessionName = %q, want %q", snap2.Statuses["s1"].SessionName, "claude-s1")
	}
}
