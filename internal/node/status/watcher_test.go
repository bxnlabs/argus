package status

import (
	"context"
	"sync"
	"testing"
	"time"

	nodesession "github.com/bxnlabs/argus/internal/node/session"
)

// mockTmuxWatcher implements TmuxWatcherOps for testing.
type mockTmuxWatcher struct {
	mu         sync.Mutex
	content    string
	contentErr error
	dims       nodesession.PaneDimensions
	dimsErr    error
	alive      bool
	aliveErr   error
}

func (m *mockTmuxWatcher) CapturePaneContent(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.content, m.contentErr
}

func (m *mockTmuxWatcher) GetPaneDimensions(ctx context.Context, name string) (nodesession.PaneDimensions, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dims, m.dimsErr
}

func (m *mockTmuxWatcher) HasSession(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.alive, m.aliveErr
}

func (m *mockTmuxWatcher) setContent(content string) {
	m.mu.Lock()
	m.content = content
	m.mu.Unlock()
}

type mockDB struct {
	mu          sync.Mutex
	unreadSince map[string]*string
	lastViewed  map[string]string
	touchCalls  []string
}

func newMockDB() *mockDB {
	return &mockDB{
		unreadSince: make(map[string]*string),
		lastViewed:  make(map[string]string),
	}
}

func (m *mockDB) SetUnreadSince(ctx context.Context, id string, ts *string) error {
	m.mu.Lock()
	m.unreadSince[id] = ts
	m.mu.Unlock()
	return nil
}

func (m *mockDB) TouchSession(ctx context.Context, id string, unixTS int64) error {
	m.mu.Lock()
	m.touchCalls = append(m.touchCalls, id)
	m.mu.Unlock()
	return nil
}

func (m *mockDB) GetSession(id string) (unreadSince, lastViewedAt *string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	us := m.unreadSince[id]
	lv := m.lastViewed[id]
	var lvp *string
	if lv != "" {
		lvp = &lv
	}
	return us, lvp, nil
}

func TestWatcher_ActiveOnContentChange(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "initial content",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   true,
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-1", "tmux-sess-1", "claude", tmux, db, snap)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.run(ctx)

	// Wait for first capture cycle
	time.Sleep(50 * time.Millisecond)

	// Need sustained content changes to transition from idle → active
	// (activation threshold = 2 consecutive changed frames)
	tmux.setContent("new content here")
	w.tick()
	time.Sleep(50 * time.Millisecond)
	tmux.setContent("still changing")
	w.tick()
	time.Sleep(50 * time.Millisecond)

	entry, ok := snap.Read().Statuses["sess-1"]
	if !ok {
		t.Fatal("session not in snapshot")
	}
	if entry.State != StateActive {
		t.Errorf("expected active, got %s", entry.State)
	}

	cancel()
}

func TestWatcher_IdleAfterStableFrames(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "stable content",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   true,
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-2", "tmux-sess-2", "claude", tmux, db, snap)
	w.stabilityThreshold = 2

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.run(ctx)

	// Wait for initial capture + stable frames
	for i := 0; i < 4; i++ {
		time.Sleep(20 * time.Millisecond)
		w.tick()
	}
	time.Sleep(20 * time.Millisecond)

	entry, ok := snap.Read().Statuses["sess-2"]
	if !ok {
		t.Fatal("session not in snapshot")
	}
	if entry.State != StateIdle {
		t.Errorf("expected idle, got %s", entry.State)
	}

	cancel()
}

func TestWatcher_DeadOnSessionGone(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "content",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   false, // session gone
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-3", "tmux-sess-3", "claude", tmux, db, snap)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.run(ctx)

	// Wait for first capture cycle to detect dead
	time.Sleep(50 * time.Millisecond)

	entry, ok := snap.Read().Statuses["sess-3"]
	if !ok {
		t.Fatal("session not in snapshot")
	}
	if entry.State != StateDead {
		t.Errorf("expected dead, got %s", entry.State)
	}

	cancel()
}

