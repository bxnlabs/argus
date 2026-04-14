package status

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
)

// SessionLister lists sessions from the database.
type SessionLister interface {
	List(ctx context.Context) ([]*db.Session, error)
}

// SessionToucher updates a session's updated_at timestamp.
type SessionToucher interface {
	TouchSession(ctx context.Context, id string, unixTS int64) error
}

// StatusDetector detects statuses for a set of session names.
type StatusDetector interface {
	GetAllStatuses(ctx context.Context, names []string) map[string]SessionStatus
}

// monitorEntry holds the status data for a single session (internal to Monitor).
type monitorEntry struct {
	SessionName  string
	Status       SessionStatus
	ProviderType string
}

// monitorSnapshot is the in-memory status snapshot used internally by Monitor.
type monitorSnapshot struct {
	Statuses        map[string]monitorEntry // keyed by session ID
	LastRefreshedAt time.Time               // when the last successful refresh completed
}

// Monitor runs a background loop that detects session statuses and syncs
// updated_at to the DB based on detected status. The GET handler reads
// from the in-memory Snapshot with no side effects.
//
// Follows the RepoIndexer Start/Close pattern in internal/github/repos.go.
type Monitor struct {
	lister   SessionLister
	toucher  SessionToucher
	detector StatusDetector

	mu       sync.RWMutex
	snapshot monitorSnapshot

	ctrlMu  sync.Mutex
	cancel  context.CancelFunc
	started bool
	wg      sync.WaitGroup
}

// NewMonitor creates a Monitor.
func NewMonitor(lister SessionLister, toucher SessionToucher, detector StatusDetector) *Monitor {
	return &Monitor{
		lister:   lister,
		toucher:  toucher,
		detector: detector,
		snapshot: monitorSnapshot{Statuses: make(map[string]monitorEntry)},
	}
}

// Start begins the background refresh loop. It is safe to call multiple
// times; only the first call has any effect.
func (m *Monitor) Start(ctx context.Context) {
	m.ctrlMu.Lock()
	defer m.ctrlMu.Unlock()
	if m.started {
		return
	}
	m.started = true
	ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(1)
	go m.loop(ctx)
}

// Close cancels the background loop and waits for it to exit.
func (m *Monitor) Close() {
	m.ctrlMu.Lock()
	cancel := m.cancel
	m.ctrlMu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
}

// StatusSnapshot returns a defensive copy of the current status snapshot.
func (m *Monitor) StatusSnapshot() monitorSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cp := monitorSnapshot{
		Statuses:        make(map[string]monitorEntry, len(m.snapshot.Statuses)),
		LastRefreshedAt: m.snapshot.LastRefreshedAt,
	}
	for k, v := range m.snapshot.Statuses {
		cp.Statuses[k] = v
	}
	return cp
}

func (m *Monitor) loop(ctx context.Context) {
	defer m.wg.Done()

	// Refresh immediately on start.
	m.refresh(ctx)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refresh(ctx)
		}
	}
}

func (m *Monitor) refresh(ctx context.Context) {
	sessions, err := m.lister.List(ctx)
	if err != nil {
		log.Printf("status monitor: list sessions: %v", err)
		return
	}

	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.TmuxName)
	}

	statuses := m.detector.GetAllStatuses(ctx, names)

	// Read prior snapshot for status carryforward and transition detection.
	m.mu.RLock()
	prev := m.snapshot
	m.mu.RUnlock()

	now := time.Now()
	nowUnix := now.Unix()

	snap := monitorSnapshot{Statuses: make(map[string]monitorEntry, len(sessions))}
	for _, s := range sessions {
		st, ok := statuses[s.TmuxName]
		if !ok {
			if prev, exists := prev.Statuses[s.ID]; exists {
				st = prev.Status
			} else {
				st = StatusIdle
			}
		}
		snap.Statuses[s.ID] = monitorEntry{
			SessionName:  s.TmuxName,
			Status:       st,
			ProviderType: s.ProviderType,
		}
		if ctx.Err() != nil {
			continue
		}
		// Bump updated_at when the agent is active or status changed.
		prevEntry, hadPrev := prev.Statuses[s.ID]
		statusChanged := hadPrev && prevEntry.Status != st
		active := st == StatusRunning || st == StatusWaiting
		if active || statusChanged {
			if err := m.toucher.TouchSession(ctx, s.ID, nowUnix); err != nil {
				log.Printf("status monitor: touch session %s: %v", s.ID, err)
			}
		}
	}

	snap.LastRefreshedAt = now

	m.mu.Lock()
	m.snapshot = snap
	m.mu.Unlock()
}
