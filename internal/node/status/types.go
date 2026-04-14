package status

import (
	"context"
	"sync"
	"time"
)

// SessionState is the activity state of a session.
type SessionState string

const (
	StateActive   SessionState = "active"
	StateIdle SessionState = "idle"
	StateDead     SessionState = "dead"
)

// SnapshotEntry holds the activity state for a single session.
type SnapshotEntry struct {
	SessionName  string
	State        SessionState
	ProviderType string
}

// Snapshot is the in-memory activity state snapshot read by the API handler.
type Snapshot struct {
	Statuses        map[string]SnapshotEntry // keyed by session ID
	LastRefreshedAt time.Time
}

// ActivitySnapshot is a thread-safe container for session activity state.
// Watchers write to it; the API handler reads from it.
type ActivitySnapshot struct {
	mu       sync.RWMutex
	statuses map[string]SnapshotEntry
	updated  time.Time
}

// NewActivitySnapshot creates an empty snapshot.
func NewActivitySnapshot() *ActivitySnapshot {
	return &ActivitySnapshot{
		statuses: make(map[string]SnapshotEntry),
	}
}

// Set writes a single session's activity state.
func (s *ActivitySnapshot) Set(sessionID string, entry SnapshotEntry) {
	s.mu.Lock()
	s.statuses[sessionID] = entry
	s.updated = time.Now()
	s.mu.Unlock()
}

// Remove deletes a session from the snapshot.
func (s *ActivitySnapshot) Remove(sessionID string) {
	s.mu.Lock()
	delete(s.statuses, sessionID)
	s.mu.Unlock()
}

// Read returns a defensive copy of the current snapshot.
func (s *ActivitySnapshot) Read() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cp := Snapshot{
		Statuses:        make(map[string]SnapshotEntry, len(s.statuses)),
		LastRefreshedAt: s.updated,
	}
	for k, v := range s.statuses {
		cp.Statuses[k] = v
	}
	return cp
}

// Notification holds the data passed to a Notifier backend.
type Notification struct {
	SessionID   string
	SessionName string
	UnreadSince time.Time
	State       SessionState
}

// Notifier is a pluggable interface for future notification backends
// (Slack, email, webhooks). No concrete implementations ship initially.
type Notifier interface {
	Notify(ctx context.Context, notification Notification) error
}
