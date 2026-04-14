package status

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	nodesession "github.com/bxnlabs/argus/internal/node/session"
)

const (
	defaultCaptureInterval    = 2 * time.Second
	defaultStabilityThreshold = 2 // consecutive identical frames
)

const sqliteDatetimeFormat = "2006-01-02 15:04:05"

// TmuxWatcherOps abstracts tmux operations needed by the watcher.
type TmuxWatcherOps interface {
	CapturePaneContent(ctx context.Context, name string) (string, error)
	GetPaneDimensions(ctx context.Context, name string) (nodesession.PaneDimensions, error)
	HasSession(ctx context.Context, name string) (bool, error)
}

// WatcherDB abstracts database operations needed by the watcher.
type WatcherDB interface {
	SetUnreadSince(ctx context.Context, id string, ts *string) error
	TouchSession(ctx context.Context, id string, unixTS int64) error
	GetSession(id string) (unreadSince, lastViewedAt *string, err error)
}

// SessionWatcher runs a per-session goroutine that captures tmux pane content,
// compares consecutive frames, and infers activity state.
type SessionWatcher struct {
	sessionID    string
	tmuxName     string
	providerType string

	tmux     TmuxWatcherOps
	db       WatcherDB
	snapshot *ActivitySnapshot

	// State machine (mu guards state for cross-goroutine reads from WatcherManager)
	stateMu            sync.RWMutex
	state              SessionState
	prevContent        string
	prevDims           nodesession.PaneDimensions
	hasPrevFrame       bool
	stabilityCounter   int
	stabilityThreshold int
	lastActivityTime   time.Time

	// Manual tick channel for testing (bypasses timer)
	tickCh chan struct{}

	// nowFn for testing
	nowFn func() time.Time
}

func newSessionWatcher(
	sessionID, tmuxName, providerType string,
	tmux TmuxWatcherOps,
	db WatcherDB,
	snapshot *ActivitySnapshot,
) *SessionWatcher {
	return &SessionWatcher{
		sessionID:          sessionID,
		tmuxName:           tmuxName,
		providerType:       providerType,
		tmux:               tmux,
		db:                 db,
		snapshot:           snapshot,
		state:              StateIdle,
		stabilityThreshold: defaultStabilityThreshold,
		tickCh:             make(chan struct{}, 1),
		nowFn:              time.Now,
	}
}

// tick triggers a manual capture cycle (for testing).
func (w *SessionWatcher) tick() {
	select {
	case w.tickCh <- struct{}{}:
	default:
	}
}

func (w *SessionWatcher) run(ctx context.Context) {
	// Initial capture cycle
	w.capture(ctx)

	ticker := time.NewTicker(defaultCaptureInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.capture(ctx)
		case <-w.tickCh:
			w.capture(ctx)
		}
		if w.currentState() == StateDead {
			return
		}
	}
}

