package status

import (
	"context"
	"log"
	"sync"

	"github.com/bxnlabs/argus/internal/node/db"
)

// SessionLister lists sessions from the database.
type SessionLister interface {
	List(ctx context.Context) ([]*db.Session, error)
}

// WatcherManager manages the lifecycle of all SessionWatcher goroutines.
type WatcherManager struct {
	lister   SessionLister
	db       WatcherDB
	tmux     TmuxWatcherOps
	snapshot *ActivitySnapshot

	mu       sync.Mutex
	watchers map[string]*watcherEntry // keyed by session ID
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	ctrlMu  sync.Mutex
	started bool
}

type watcherEntry struct {
	watcher *SessionWatcher
	cancel  context.CancelFunc
}

// NewWatcherManager creates a WatcherManager.
func NewWatcherManager(lister SessionLister, db WatcherDB, tmux TmuxWatcherOps) *WatcherManager {
	return &WatcherManager{
		lister:   lister,
		db:       db,
		tmux:     tmux,
		snapshot: NewActivitySnapshot(),
		watchers: make(map[string]*watcherEntry),
	}
}

// Start begins the manager: queries all sessions and starts watchers.
// Safe to call multiple times; only the first call has effect.
func (m *WatcherManager) Start(ctx context.Context) {
	m.ctrlMu.Lock()
	defer m.ctrlMu.Unlock()
	if m.started {
		return
	}
	m.ctx, m.cancel = context.WithCancel(ctx)

	sessions, err := m.lister.List(m.ctx)
	if err != nil {
		log.Printf("watcher manager: list sessions on startup: %v", err)
		// Leave m.ctx alive so EnsureWatching can still start individual watchers.
		return
	}
	m.started = true

	for _, s := range sessions {
		m.startWatcherLocked(s.ID, s.TmuxName, s.ProviderType)
	}
}

// Close cancels all watchers and waits for them to exit.
func (m *WatcherManager) Close() {
	m.ctrlMu.Lock()
	cancel := m.cancel
	m.ctrlMu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
}

// EnsureWatching guarantees a watcher is running for the given session.
// Idempotent — safe to call on every EnsureSession return.
func (m *WatcherManager) EnsureWatching(sessionID, tmuxName, providerType string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry, ok := m.watchers[sessionID]; ok {
		if entry.watcher.currentState() == StateDead {
			entry.cancel()
			delete(m.watchers, sessionID)
			m.snapshot.Remove(sessionID)
		} else {
			return
		}
	}

	m.startWatcherLocked(sessionID, tmuxName, providerType)
}

// StopWatcher stops and removes the watcher for a session (on delete).
func (m *WatcherManager) StopWatcher(sessionID string) {
	m.mu.Lock()
	entry, ok := m.watchers[sessionID]
	if ok {
		delete(m.watchers, sessionID)
	}
	m.mu.Unlock()

	if ok {
		entry.cancel()
		m.snapshot.Remove(sessionID)
	}
}

// Snapshot returns a defensive copy of the current activity state.
func (m *WatcherManager) Snapshot() Snapshot {
	return m.snapshot.Read()
}

// startWatcherLocked starts a watcher goroutine. Must hold m.mu.
func (m *WatcherManager) startWatcherLocked(sessionID, tmuxName, providerType string) {
	ctx, cancel := context.WithCancel(m.ctx)
	w := newSessionWatcher(sessionID, tmuxName, providerType, m.tmux, m.db, m.snapshot)

	m.watchers[sessionID] = &watcherEntry{watcher: w, cancel: cancel}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		w.run(ctx)
	}()
}