func TestWatcher_UnreadNotSetWhenObserved(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "content A",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   true,
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	recentTime := time.Now().UTC().Format(sqliteDatetimeFormat)
	db.lastViewed["sess-obs"] = recentTime

	w := newSessionWatcher("sess-obs", "tmux-sess-obs", "claude", tmux, db, snap)
	w.stabilityThreshold = 1
	w.activationThreshold = 1 // use low threshold for this test to focus on unread logic
	w.nowFn = func() time.Time {
		t, _ := time.Parse(sqliteDatetimeFormat, recentTime)
		return t.Add(-1 * time.Second)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.run(ctx)

	time.Sleep(30 * time.Millisecond)

	// Change content to trigger active
	tmux.setContent("content B")
	w.tick()
	time.Sleep(30 * time.Millisecond)

	// Stabilize to trigger active->idle
	w.tick()
	time.Sleep(30 * time.Millisecond)
	w.tick()
	time.Sleep(30 * time.Millisecond)

	db.mu.Lock()
	us := db.unreadSince["sess-obs"]
	db.mu.Unlock()

	if us != nil {
		t.Errorf("expected nil unread_since (session was observed), got %v", *us)
	}

	cancel()
}

func TestWatcher_SingleFrameChangeFromIdleDoesNotReactivate(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "initial",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   true,
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-flap", "tmux-sess-flap", "claude", tmux, db, snap)
	w.stabilityThreshold = 2

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Phase 1: establish active state (need sustained changes from idle)
	w.capture(ctx) // baseline frame (hasPrevFrame = true)

	tmux.setContent("active content 1")
	w.capture(ctx) // changed frame 1 (activationCounter = 1)
	tmux.setContent("active content 2")
	w.capture(ctx) // changed frame 2 → active
	if w.currentState() != StateActive {
		t.Fatalf("expected active after sustained changes, got %s", w.currentState())
	}

	// Phase 2: go idle (2 stable frames)
	w.capture(ctx) // stable frame 1
	w.capture(ctx) // stable frame 2 → idle
	if w.currentState() != StateIdle {
		t.Fatalf("expected idle after stable frames, got %s", w.currentState())
	}

	// Phase 3: single-frame content blip (e.g., status bar update)
	tmux.setContent("status bar updated")
	w.capture(ctx)

	// BUG: with current code this transitions to Active immediately.
	// EXPECTED: should remain Idle — a single frame change is not sustained activity.
	entry, _ := snap.Read().Statuses["sess-flap"]
	if entry.State != StateIdle {
		t.Errorf("single-frame blip should not reactivate: got %s, want idle", entry.State)
	}

	// Phase 4: content stabilizes again
	w.capture(ctx)
	entry, _ = snap.Read().Statuses["sess-flap"]
	if entry.State != StateIdle {
		t.Errorf("expected idle after blip stabilized, got %s", entry.State)
	}
}

func TestWatcher_SustainedChangeFromIdleReactivates(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "initial",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   true,
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-react", "tmux-sess-react", "claude", tmux, db, snap)
	w.stabilityThreshold = 2

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Establish active, then idle
	w.capture(ctx) // baseline
	tmux.setContent("active 1")
	w.capture(ctx) // changed frame 1
	tmux.setContent("active 2")
	w.capture(ctx) // changed frame 2 → active
	w.capture(ctx) // stable 1
	w.capture(ctx) // stable 2 → idle
	if w.currentState() != StateIdle {
		t.Fatalf("expected idle, got %s", w.currentState())
	}

	// Sustained content changes (user typing) should reactivate
	tmux.setContent("typing line 1")
	w.capture(ctx) // changed frame 1 — not yet active
	tmux.setContent("typing line 2")
	w.capture(ctx) // changed frame 2 — should now be active

	entry, _ := snap.Read().Statuses["sess-react"]
	if entry.State != StateActive {
		t.Errorf("sustained changes should reactivate: got %s, want active", entry.State)
	}
}

func TestWatcher_ResizeGuardDiscardsFrame(t *testing.T) {
	dims80x24 := nodesession.PaneDimensions{Width: 80, Height: 24}
	dims100x30 := nodesession.PaneDimensions{Width: 100, Height: 30}

	dimsMock := &changingDimsMock{
		content:   "content",
		alive:     true,
		dimsCalls: []nodesession.PaneDimensions{dims80x24, dims100x30},
	}

	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-resize", "tmux-sess-resize", "claude", dimsMock, db, snap)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.capture(ctx)

	if w.stabilityCounter != 0 {
		t.Errorf("expected stability counter reset after resize, got %d", w.stabilityCounter)
	}

	cancel()
}

// changingDimsMock returns different dimensions on successive calls.
type changingDimsMock struct {
	mu        sync.Mutex
	content   string
	alive     bool
	dimsCalls []nodesession.PaneDimensions
	callIdx   int
}

func (m *changingDimsMock) CapturePaneContent(ctx context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.content, nil
}

func (m *changingDimsMock) GetPaneDimensions(ctx context.Context, name string) (nodesession.PaneDimensions, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callIdx >= len(m.dimsCalls) {
		return m.dimsCalls[len(m.dimsCalls)-1], nil
	}
	d := m.dimsCalls[m.callIdx]
	m.callIdx++
	return d, nil
}

func (m *changingDimsMock) HasSession(ctx context.Context, name string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.alive, nil
}