func (w *SessionWatcher) capture(ctx context.Context) {
	now := w.nowFn()

	// Dead detection: check if tmux session exists.
	alive, err := w.tmux.HasSession(ctx, w.tmuxName)
	if err != nil {
		log.Printf("watcher %s: has-session error: %v", w.sessionID, err)
		return
	}
	if !alive {
		w.transitionTo(ctx, StateDead, now)
		return
	}

	// Pre-capture dimensions
	preDims, err := w.tmux.GetPaneDimensions(ctx, w.tmuxName)
	if err != nil {
		log.Printf("watcher %s: pre-dims error: %v", w.sessionID, err)
		return
	}

	// Capture content
	content, err := w.tmux.CapturePaneContent(ctx, w.tmuxName)
	if err != nil {
		log.Printf("watcher %s: capture error: %v", w.sessionID, err)
		return
	}

	// Post-capture dimensions
	postDims, err := w.tmux.GetPaneDimensions(ctx, w.tmuxName)
	if err != nil {
		log.Printf("watcher %s: post-dims error: %v", w.sessionID, err)
		return
	}

	// Atomic resize guard: discard frame if dimensions changed during capture
	if preDims != postDims {
		w.stabilityCounter = 0
		return
	}

	// If no previous frame, store baseline and return
	if !w.hasPrevFrame {
		w.prevContent = content
		w.prevDims = preDims
		w.hasPrevFrame = true
		w.writeSnapshot()
		return
	}

	// Dimensions differ from previous baseline — store new baseline, skip comparison
	if preDims != w.prevDims {
		w.prevContent = content
		w.prevDims = preDims
		w.stabilityCounter = 0
		w.writeSnapshot()
		return
	}

	// Compare content
	if content != w.prevContent {
		// Content changed → active
		w.prevContent = content
		w.stabilityCounter = 0
		w.lastActivityTime = now

		prevState := w.currentState()
		w.setState(StateActive)

		// Clear unread_since on activity resume (idle -> active)
		if prevState == StateIdle {
			if err := w.db.SetUnreadSince(ctx, w.sessionID, nil); err != nil {
				log.Printf("watcher %s: clear unread: %v", w.sessionID, err)
			}
		}

		// Touch updated_at while active
		if err := w.db.TouchSession(ctx, w.sessionID, now.Unix()); err != nil {
			log.Printf("watcher %s: touch: %v", w.sessionID, err)
		}
	} else {
		// Content identical → increment stability counter
		w.stabilityCounter++

		if w.stabilityCounter >= w.stabilityThreshold && w.currentState() != StateIdle {
			prevState := w.currentState()
			w.transitionToIdle(ctx, prevState, now)
		}
	}

	w.writeSnapshot()
}

func (w *SessionWatcher) transitionToIdle(ctx context.Context, prevState SessionState, now time.Time) {
	w.setState(StateIdle)

	if prevState == StateActive {
		_, lastViewedAt, err := w.db.GetSession(w.sessionID)
		if err != nil {
			log.Printf("watcher %s: get session for unread check: %v", w.sessionID, err)
			return
		}

		observed := false
		if lastViewedAt != nil {
			viewedTime, err := time.Parse(sqliteDatetimeFormat, *lastViewedAt)
			if err == nil {
				observed = !viewedTime.Before(w.lastActivityTime)
			}
		}

		if !observed {
			ts := now.UTC().Format(sqliteDatetimeFormat)
			if err := w.db.SetUnreadSince(ctx, w.sessionID, &ts); err != nil {
				log.Printf("watcher %s: set unread: %v", w.sessionID, err)
			}
		}

		if err := w.db.TouchSession(ctx, w.sessionID, now.Unix()); err != nil {
			log.Printf("watcher %s: touch: %v", w.sessionID, err)
		}
	}
}

func (w *SessionWatcher) transitionTo(ctx context.Context, state SessionState, now time.Time) {
	prevState := w.currentState()
	w.setState(state)

	if state == StateDead {
		if err := w.db.SetUnreadSince(ctx, w.sessionID, nil); err != nil {
			log.Printf("watcher %s: clear unread on dead: %v", w.sessionID, err)
		}
	}

	if prevState != state && state != StateDead {
		if err := w.db.TouchSession(ctx, w.sessionID, now.Unix()); err != nil {
			log.Printf("watcher %s: touch: %v", w.sessionID, err)
		}
	}

	w.writeSnapshot()
}

func (w *SessionWatcher) writeSnapshot() {
	w.snapshot.Set(w.sessionID, SnapshotEntry{
		SessionName:  w.tmuxName,
		State:        w.currentState(),
		ProviderType: w.providerType,
	})
}

func (w *SessionWatcher) currentState() SessionState {
	w.stateMu.RLock()
	defer w.stateMu.RUnlock()
	return w.state
}

func (w *SessionWatcher) setState(s SessionState) {
	w.stateMu.Lock()
	w.state = s
	w.stateMu.Unlock()
}

func (w *SessionWatcher) String() string {
	return fmt.Sprintf("watcher[%s/%s state=%s]", w.sessionID, w.tmuxName, w.currentState())
}
