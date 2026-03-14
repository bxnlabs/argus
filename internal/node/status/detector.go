package status

import (
	"context"
	"strings"
	"sync"
	"time"

	nodesession "github.com/bxnlabs/argus/internal/node/session"
)

// SessionStatus is the detected status of a session.
type SessionStatus string

const (
	StatusRunning SessionStatus = "running"
	StatusWaiting SessionStatus = "waiting"
	StatusIdle    SessionStatus = "idle"
	StatusDead    SessionStatus = "dead"
)

// Configuration constants.
const (
	cacheValidityMS = 2000
)

// TmuxQuerier abstracts tmux interactions for testability.
type TmuxQuerier interface {
	GetSessionActivities(ctx context.Context) ([]nodesession.SessionActivity, error)
	CapturePaneContent(ctx context.Context, name string) (string, error)
}

// prodTmux is the production implementation of TmuxQuerier.
type prodTmux struct{}

func (prodTmux) GetSessionActivities(ctx context.Context) ([]nodesession.SessionActivity, error) {
	return nodesession.GetSessionActivitiesContext(ctx)
}

func (prodTmux) CapturePaneContent(ctx context.Context, name string) (string, error) {
	return nodesession.CapturePaneContext(ctx, name)
}

// sessionCache tracks which tmux sessions exist.
type sessionCache struct {
	names     map[string]struct{}
	updatedAt int64
}

// Detector detects session statuses by analyzing tmux pane content.
//
// tmux activity timestamps (session_activity, window_activity) are
// intentionally NOT used — they report spurious changes when a client
// attaches or the terminal resizes, causing the TUI to re-render
// without any real agent activity.
type Detector struct {
	mu    sync.Mutex
	cache sessionCache
	tmux  TmuxQuerier
	nowFn func() int64 // returns current time in unix millis; overridable for tests
}

// NewDetector creates a new status detector.
func NewDetector() *Detector {
	return newDetector(prodTmux{})
}

func newDetector(tmux TmuxQuerier) *Detector {
	return &Detector{
		cache: sessionCache{names: make(map[string]struct{})},
		tmux:  tmux,
		nowFn: func() int64 { return time.Now().UnixMilli() },
	}
}

func (d *Detector) now() int64 {
	return d.nowFn()
}

func (d *Detector) refreshCache(ctx context.Context) {
	now := d.now()
	if now-d.cache.updatedAt < cacheValidityMS {
		return
	}

	activities, err := d.tmux.GetSessionActivities(ctx)
	if err != nil {
		return
	}

	names := make(map[string]struct{}, len(activities))
	for _, a := range activities {
		names[a.Name] = struct{}{}
	}
	d.cache = sessionCache{names: names, updatedAt: now}
}

func (d *Detector) sessionExists(name string) bool {
	_, ok := d.cache.names[name]
	return ok
}

// checkBusyIndicators checks if terminal content shows busy indicators.
func checkBusyIndicators(content string) bool {
	lines := strings.Split(content, "\n")
	// Focus on last 10 lines
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}
	recentContent := strings.ToLower(strings.Join(lines[start:], "\n"))

	// Check text indicators
	for _, ind := range BusyIndicators {
		if strings.Contains(recentContent, ind) {
			return true
		}
	}

	// Check for Claude Code status line pattern: <word>… (e.g. "✶ Tinkering…")
	if StatusLinePattern.MatchString(recentContent) {
		return true
	}

	// Check spinners in last 5 lines
	spinnerStart := len(lines) - 5
	if spinnerStart < 0 {
		spinnerStart = 0
	}
	last5 := strings.Join(lines[spinnerStart:], "")
	for _, s := range SpinnerChars {
		if strings.Contains(last5, s) {
			return true
		}
	}

	return false
}

// checkWaitingPatterns checks if terminal content shows waiting patterns.
func checkWaitingPatterns(content string) bool {
	lines := strings.Split(content, "\n")
	// Use last 15 lines — plan approval prompts span 12+ lines on narrow
	// terminals (header + 4 numbered options + hints, each potentially wrapped).
	start := len(lines) - 15
	if start < 0 {
		start = 0
	}
	recentLines := strings.Join(lines[start:], "\n")

	for _, p := range WaitingPatterns {
		if p.MatchString(recentLines) {
			return true
		}
	}
	return false
}

// GetStatus returns the detected status for a single session.
//
// Detection is purely content-based:
//  1. Session not in tmux → dead
//  2. Capture pane content:
//     - Busy indicators match → running
//     - Waiting patterns match → waiting
//     - Nothing matches → idle
//  3. Capture error → idle (self-corrects on next poll)
func (d *Detector) GetStatus(ctx context.Context, sessionName string) SessionStatus {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.refreshCache(ctx)

	if !d.sessionExists(sessionName) {
		return StatusDead
	}

	content, err := d.tmux.CapturePaneContent(ctx, sessionName)
	if err != nil {
		return StatusIdle
	}

	if checkBusyIndicators(content) {
		return StatusRunning
	}

	if checkWaitingPatterns(content) {
		return StatusWaiting
	}

	return StatusIdle
}

// GetAllStatuses returns statuses for all given session names.
// It uses the provided context for cancellation and applies a 15-second timeout.
func (d *Detector) GetAllStatuses(ctx context.Context, names []string) map[string]SessionStatus {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	results := make(map[string]SessionStatus, len(names))
	for _, name := range names {
		if ctx.Err() != nil {
			// Don't report unchecked sessions as dead — omit them so the
			// caller can apply a safe fallback (e.g. keep previous status).
			continue
		}
		results[name] = d.GetStatus(ctx, name)
	}
	return results
}

