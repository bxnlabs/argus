package status

import (
	"context"
	"strings"
	"sync"
	"time"

	agentsession "github.com/bxnlabs/argus/internal/agent/session"
)

// SessionStatus is the detected status of a session.
type SessionStatus string

const (
	StatusRunning SessionStatus = "running"
	StatusWaiting SessionStatus = "waiting"
	StatusIdle    SessionStatus = "idle"
	StatusDead    SessionStatus = "dead"
)

// Configuration constants (match TypeScript implementation).
const (
	activityCooldownMS = 2000
	spikeWindowMS      = 1000
	sustainedThreshold = 2
	cacheValidityMS    = 2000
)

type stateTracker struct {
	lastChangeTime        int64
	lastActivityTimestamp int64
	spikeWindowStart     *int64
	spikeChangeCount     int
}

type sessionCache struct {
	data      map[string]int64
	updatedAt int64
}

// Detector detects session statuses by analyzing tmux pane content and activity.
type Detector struct {
	mu       sync.Mutex
	trackers map[string]*stateTracker
	cache    sessionCache
}

// NewDetector creates a new status detector.
func NewDetector() *Detector {
	return &Detector{
		trackers: make(map[string]*stateTracker),
		cache:    sessionCache{data: make(map[string]int64)},
	}
}

func (d *Detector) refreshCache(ctx context.Context) {
	now := time.Now().UnixMilli()
	if now-d.cache.updatedAt < cacheValidityMS {
		return
	}

	activities, err := agentsession.GetSessionActivitiesContext(ctx)
	if err != nil {
		return
	}

	newData := make(map[string]int64)
	for _, a := range activities {
		newData[a.Name] = a.Timestamp
	}
	d.cache = sessionCache{data: newData, updatedAt: now}
}

func (d *Detector) sessionExists(name string) bool {
	_, ok := d.cache.data[name]
	return ok
}

func (d *Detector) getTimestamp(name string) int64 {
	return d.cache.data[name]
}

func (d *Detector) getTracker(name string, timestamp int64) *stateTracker {
	t, ok := d.trackers[name]
	if !ok {
		now := time.Now().UnixMilli()
		t = &stateTracker{
			lastChangeTime:        now - activityCooldownMS,
			lastActivityTimestamp: timestamp,
		}
		d.trackers[name] = t
	}
	return t
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
	start := len(lines) - 5
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

func (d *Detector) processSpikeDetection(tracker *stateTracker, currentTimestamp int64) SessionStatus {
	now := time.Now().UnixMilli()
	timestampChanged := tracker.lastActivityTimestamp != currentTimestamp

	if timestampChanged {
		tracker.lastActivityTimestamp = currentTimestamp

		windowExpired := tracker.spikeWindowStart == nil ||
			now-*tracker.spikeWindowStart > spikeWindowMS

		if windowExpired {
			tracker.spikeWindowStart = &now
			tracker.spikeChangeCount = 1
		} else {
			tracker.spikeChangeCount++
			if tracker.spikeChangeCount >= sustainedThreshold {
				tracker.lastChangeTime = now
				tracker.spikeWindowStart = nil
				tracker.spikeChangeCount = 0
				return StatusRunning
			}
		}
	} else if tracker.spikeChangeCount == 1 && tracker.spikeWindowStart != nil {
		if now-*tracker.spikeWindowStart > spikeWindowMS {
			tracker.spikeWindowStart = nil
			tracker.spikeChangeCount = 0
		}
	}

	return ""
}

func (d *Detector) isInSpikeWindow(tracker *stateTracker) bool {
	return tracker.spikeWindowStart != nil &&
		time.Now().UnixMilli()-*tracker.spikeWindowStart < spikeWindowMS
}

func (d *Detector) isInCooldown(tracker *stateTracker) bool {
	return time.Now().UnixMilli()-tracker.lastChangeTime < activityCooldownMS
}

// GetStatus returns the detected status for a single session.
func (d *Detector) GetStatus(ctx context.Context, sessionName string) SessionStatus {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.refreshCache(ctx)

	if !d.sessionExists(sessionName) {
		delete(d.trackers, sessionName)
		return StatusDead
	}

	timestamp := d.getTimestamp(sessionName)
	tracker := d.getTracker(sessionName, timestamp)

	content, _ := agentsession.CapturePaneContext(ctx, sessionName)

	// 1. Busy indicators (highest priority)
	if checkBusyIndicators(content) {
		tracker.lastChangeTime = time.Now().UnixMilli()
		return StatusRunning
	}

	// 2. Waiting patterns
	if checkWaitingPatterns(content) {
		return StatusWaiting
	}

	// 3. Spike detection
	if spikeResult := d.processSpikeDetection(tracker, timestamp); spikeResult != "" {
		return spikeResult
	}

	// 4. During spike window, maintain stable status
	if d.isInSpikeWindow(tracker) {
		if d.isInCooldown(tracker) {
			return StatusRunning
		}
		return StatusIdle
	}

	// 5. Cooldown check
	if d.isInCooldown(tracker) {
		return StatusRunning
	}

	// 6. Cooldown expired
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

// Cleanup removes trackers for sessions that no longer exist.
func (d *Detector) Cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.refreshCache(context.Background())
	for name := range d.trackers {
		if !d.sessionExists(name) {
			delete(d.trackers, name)
		}
	}
}
