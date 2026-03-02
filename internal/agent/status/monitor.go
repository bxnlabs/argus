package status

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bxnlabs/argus/internal/agent/db"
	agentsession "github.com/bxnlabs/argus/internal/agent/session"
)

// SessionLister lists sessions from the database.
type SessionLister interface {
	List() ([]*db.Session, error)
}

// SessionToucher updates a session's updated_at timestamp.
type SessionToucher interface {
	TouchSession(id string, unixTS int64) error
}

// StatusDetector detects statuses for a set of session names.
type StatusDetector interface {
	GetAllStatuses(ctx context.Context, names []string) map[string]SessionStatus
	Cleanup()
}

// ActivityEntry holds a session name and its last activity timestamp.
type ActivityEntry struct {
	Name      string
	Timestamp int64
}

// ActivityFetcher returns activity timestamps for all tmux sessions.
// Pass nil to NewMonitor to use the default (session.GetSessionActivitiesContext).
type ActivityFetcher func(ctx context.Context) ([]ActivityEntry, error)

// SnapshotEntry holds the status data for a single session.
type SnapshotEntry struct {
	SessionName string
	Status      SessionStatus
	AgentType   string
}

// Snapshot is the in-memory status snapshot read by the API handler.
type Snapshot struct {
	Statuses map[string]SnapshotEntry // keyed by session ID
}

// Monitor runs a background loop that detects session statuses, fetches
// tmux activity timestamps, and syncs updated_at to the DB. The GET
// handler reads from the in-memory Snapshot with no side effects.
//
// Follows the RepoIndexer Start/Close pattern in internal/github/repos.go.
type Monitor struct {
	lister   SessionLister
	toucher  SessionToucher
	detector StatusDetector
	fetchAct ActivityFetcher

	mu       sync.RWMutex
	snapshot Snapshot

	ctrlMu  sync.Mutex
	cancel  context.CancelFunc
	started bool
	wg      sync.WaitGroup
}

// NewMonitor creates a Monitor. If fetchAct is nil, the default
// (session.GetSessionActivitiesContext) is used.
func NewMonitor(lister SessionLister, toucher SessionToucher, detector StatusDetector, fetchAct ActivityFetcher) *Monitor {
	if fetchAct == nil {
		fetchAct = defaultActivityFetcher
	}
	return &Monitor{
		lister:   lister,
		toucher:  toucher,
		detector: detector,
		fetchAct: fetchAct,
		snapshot: Snapshot{Statuses: make(map[string]SnapshotEntry)},
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

// Snapshot returns a defensive copy of the current status snapshot.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cp := Snapshot{Statuses: make(map[string]SnapshotEntry, len(m.snapshot.Statuses))}
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
	sessions, err := m.lister.List()
	if err != nil {
		log.Printf("status monitor: list sessions: %v", err)
		return
	}

	// Collect tmux names.
	names := make([]string, 0, len(sessions))
	for _, s := range sessions {
		names = append(names, s.TmuxName)
	}

	statuses := m.detector.GetAllStatuses(ctx, names)
	m.detector.Cleanup()

	// Fetch activity timestamps with a 2s timeout.
	actCtx, actCancel := context.WithTimeout(ctx, 2*time.Second)
	activities, err := m.fetchAct(actCtx)
	actCancel()
	if err != nil {
		log.Printf("status monitor: fetch activities: %v", err)
	}

	activityByName := make(map[string]int64, len(activities))
	for _, a := range activities {
		activityByName[a.Name] = a.Timestamp
	}

	// Read prior snapshot to carry forward statuses when the detector
	// omits sessions (e.g. on timeout/cancellation).
	m.mu.RLock()
	prev := m.snapshot
	m.mu.RUnlock()

	// Build new snapshot and touch sessions.
	snap := Snapshot{Statuses: make(map[string]SnapshotEntry, len(sessions))}
	for _, s := range sessions {
		st, ok := statuses[s.TmuxName]
		if !ok {
			// Detector omitted this session (timeout/cancel). Carry
			// forward the prior status; default to idle only when
			// there is no prior value.
			if prev, exists := prev.Statuses[s.ID]; exists {
				st = prev.Status
			} else {
				st = StatusIdle
			}
		}
		snap.Statuses[s.ID] = SnapshotEntry{
			SessionName: s.TmuxName,
			Status:      st,
			AgentType:   s.AgentType,
		}
		if ctx.Err() != nil {
			continue // shutdown requested; skip remaining DB writes
		}
		if ts, ok := activityByName[s.TmuxName]; ok {
			if err := m.toucher.TouchSession(s.ID, ts); err != nil {
				log.Printf("status monitor: touch session %s: %v", s.ID, err)
			}
		}
	}

	m.mu.Lock()
	m.snapshot = snap
	m.mu.Unlock()
}

func defaultActivityFetcher(ctx context.Context) ([]ActivityEntry, error) {
	activities, err := agentsession.GetSessionActivitiesContext(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]ActivityEntry, len(activities))
	for i, a := range activities {
		entries[i] = ActivityEntry{Name: a.Name, Timestamp: a.Timestamp}
	}
	return entries, nil
}
