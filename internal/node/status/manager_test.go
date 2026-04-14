package status

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
	nodesession "github.com/bxnlabs/argus/internal/node/session"
)

type fakeManagerLister struct {
	mu       sync.Mutex
	sessions []*db.Session
}

func (f *fakeManagerLister) List(ctx context.Context) ([]*db.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sessions, nil
}

type fakeManagerTmux struct {
	mu      sync.Mutex
	content map[string]string
	dims    nodesession.PaneDimensions
	alive   map[string]bool
}

func newFakeManagerTmux() *fakeManagerTmux {
	return &fakeManagerTmux{
		content: make(map[string]string),
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   make(map[string]bool),
	}
}

func (f *fakeManagerTmux) CapturePaneContent(ctx context.Context, name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.content[name], nil
}

func (f *fakeManagerTmux) GetPaneDimensions(ctx context.Context, name string) (nodesession.PaneDimensions, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.dims, nil
}

func (f *fakeManagerTmux) HasSession(ctx context.Context, name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[name], nil
}

func TestManager_StartupStartsWatchers(t *testing.T) {
	lister := &fakeManagerLister{
		sessions: []*db.Session{
			{ID: "s1", TmuxName: "claude-s1", ProviderType: "claude"},
			{ID: "s2", TmuxName: "claude-s2", ProviderType: "claude"},
		},
	}
	tmux := newFakeManagerTmux()
	tmux.alive["claude-s1"] = true
	tmux.alive["claude-s2"] = true
	tmux.content["claude-s1"] = "content1"
	tmux.content["claude-s2"] = "content2"

	mdb := newMockDB()

	mgr := NewWatcherManager(lister, mdb, tmux)
	mgr.Start(context.Background())
	defer mgr.Close()

	time.Sleep(100 * time.Millisecond)

	snap := mgr.Snapshot()
	if len(snap.Statuses) != 2 {
		t.Errorf("expected 2 statuses, got %d", len(snap.Statuses))
	}
}

func TestManager_EnsureWatchingStartsNewWatcher(t *testing.T) {
	lister := &fakeManagerLister{}
	tmux := newFakeManagerTmux()
	tmux.alive["claude-new"] = true
	tmux.content["claude-new"] = "content"
	mdb := newMockDB()

	mgr := NewWatcherManager(lister, mdb, tmux)
	mgr.Start(context.Background())
	defer mgr.Close()

	mgr.EnsureWatching("new-sess", "claude-new", "claude")

	time.Sleep(100 * time.Millisecond)

	snap := mgr.Snapshot()
	if _, ok := snap.Statuses["new-sess"]; !ok {
		t.Error("expected new session in snapshot after EnsureWatching")
	}
}

func TestManager_StopWatcher(t *testing.T) {
	lister := &fakeManagerLister{}
	tmux := newFakeManagerTmux()
	tmux.alive["claude-del"] = true
	tmux.content["claude-del"] = "content"
	mdb := newMockDB()

	mgr := NewWatcherManager(lister, mdb, tmux)
	mgr.Start(context.Background())
	defer mgr.Close()

	mgr.EnsureWatching("del-sess", "claude-del", "claude")
	time.Sleep(50 * time.Millisecond)

	mgr.StopWatcher("del-sess")
	time.Sleep(50 * time.Millisecond)

	snap := mgr.Snapshot()
	if _, ok := snap.Statuses["del-sess"]; ok {
		t.Error("expected session removed from snapshot after StopWatcher")
	}
}
