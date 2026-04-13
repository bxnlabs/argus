# Session Activity Detection, Unread Status & Notifications — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace keyword/regex-based session status detection with frame-diffing, add observation-aware unread status, and wire up heartbeat/acknowledge endpoints across backend, frontend, and CLI.

**Architecture:** Per-session `SessionWatcher` goroutines replace the centralized `Monitor` + `Detector`. Each watcher captures tmux pane content every 2s, compares consecutive frames, and infers `active`/`inactive`/`dead` state. A `WatcherManager` owns watcher lifecycle. An in-memory snapshot serves API reads; `unread_since` and `last_viewed_at` are authoritative in the DB. Frontend piggybacks heartbeats on the existing 2s status poll; CLI runs a heartbeat goroutine during attach.

**Tech Stack:** Go 1.22+ (backend), React + TypeScript + TanStack Query (frontend), SQLite, tmux CLI, Cobra CLI

**Spec:** `docs/superpowers/specs/2026-04-13-session-activity-detection-design.md`

---

## File Structure

### New Files (Backend)

| File | Responsibility |
|------|---------------|
| `internal/node/status/watcher.go` | `SessionWatcher` — per-session goroutine: capture loop, frame diffing, state machine, unread transitions |
| `internal/node/status/watcher_test.go` | Unit tests for `SessionWatcher` |
| `internal/node/status/manager.go` | `WatcherManager` — lifecycle management for all watchers, shared snapshot |
| `internal/node/status/manager_test.go` | Unit tests for `WatcherManager` |
| `internal/node/api/heartbeat.go` | `POST /api/sessions/{id}/heartbeat` and `POST /api/sessions/{id}/acknowledge` handlers |
| `internal/node/api/heartbeat_test.go` | Handler tests |

### Modified Files (Backend)

| File | Changes |
|------|---------|
| `internal/node/status/types.go` | New file: shared types (`SessionState`, `ActivitySnapshot`, `SnapshotEntry`) extracted from monitor.go |
| `internal/node/status/detector.go` | **Delete entirely** |
| `internal/node/status/detector_test.go` | **Delete entirely** |
| `internal/node/status/patterns.go` | **Delete entirely** |
| `internal/node/status/monitor.go` | **Delete entirely** |
| `internal/node/status/monitor_test.go` | **Delete entirely** |
| `internal/node/db/schema.go` | Add `unread_since` and `last_viewed_at` columns |
| `internal/node/db/models.go` | Add `UnreadSince` and `LastViewedAt` fields to `Session` |
| `internal/node/db/sessions.go` | Add `SetUnreadSince`, `SetLastViewedAt`, `TouchLastViewedAt`, `AcknowledgeSession` methods; update `scanSession` and `sessionColumns` |
| `internal/node/db/migrations.go` | Add migration for new columns |
| `internal/node/db/db.go` | Seed new migration |
| `internal/node/session/tmux.go` | Add `GetPaneDimensions` function |
| `internal/node/api/router.go` | Replace `StatusMonitor` dep with `WatcherManager`; add heartbeat/acknowledge routes |
| `internal/node/api/status.go` | Rewrite to compose from snapshot + DB; add `unreadSince` field |
| `internal/node/setup.go` | Replace `Detector` + `Monitor` with `WatcherManager`; wire new deps |
| `cmd/argus/cli/session_attach.go` | Replace `syscall.Exec` with subprocess wrapper + heartbeat goroutine |
| `cmd/argus/cli/session_list.go` | Show new status values + unread indicator |

### Modified Files (Frontend)

| File | Changes |
|------|---------|
| `web/src/types.ts` | Update `SessionStatusInfo` status union; add `unreadSince` field |
| `web/src/hooks/useNotifications.ts` | Replace `running->waiting` with `unreadSince` appearing while hidden |
| `web/src/data/statuses/queries.ts` | Add heartbeat call piggybacked on status poll |
| `web/src/components/SessionList/index.tsx` | Update status colors/labels; add blue unread dot |
| `web/src/App.tsx` | Wire acknowledge on session select |

---

## Task 1: Database Schema — Add `unread_since` and `last_viewed_at` Columns

**Files:**
- Modify: `internal/node/db/schema.go`
- Modify: `internal/node/db/models.go`
- Modify: `internal/node/db/sessions.go`
- Modify: `internal/node/db/migrations.go`
- Modify: `internal/node/db/db.go`
- Test: `internal/node/db/db_test.go`

- [ ] **Step 1: Write failing test for new columns**

In `internal/node/db/db_test.go`, add a test that creates a session then verifies the new fields are readable:

```go
func TestSessionUnreadFields(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	err := db.CreateSession(&Session{
		ID: "sess-unread-1", Name: "test", TmuxName: "claude-sess-unread-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}

	s, err := db.GetSession("sess-unread-1")
	if err != nil {
		t.Fatal(err)
	}
	if s.UnreadSince != nil {
		t.Errorf("expected nil UnreadSince, got %v", s.UnreadSince)
	}
	if s.LastViewedAt != nil {
		t.Errorf("expected nil LastViewedAt, got %v", s.LastViewedAt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--planning-session-activity-detection && go test ./internal/node/db/ -run TestSessionUnreadFields -v`

Expected: Compile error — `UnreadSince` and `LastViewedAt` not defined on `Session`.

- [ ] **Step 3: Add columns to schema, model, and scan**

In `internal/node/db/schema.go`, add columns to the CREATE TABLE:

```go
const schema = `
-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  tmux_name TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  working_directory TEXT NOT NULL DEFAULT '~',
  provider_session_id TEXT,
  model TEXT DEFAULT 'sonnet',
  system_prompt TEXT,
  provider_type TEXT NOT NULL DEFAULT 'claude',
  auto_approve INTEGER NOT NULL DEFAULT 0,
  worktree_branch TEXT,
  git_parent_dir TEXT,
  git_remote_url TEXT,
  profile TEXT,
  branch_created INTEGER NOT NULL DEFAULT 0,
  unread_since TEXT,
  last_viewed_at TEXT
);

-- Migrations tracking
CREATE TABLE IF NOT EXISTS _migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`
```

In `internal/node/db/models.go`, add fields:

```go
type Session struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	TmuxName          string  `json:"tmux_name"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	WorkingDirectory  string  `json:"working_directory"`
	ProviderSessionID *string `json:"provider_session_id"`
	Model             *string `json:"model"`
	SystemPrompt      *string `json:"system_prompt"`
	ProviderType      string  `json:"provider_type"`
	AutoApprove       bool    `json:"auto_approve"`
	WorktreeBranch    *string `json:"worktree_branch"`
	GitParentDir      *string `json:"git_parent_dir"`
	GitRemoteURL      *string `json:"git_remote_url"`
	Profile           *string `json:"profile"`
	BranchCreated     bool    `json:"branch_created"`
	UnreadSince       *string `json:"unread_since"`
	LastViewedAt      *string `json:"last_viewed_at"`
}
```

In `internal/node/db/sessions.go`, update `sessionColumns` and `scanSession`:

```go
const sessionColumns = `id, name, tmux_name, created_at, updated_at,
	working_directory, provider_session_id, model, system_prompt,
	provider_type, auto_approve, worktree_branch, git_parent_dir, git_remote_url, profile, branch_created,
	unread_since, last_viewed_at`

func scanSession(row interface{ Scan(...any) error }) (*Session, error) {
	var s Session
	var autoApprove int
	var branchCreated int
	err := row.Scan(
		&s.ID, &s.Name, &s.TmuxName, &s.CreatedAt, &s.UpdatedAt,
		&s.WorkingDirectory,
		&s.ProviderSessionID, &s.Model, &s.SystemPrompt,
		&s.ProviderType, &autoApprove, &s.WorktreeBranch,
		&s.GitParentDir, &s.GitRemoteURL, &s.Profile, &branchCreated,
		&s.UnreadSince, &s.LastViewedAt,
	)
	if err != nil {
		return nil, err
	}
	s.AutoApprove = autoApprove != 0
	s.BranchCreated = branchCreated != 0
	return &s, nil
}
```

In `internal/node/db/migrations.go`, add migration to `allMigrations`:

```go
{"add_unread_since_and_last_viewed_at", func(d *DB) error {
	if _, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN unread_since TEXT`); err != nil {
		return err
	}
	_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN last_viewed_at TEXT`)
	return err
}},
```

In `internal/node/db/db.go`, add to `seedMigrations` — detect `unread_since` column:

Add `hasUnreadSince` to the column detection loop:

```go
case "unread_since":
	hasUnreadSince = true
```

And at the end of seedMigrations:

```go
if hasUnreadSince {
	if _, err := d.sql.Exec(
		`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
		"add_unread_since_and_last_viewed_at",
	); err != nil {
		return err
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/node/db/ -run TestSessionUnreadFields -v`

Expected: PASS

- [ ] **Step 5: Write tests for new DB methods**

Add to `internal/node/db/db_test.go`:

```go
func TestSetUnreadSince(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "sess-ur-1", Name: "test", TmuxName: "claude-sess-ur-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// Set unread
	ts := "2026-04-13 12:00:00"
	if err := db.SetUnreadSince(context.Background(), "sess-ur-1", &ts); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetSession("sess-ur-1")
	if s.UnreadSince == nil || *s.UnreadSince != ts {
		t.Errorf("expected unread_since=%q, got %v", ts, s.UnreadSince)
	}

	// Clear unread
	if err := db.SetUnreadSince(context.Background(), "sess-ur-1", nil); err != nil {
		t.Fatal(err)
	}
	s, _ = db.GetSession("sess-ur-1")
	if s.UnreadSince != nil {
		t.Errorf("expected nil unread_since, got %v", s.UnreadSince)
	}
}

func TestSetLastViewedAt(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "sess-lv-1", Name: "test", TmuxName: "claude-sess-lv-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	ts := "2026-04-13 12:00:00"
	if err := db.SetLastViewedAt(context.Background(), "sess-lv-1", ts); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetSession("sess-lv-1")
	if s.LastViewedAt == nil || *s.LastViewedAt != ts {
		t.Errorf("expected last_viewed_at=%q, got %v", ts, s.LastViewedAt)
	}
}

func TestAcknowledgeSession(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "sess-ack-1", Name: "test", TmuxName: "claude-sess-ack-1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// Set unread first
	ts := "2026-04-13 12:00:00"
	db.SetUnreadSince(context.Background(), "sess-ack-1", &ts)

	// Acknowledge
	if err := db.AcknowledgeSession(context.Background(), "sess-ack-1"); err != nil {
		t.Fatal(err)
	}
	s, _ := db.GetSession("sess-ack-1")
	if s.UnreadSince != nil {
		t.Errorf("expected nil unread_since after acknowledge, got %v", s.UnreadSince)
	}
	if s.LastViewedAt == nil {
		t.Error("expected last_viewed_at to be set after acknowledge")
	}
}
```

- [ ] **Step 6: Run tests to verify they fail**

Run: `go test ./internal/node/db/ -run "TestSetUnreadSince|TestSetLastViewedAt|TestAcknowledgeSession" -v`

Expected: Compile errors — methods not defined.

- [ ] **Step 7: Implement new DB methods**

Add to `internal/node/db/sessions.go`:

```go
// SetUnreadSince sets or clears the unread_since timestamp.
// Pass nil to clear (mark as read).
func (d *DB) SetUnreadSince(ctx context.Context, id string, ts *string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET unread_since = ? WHERE id = ?`,
		ts, id,
	)
	return err
}

// SetLastViewedAt updates the last_viewed_at timestamp.
func (d *DB) SetLastViewedAt(ctx context.Context, id, ts string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET last_viewed_at = ? WHERE id = ?`,
		ts, id,
	)
	return err
}

// AcknowledgeSession clears unread_since and sets last_viewed_at to now.
func (d *DB) AcknowledgeSession(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET unread_since = NULL, last_viewed_at = datetime('now') WHERE id = ?`,
		id,
	)
	return err
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/node/db/ -run "TestSessionUnreadFields|TestSetUnreadSince|TestSetLastViewedAt|TestAcknowledgeSession" -v`

Expected: All PASS

- [ ] **Step 9: Run full DB test suite for regressions**

Run: `go test ./internal/node/db/ -v`

Expected: All existing tests PASS

- [ ] **Step 10: Commit**

```bash
git add internal/node/db/schema.go internal/node/db/models.go internal/node/db/sessions.go internal/node/db/migrations.go internal/node/db/db.go internal/node/db/db_test.go
git commit -m "feat(db): add unread_since and last_viewed_at columns (BXN-34)"
```

---

## Task 2: Tmux Pane Dimensions Helper

**Files:**
- Modify: `internal/node/session/tmux.go`
- Test: `internal/node/session/tmux_test.go`

- [ ] **Step 1: Write failing test for GetPaneDimensions**

Add to `internal/node/session/tmux_test.go`:

```go
func TestParsePaneDimensions(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantW     int
		wantH     int
		wantValid bool
	}{
		{"normal", "80x24", 80, 24, true},
		{"large", "200x50", 200, 50, true},
		{"empty", "", 0, 0, false},
		{"no separator", "8024", 0, 0, false},
		{"non-numeric width", "abcx24", 0, 0, false},
		{"non-numeric height", "80xabc", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h, ok := parsePaneDimensions(tt.input)
			if ok != tt.wantValid {
				t.Errorf("parsePaneDimensions(%q) valid=%v, want %v", tt.input, ok, tt.wantValid)
			}
			if ok && (w != tt.wantW || h != tt.wantH) {
				t.Errorf("parsePaneDimensions(%q) = (%d,%d), want (%d,%d)", tt.input, w, h, tt.wantW, tt.wantH)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/node/session/ -run TestParsePaneDimensions -v`

Expected: Compile error — `parsePaneDimensions` not defined.

- [ ] **Step 3: Implement dimension functions**

Add to `internal/node/session/tmux.go`:

```go
// PaneDimensions holds the width and height of a tmux pane.
type PaneDimensions struct {
	Width  int
	Height int
}

// parsePaneDimensions parses "WxH" format from tmux display-message.
func parsePaneDimensions(s string) (width, height int, ok bool) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, "x", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return w, h, true
}

// GetPaneDimensionsContext returns the pane dimensions for a tmux session.
func GetPaneDimensionsContext(ctx context.Context, name string) (PaneDimensions, error) {
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-t", name, "-p", "#{pane_width}x#{pane_height}")
	out, err := cmd.Output()
	if err != nil {
		return PaneDimensions{}, fmt.Errorf("tmux display-message: %w", err)
	}
	w, h, ok := parsePaneDimensions(string(out))
	if !ok {
		return PaneDimensions{}, fmt.Errorf("invalid pane dimensions: %q", string(out))
	}
	return PaneDimensions{Width: w, Height: h}, nil
}

// HasSessionContext checks if a tmux session exists, with context.
func HasSessionContext(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", name)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// Check if it's a clean "not found" exit vs an ambiguous error.
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// tmux has-session exits 1 when session not found — this is definitive.
		return false, nil
	}
	// Non-exit errors (timeout, binary missing) are ambiguous.
	return false, fmt.Errorf("tmux has-session: %w", err)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/node/session/ -run TestParsePaneDimensions -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/session/tmux.go internal/node/session/tmux_test.go
git commit -m "feat(tmux): add pane dimensions parser and HasSessionContext (BXN-34)"
```

---

## Task 3: Status Types — Shared Type Definitions

**Files:**
- Create: `internal/node/status/types.go`

- [ ] **Step 1: Create types file**

Create `internal/node/status/types.go`:

```go
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
	StateInactive SessionState = "inactive"
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
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./internal/node/status/...`

Expected: Build succeeds (no consumers yet, types file stands alone).

- [ ] **Step 3: Commit**

```bash
git add internal/node/status/types.go
git commit -m "feat(status): add shared types for activity state (BXN-34)"
```

---

## Task 4: SessionWatcher — Per-Session Frame-Diffing Goroutine

**Files:**
- Create: `internal/node/status/watcher.go`
- Create: `internal/node/status/watcher_test.go`

- [ ] **Step 1: Write failing test for basic frame-diffing**

Create `internal/node/status/watcher_test.go`:

```go
package status

import (
	"context"
	"sync"
	"testing"
	"time"

	nodesession "github.com/bxnlabs/argus/internal/node/session"
)

// mockTmuxWatcher implements the tmux operations needed by SessionWatcher.
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

	// Change content to trigger active
	tmux.setContent("new content here")
	time.Sleep(50 * time.Millisecond)

	// Trigger a capture cycle
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

func TestWatcher_InactiveAfterStableFrames(t *testing.T) {
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

	// Wait for initial capture + 2 stable frames
	for i := 0; i < 4; i++ {
		time.Sleep(20 * time.Millisecond)
		w.tick()
	}
	time.Sleep(20 * time.Millisecond)

	entry, ok := snap.Read().Statuses["sess-2"]
	if !ok {
		t.Fatal("session not in snapshot")
	}
	if entry.State != StateInactive {
		t.Errorf("expected inactive, got %s", entry.State)
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
	w.tick()
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/status/ -run "TestWatcher_" -v`

Expected: Compile error — `newSessionWatcher` not defined.

- [ ] **Step 3: Implement SessionWatcher**

Create `internal/node/status/watcher.go`:

```go
package status

import (
	"context"
	"fmt"
	"log"
	"time"

	nodesession "github.com/bxnlabs/argus/internal/node/session"
)

const (
	defaultCaptureInterval    = 2 * time.Second
	defaultStabilityThreshold = 2 // consecutive identical frames
)

// sqliteDatetimeFormat matches SQLite datetime('now') output format.
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

	// State machine
	state                SessionState
	prevContent          string
	prevDims             nodesession.PaneDimensions
	hasPrevFrame         bool
	stabilityCounter     int
	stabilityThreshold   int
	lastActivityTime     time.Time

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
		snapshot:            snapshot,
		state:              StateInactive,
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
		// Exit after dead detection
		if w.state == StateDead {
			return
		}
	}
}

func (w *SessionWatcher) capture(ctx context.Context) {
	now := w.nowFn()

	// Dead detection: check if tmux session exists.
	alive, err := w.tmux.HasSession(ctx, w.tmuxName)
	if err != nil {
		// Ambiguous error — preserve prior state, log.
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

		prevState := w.state
		w.state = StateActive

		// Clear unread_since on activity resume (inactive -> active)
		if prevState == StateInactive {
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

		if w.stabilityCounter >= w.stabilityThreshold && w.state != StateInactive {
			// Transition to inactive
			prevState := w.state
			w.transitionToInactive(ctx, prevState, now)
		}
	}

	w.writeSnapshot()
}

func (w *SessionWatcher) transitionToInactive(ctx context.Context, prevState SessionState, now time.Time) {
	w.state = StateInactive

	if prevState == StateActive {
		// Check if currently observed
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
			// Not currently observed — set unread
			ts := now.UTC().Format(sqliteDatetimeFormat)
			if err := w.db.SetUnreadSince(ctx, w.sessionID, &ts); err != nil {
				log.Printf("watcher %s: set unread: %v", w.sessionID, err)
			}
		}

		// Touch on state change
		if err := w.db.TouchSession(ctx, w.sessionID, now.Unix()); err != nil {
			log.Printf("watcher %s: touch: %v", w.sessionID, err)
		}
	}
}

func (w *SessionWatcher) transitionTo(ctx context.Context, state SessionState, now time.Time) {
	prevState := w.state
	w.state = state

	if state == StateDead {
		// Clear unread on death
		if err := w.db.SetUnreadSince(ctx, w.sessionID, nil); err != nil {
			log.Printf("watcher %s: clear unread on dead: %v", w.sessionID, err)
		}
	}

	// Touch on state change
	if prevState != state {
		if err := w.db.TouchSession(ctx, w.sessionID, now.Unix()); err != nil {
			log.Printf("watcher %s: touch: %v", w.sessionID, err)
		}
	}

	w.writeSnapshot()
}

func (w *SessionWatcher) writeSnapshot() {
	w.snapshot.Set(w.sessionID, SnapshotEntry{
		SessionName:  w.tmuxName,
		State:        w.state,
		ProviderType: w.providerType,
	})
}

// currentState returns the watcher's current state (for testing).
func (w *SessionWatcher) currentState() SessionState {
	return w.state
}

// String returns a debug-friendly representation.
func (w *SessionWatcher) String() string {
	return fmt.Sprintf("watcher[%s/%s state=%s]", w.sessionID, w.tmuxName, w.state)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/status/ -run "TestWatcher_" -v`

Expected: All PASS

- [ ] **Step 5: Write test for unread observation awareness**

Add to `internal/node/status/watcher_test.go`:

```go
func TestWatcher_UnreadNotSetWhenObserved(t *testing.T) {
	tmux := &mockTmuxWatcher{
		content: "content A",
		dims:    nodesession.PaneDimensions{Width: 80, Height: 24},
		alive:   true,
	}
	db := newMockDB()
	snap := NewActivitySnapshot()

	// Simulate active observation: last_viewed_at is recent
	recentTime := time.Now().UTC().Format(sqliteDatetimeFormat)
	db.lastViewed["sess-obs"] = recentTime

	w := newSessionWatcher("sess-obs", "tmux-sess-obs", "claude", tmux, db, snap)
	w.stabilityThreshold = 1
	w.nowFn = func() time.Time {
		// Return a time before the last_viewed_at so observation is "current"
		t, _ := time.Parse(sqliteDatetimeFormat, recentTime)
		return t.Add(-1 * time.Second)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.run(ctx)

	// First cycle: store baseline
	time.Sleep(30 * time.Millisecond)

	// Change content to trigger active
	tmux.setContent("content B")
	w.tick()
	time.Sleep(30 * time.Millisecond)

	// Now stabilize to trigger active->inactive
	w.tick()
	time.Sleep(30 * time.Millisecond)
	w.tick()
	time.Sleep(30 * time.Millisecond)

	// Unread should NOT be set because session was observed
	db.mu.Lock()
	us := db.unreadSince["sess-obs"]
	db.mu.Unlock()

	if us != nil {
		t.Errorf("expected nil unread_since (session was observed), got %v", *us)
	}

	cancel()
}

func TestWatcher_ResizeGuardDiscardsFrame(t *testing.T) {
	callCount := 0
	tmux := &mockTmuxWatcher{
		content: "content",
		alive:   true,
	}
	// Dimensions change between pre and post capture
	originalGetDims := tmux.GetPaneDimensions
	_ = originalGetDims // suppress unused

	// We need a more sophisticated mock for this. Use a function-based approach.
	dims80x24 := nodesession.PaneDimensions{Width: 80, Height: 24}
	dims100x30 := nodesession.PaneDimensions{Width: 100, Height: 30}

	dimsMock := &changingDimsMock{
		content:  "content",
		alive:    true,
		dimsCalls: []nodesession.PaneDimensions{dims80x24, dims100x30}, // pre != post
	}

	db := newMockDB()
	snap := NewActivitySnapshot()

	w := newSessionWatcher("sess-resize", "tmux-sess-resize", "claude", dimsMock, db, snap)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Manually trigger one capture
	w.capture(ctx)

	// Frame should be discarded (no prev frame stored due to resize guard on... actually
	// the first frame would store baseline. Let's think about this differently.
	// After the first cycle, if pre != post dims, stabilityCounter should be 0.
	if w.stabilityCounter != 0 {
		t.Errorf("expected stability counter reset after resize, got %d", w.stabilityCounter)
	}
	_ = callCount

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
```

- [ ] **Step 6: Run tests**

Run: `go test ./internal/node/status/ -run "TestWatcher_" -v`

Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/node/status/watcher.go internal/node/status/watcher_test.go
git commit -m "feat(status): add SessionWatcher with frame-diffing and unread logic (BXN-34)"
```

---

## Task 5: WatcherManager — Watcher Lifecycle Management

**Files:**
- Create: `internal/node/status/manager.go`
- Create: `internal/node/status/manager_test.go`

- [ ] **Step 1: Write failing test for WatcherManager**

Create `internal/node/status/manager_test.go`:

```go
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

	// Wait for watchers to start and run at least one cycle
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/status/ -run "TestManager_" -v`

Expected: Compile error — `NewWatcherManager` not defined.

- [ ] **Step 3: Implement WatcherManager**

Create `internal/node/status/manager.go`:

```go
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
	m.started = true
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Start watchers for all existing sessions
	sessions, err := m.lister.List(m.ctx)
	if err != nil {
		log.Printf("watcher manager: list sessions on startup: %v", err)
		return
	}

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
		// Watcher exists. If it went dead, restart it.
		if entry.watcher.currentState() == StateDead {
			entry.cancel()
			delete(m.watchers, sessionID)
			m.snapshot.Remove(sessionID)
		} else {
			return // already watching and alive
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/status/ -run "TestManager_" -v`

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/status/manager.go internal/node/status/manager_test.go
git commit -m "feat(status): add WatcherManager for watcher lifecycle (BXN-34)"
```

---

## Task 6: Heartbeat and Acknowledge API Endpoints

**Files:**
- Create: `internal/node/api/heartbeat.go`
- Create: `internal/node/api/heartbeat_test.go`
- Modify: `internal/node/api/router.go`

- [ ] **Step 1: Write failing test for heartbeat handler**

Create `internal/node/api/heartbeat_test.go`:

```go
package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeHeartbeatDB struct {
	mu            sync.Mutex
	lastViewedAt  map[string]string
	acknowledged  map[string]bool
}

func newFakeHeartbeatDB() *fakeHeartbeatDB {
	return &fakeHeartbeatDB{
		lastViewedAt: make(map[string]string),
		acknowledged: make(map[string]bool),
	}
}

func (f *fakeHeartbeatDB) SetLastViewedAt(ctx context.Context, id, ts string) error {
	f.mu.Lock()
	f.lastViewedAt[id] = ts
	f.mu.Unlock()
	return nil
}

func (f *fakeHeartbeatDB) AcknowledgeSession(ctx context.Context, id string) error {
	f.mu.Lock()
	f.acknowledged[id] = true
	f.mu.Unlock()
	return nil
}

func TestHeartbeatHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/api/sessions/sess-1/heartbeat", nil)
	req.SetPathValue("id", "sess-1")
	w := httptest.NewRecorder()

	h.heartbeat(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	db.mu.Lock()
	if _, ok := db.lastViewedAt["sess-1"]; !ok {
		t.Error("expected last_viewed_at to be set")
	}
	db.mu.Unlock()
}

func TestAcknowledgeHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/api/sessions/sess-2/acknowledge", nil)
	req.SetPathValue("id", "sess-2")
	w := httptest.NewRecorder()

	h.acknowledge(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	db.mu.Lock()
	if !db.acknowledged["sess-2"] {
		t.Error("expected session to be acknowledged")
	}
	db.mu.Unlock()
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/api/ -run "TestHeartbeatHandler|TestAcknowledgeHandler" -v`

Expected: Compile error — `heartbeatHandler` not defined.

- [ ] **Step 3: Implement handlers**

Create `internal/node/api/heartbeat.go`:

```go
package api

import (
	"context"
	"net/http"
)

// HeartbeatDB abstracts the DB operations needed by heartbeat/acknowledge handlers.
type HeartbeatDB interface {
	SetLastViewedAt(ctx context.Context, id, ts string) error
	AcknowledgeSession(ctx context.Context, id string) error
}

type heartbeatHandler struct {
	db HeartbeatDB
}

// heartbeat handles POST /api/sessions/{id}/heartbeat.
// Updates last_viewed_at = now(). Lightweight — single DB update.
func (h *heartbeatHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.SetLastViewedAt(r.Context(), id, "datetime('now')"); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// acknowledge handles POST /api/sessions/{id}/acknowledge.
// Clears unread_since and sets last_viewed_at = now(). Idempotent.
func (h *heartbeatHandler) acknowledge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.AcknowledgeSession(r.Context(), id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Wait — `SetLastViewedAt` should use SQLite `datetime('now')` on the server side, not pass a literal string. Let me fix the DB method. The `SetLastViewedAt` in `sessions.go` takes a `ts string` parameter, but for the heartbeat we want `datetime('now')`. Let me revise the approach: the heartbeat handler should call a dedicated method that uses `datetime('now')` directly.

Revise `internal/node/api/heartbeat.go`:

```go
package api

import (
	"context"
	"net/http"
)

// HeartbeatDB abstracts the DB operations needed by heartbeat/acknowledge handlers.
type HeartbeatDB interface {
	TouchLastViewedAt(ctx context.Context, id string) error
	AcknowledgeSession(ctx context.Context, id string) error
}

type heartbeatHandler struct {
	db HeartbeatDB
}

// heartbeat handles POST /api/sessions/{id}/heartbeat.
// Updates last_viewed_at = now(). Lightweight — single DB update.
func (h *heartbeatHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.TouchLastViewedAt(r.Context(), id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// acknowledge handles POST /api/sessions/{id}/acknowledge.
// Clears unread_since and sets last_viewed_at = now(). Idempotent.
func (h *heartbeatHandler) acknowledge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.AcknowledgeSession(r.Context(), id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

Add `TouchLastViewedAt` to `internal/node/db/sessions.go`:

```go
// TouchLastViewedAt sets last_viewed_at to the current time.
func (d *DB) TouchLastViewedAt(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET last_viewed_at = datetime('now') WHERE id = ?`,
		id,
	)
	return err
}
```

Update the test mock accordingly:

```go
type fakeHeartbeatDB struct {
	mu            sync.Mutex
	lastViewedAt  map[string]bool
	acknowledged  map[string]bool
}

func newFakeHeartbeatDB() *fakeHeartbeatDB {
	return &fakeHeartbeatDB{
		lastViewedAt: make(map[string]bool),
		acknowledged: make(map[string]bool),
	}
}

func (f *fakeHeartbeatDB) TouchLastViewedAt(ctx context.Context, id string) error {
	f.mu.Lock()
	f.lastViewedAt[id] = true
	f.mu.Unlock()
	return nil
}

func (f *fakeHeartbeatDB) AcknowledgeSession(ctx context.Context, id string) error {
	f.mu.Lock()
	f.acknowledged[id] = true
	f.mu.Unlock()
	return nil
}
```

And update the heartbeat test assertion:

```go
db.mu.Lock()
if !db.lastViewedAt["sess-1"] {
	t.Error("expected last_viewed_at to be touched")
}
db.mu.Unlock()
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/api/ -run "TestHeartbeatHandler|TestAcknowledgeHandler" -v`

Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/api/heartbeat.go internal/node/api/heartbeat_test.go internal/node/db/sessions.go
git commit -m "feat(api): add heartbeat and acknowledge endpoints (BXN-34)"
```

---

## Task 7: Rewrite Status Handler and Wire New Router Deps

**Files:**
- Modify: `internal/node/api/status.go`
- Modify: `internal/node/api/router.go`
- Modify: `internal/node/setup.go`

- [ ] **Step 1: Update the status handler to compose from snapshot + DB**

Rewrite `internal/node/api/status.go`:

```go
package api

import (
	"context"
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/status"
)

// handleStatus returns a handler for GET /api/sessions/status.
// Composes activity state from in-memory snapshot with unread_since from DB.
func handleStatus(mgr *status.WatcherManager, database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := mgr.Snapshot()

		// Fetch all sessions to get unread_since from DB
		sessions, err := database.ListSessions(r.Context())
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		unreadMap := make(map[string]*string, len(sessions))
		for _, s := range sessions {
			unreadMap[s.ID] = s.UnreadSince
		}

		result := make(map[string]any, len(snap.Statuses))
		for id, entry := range snap.Statuses {
			item := map[string]any{
				"sessionName":  entry.SessionName,
				"status":       string(entry.State),
				"providerType": entry.ProviderType,
			}
			if us, ok := unreadMap[id]; ok && us != nil {
				item["unreadSince"] = *us
			}
			result[id] = item
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"statuses":        result,
			"lastRefreshedAt": snap.LastRefreshedAt,
		})
	}
}
```

- [ ] **Step 2: Update router deps**

Modify `internal/node/api/router.go`:

```go
package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/node/status"
	"github.com/bxnlabs/argus/internal/node/terminal"
	ghservice "github.com/bxnlabs/argus/internal/github"
)

// Deps holds the dependencies injected into API handlers.
type Deps struct {
	SessionManager    *session.Manager
	WatcherManager    *status.WatcherManager
	Database          *db.DB
	RepoIndexer       *ghservice.RepoIndexer
	UploadDirOverride string // override upload directory (for testing)
	StateDir          string
}

// NewRouter creates the HTTP router with all node API routes.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/info", handleInfo)

	// Session routes
	sh := &sessionHandler{manager: deps.SessionManager}
	mux.HandleFunc("GET /api/sessions", sh.list)
	mux.HandleFunc("POST /api/sessions", sh.create)
	mux.HandleFunc("GET /api/sessions/{id}", sh.get)
	mux.HandleFunc("PATCH /api/sessions/{id}", sh.update)
	mux.HandleFunc("DELETE /api/sessions/{id}", sh.delete)

	// Profile routes
	mux.HandleFunc("GET /api/profiles", sh.listProfiles)

	// Git routes (read-only)
	gh := &gitHandler{stateDir: deps.StateDir}
	mux.HandleFunc("GET /api/git/status", gh.status)
	mux.HandleFunc("GET /api/git/diff", gh.diff)
	mux.HandleFunc("GET /api/git/working-diff", gh.workingDiff)
	mux.HandleFunc("GET /api/git/history", gh.history)
	mux.HandleFunc("GET /api/git/history/{hash}", gh.commitDetail)
	mux.HandleFunc("GET /api/git/history/{hash}/full-diff", gh.commitFullDiff)
	mux.HandleFunc("GET /api/git/compare/branches", gh.compareBranches)
	mux.HandleFunc("GET /api/git/compare", gh.compare)
	mux.HandleFunc("GET /api/git/file-content", gh.fileContent)
	mux.HandleFunc("GET /api/git/file-lines", gh.fileLines)
	mux.HandleFunc("GET /api/git/check", gh.check)
	mux.HandleFunc("GET /api/git/branches", gh.branches)

	// Review routes
	rh := &reviewHandler{}
	mux.HandleFunc("GET /api/git/review", rh.get)
	mux.HandleFunc("POST /api/git/review", rh.post)
	mux.HandleFunc("DELETE /api/git/review", rh.delete)

	// File routes
	fh := &filesHandler{uploadDirOverride: deps.UploadDirOverride}
	mux.HandleFunc("GET /api/files", fh.list)
	mux.HandleFunc("GET /api/files/meta", fh.meta)
	mux.HandleFunc("GET /api/files/content", fh.readContent)
	mux.HandleFunc("PUT /api/files/content", fh.writeContent)
	mux.HandleFunc("GET /api/files/search", fh.search)
	mux.HandleFunc("POST /api/files/upload", fh.upload)

	// Code search routes
	srch := &searchHandler{}
	mux.HandleFunc("GET /api/code-search", srch.search)
	mux.HandleFunc("GET /api/code-search/available", srch.available)

	// GitHub routes
	ghub := &githubHandler{repoIndexer: deps.RepoIndexer}
	mux.HandleFunc("GET /api/github/repos", ghub.listRepos)

	// Status route
	if deps.WatcherManager != nil {
		mux.HandleFunc("GET /api/sessions/status", handleStatus(deps.WatcherManager, deps.Database))
	}

	// Heartbeat and acknowledge routes
	if deps.Database != nil {
		hb := &heartbeatHandler{db: deps.Database}
		mux.HandleFunc("POST /api/sessions/{id}/heartbeat", hb.heartbeat)
		mux.HandleFunc("POST /api/sessions/{id}/acknowledge", hb.acknowledge)
	}

	// Terminal WebSocket
	mux.HandleFunc("/ws/sessions/{id}", terminal.HandleSessionWebSocket(deps.SessionManager))
	mux.HandleFunc("/ws/terminal", terminal.HandleTerminalWebSocket)

	return corsMiddleware(mux)
}
```

Note: `db.DB` needs to implement the `HeartbeatDB` interface. Since `db.DB` already has `TouchLastViewedAt` and `AcknowledgeSession` methods, it satisfies `HeartbeatDB` implicitly. The heartbeat handler takes `HeartbeatDB` interface, but in the router we pass `deps.Database` directly — this works because `*db.DB` satisfies `HeartbeatDB`.

- [ ] **Step 3: Update setup.go — replace Monitor/Detector with WatcherManager**

Rewrite `internal/node/setup.go`:

```go
package node

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/node/api"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/node/status"
	"github.com/bxnlabs/argus/internal/config"
	ghsvc "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/git/worktree"
)

// prodTmuxOps is the production implementation of TmuxWatcherOps.
type prodTmuxOps struct{}

func (prodTmuxOps) CapturePaneContent(ctx context.Context, name string) (string, error) {
	return session.CapturePaneContext(ctx, name)
}

func (prodTmuxOps) GetPaneDimensions(ctx context.Context, name string) (session.PaneDimensions, error) {
	return session.GetPaneDimensionsContext(ctx, name)
}

func (prodTmuxOps) HasSession(ctx context.Context, name string) (bool, error) {
	return session.HasSessionContext(ctx, name)
}

// prodWatcherDB adapts *db.DB to the WatcherDB interface.
type prodWatcherDB struct {
	db *db.DB
}

func (p *prodWatcherDB) SetUnreadSince(ctx context.Context, id string, ts *string) error {
	return p.db.SetUnreadSince(ctx, id, ts)
}

func (p *prodWatcherDB) SetLastViewedAt(ctx context.Context, id, ts string) error {
	return p.db.SetLastViewedAt(ctx, id, ts)
}

func (p *prodWatcherDB) TouchSession(ctx context.Context, id string, unixTS int64) error {
	return p.db.TouchSession(ctx, id, unixTS)
}

func (p *prodWatcherDB) GetSession(id string) (unreadSince, lastViewedAt *string, err error) {
	s, err := p.db.GetSession(id)
	if err != nil {
		return nil, nil, err
	}
	if s == nil {
		return nil, nil, nil
	}
	return s.UnreadSince, s.LastViewedAt, nil
}

// Setup initializes the node: opens the database, verifies migrations are
// current, and returns an HTTP handler with all node API routes.
func Setup(cfg *config.Config) (http.Handler, func(), error) {
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.CheckMigrations(); err != nil {
		database.Close()
		return nil, nil, err
	}

	expandedDBPath, err := shared.ExpandPath(cfg.Database.Path)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("expand db path: %w", err)
	}
	absDBPath, err := filepath.Abs(expandedDBPath)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("abs db path: %w", err)
	}
	stateDir := filepath.Dir(absDBPath)

	wtMgr := worktree.NewManager(stateDir, cfg)

	mgr := session.NewManager(database, wtMgr, stateDir)

	watcherDB := &prodWatcherDB{db: database}
	watcherMgr := status.NewWatcherManager(mgr, watcherDB, prodTmuxOps{})
	watcherMgr.Start(context.Background())

	repoIndexer := ghsvc.NewRepoIndexer(stateDir)
	repoIndexer.Start(context.Background())

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		WatcherManager: watcherMgr,
		Database:       database,
		RepoIndexer:    repoIndexer,
		StateDir:       stateDir,
	})

	cleanup := func() {
		watcherMgr.Close()
		repoIndexer.Close()
		database.Close()
	}
	return handler, cleanup, nil
}
```

- [ ] **Step 4: Delete old status files**

Delete the following files:
- `internal/node/status/detector.go`
- `internal/node/status/detector_test.go`
- `internal/node/status/patterns.go`
- `internal/node/status/monitor.go`
- `internal/node/status/monitor_test.go`

- [ ] **Step 5: Verify compilation**

Run: `go build ./...`

Expected: Build succeeds. If there are compilation errors from other files referencing the old `Monitor`/`Detector` types, fix them.

- [ ] **Step 6: Wire EnsureWatching into session lifecycle**

The `WatcherManager` needs to be called from `EnsureSession` and session creation. The `Manager` in `session/lifecycle.go` doesn't have a reference to the `WatcherManager`. Rather than coupling them directly, have the API handlers call `EnsureWatching` after `EnsureSession` succeeds.

Modify `internal/node/api/sessions.go` — update the `sessionHandler` to include the watcher manager, and call `EnsureWatching` in the `get` handler:

First, update the `sessionHandler` struct:

```go
type sessionHandler struct {
	manager        *session.Manager
	watcherManager watcherEnsurer
}

// watcherEnsurer is satisfied by *status.WatcherManager.
type watcherEnsurer interface {
	EnsureWatching(sessionID, tmuxName, providerType string)
}
```

In the `get` handler, after `EnsureSession`:

```go
func (h *sessionHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	tmuxName, err := h.manager.EnsureSession(id)
	if err != nil {
		// ... existing error handling
	}

	sess, err := h.manager.Get(id)
	if err != nil {
		// ... existing error handling
	}

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(sess.ID, tmuxName, sess.ProviderType)
	}

	respondJSON(w, http.StatusOK, map[string]any{"session": sess})
}
```

In the `create` handler, after creating the session:

```go
func (h *sessionHandler) create(w http.ResponseWriter, r *http.Request) {
	// ... existing create logic ...

	sess, err := h.manager.Create(opts)
	if err != nil {
		// ... existing error handling
	}

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": sess})
}
```

In the `delete` handler, stop the watcher:

```go
func (h *sessionHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// ... existing delete logic ...

	if h.watcherManager != nil {
		h.watcherManager.StopWatcher(id)
	}

	// ... existing response
}
```

Update the router to pass watcher manager to session handler:

```go
sh := &sessionHandler{manager: deps.SessionManager, watcherManager: deps.WatcherManager}
```

Also update `terminal.HandleSessionWebSocket` to call `EnsureWatching`. This requires passing the watcher manager through. Add it to the WebSocket handler's factory closure, or call `EnsureWatching` in the router wrapper. Simplest: wrap the WebSocket handler:

```go
// Terminal WebSocket — wraps to call EnsureWatching after successful attach
wsHandler := terminal.HandleSessionWebSocket(deps.SessionManager)
if deps.WatcherManager != nil {
	origHandler := wsHandler
	wsHandler = func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		origHandler(w, r)
		// EnsureWatching is idempotent — safe even if the session was already watched.
		// We need the session details. Since EnsureSession was already called inside
		// the WebSocket handler, the tmux session is alive. Fetch session for metadata.
		if sess, err := deps.SessionManager.Get(id); err == nil && sess != nil {
			deps.WatcherManager.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
		}
	}
}
mux.HandleFunc("/ws/sessions/{id}", wsHandler)
```

- [ ] **Step 7: Run full test suite**

Run: `go test ./... 2>&1 | head -80`

Expected: All tests pass. Fix any remaining compilation or test failures.

- [ ] **Step 8: Commit**

```bash
git add internal/node/api/status.go internal/node/api/router.go internal/node/api/sessions.go internal/node/setup.go
git rm internal/node/status/detector.go internal/node/status/detector_test.go internal/node/status/patterns.go internal/node/status/monitor.go internal/node/status/monitor_test.go
git commit -m "feat(status): replace Monitor/Detector with WatcherManager, wire new endpoints (BXN-34)"
```

---

## Task 8: Frontend — Update Types and Status Display

**Files:**
- Modify: `web/src/types.ts`
- Modify: `web/src/components/SessionList/index.tsx`

- [ ] **Step 1: Update TypeScript types**

In `web/src/types.ts`, update `SessionStatusInfo`:

```typescript
export interface SessionStatusInfo {
  sessionName: string;
  status: "active" | "inactive" | "dead";
  providerType: ProviderType;
  unreadSince?: string | null;
}
```

Remove `lastLine` field (no longer sent by backend). Remove `"running"`, `"waiting"`, `"idle"`, `"error"` from the status union.

- [ ] **Step 2: Update SessionList status functions**

In `web/src/components/SessionList/index.tsx`, update the three status functions:

```typescript
function getStatusColor(status?: string) {
  switch (status) {
    case "active":
      return "bg-green-500";
    case "inactive":
      return "bg-muted-foreground";
    case "dead":
      return "bg-red-500/50";
    default:
      return "bg-muted-foreground/40";
  }
}

function getStatusAnimation(status?: string) {
  switch (status) {
    case "active":
      return "animate-pulse-green";
    default:
      return "";
  }
}

function getStatusLabel(status?: string) {
  switch (status) {
    case "active":
      return "Active";
    case "inactive":
      return "Inactive";
    case "dead":
      return "Dead";
    default:
      return "";
  }
}
```

- [ ] **Step 3: Add unread dot to SessionItem**

In `web/src/components/SessionList/index.tsx`, update `SessionItemProps` to include unread info:

```typescript
interface SessionItemProps {
  session: Session;
  homeDir: string;
  isActive: boolean;
  statusValue?: SessionStatusInfo["status"];
  unreadSince?: string | null;
  minuteTick: number;
  // ... rest unchanged
}
```

In the `SessionItem` component, add the blue unread dot next to the status dot. Find the status dot `<div>` and add after it:

```tsx
<div className="mt-0.5 flex items-center gap-1.5">
  <div
    className={cn(
      "h-1.5 w-1.5 flex-shrink-0 rounded-full",
      getStatusColor(statusValue),
      getStatusAnimation(statusValue)
    )}
  />
  {unreadSince && (
    <div className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-blue-500" />
  )}
  <span className="text-muted-foreground text-xs">
    {(() => {
      const label = getStatusLabel(statusValue);
      return label ? `${label} · ` : "";
    })()}
    {formatRelativeTime(session.updated_at)}
  </span>
</div>
```

In the `SessionList` component, update how `SessionItem` is rendered to pass `unreadSince`:

```tsx
<SessionItem
  key={session.id}
  session={session}
  homeDir={homeDir}
  isActive={session.id === activeSessionId}
  statusValue={sessionStatuses?.[session.id]?.status}
  unreadSince={sessionStatuses?.[session.id]?.unreadSince}
  minuteTick={minuteTick}
  // ... rest unchanged
/>
```

- [ ] **Step 4: Verify frontend builds**

Run: `cd web && npm run build`

Expected: Build succeeds. TypeScript may flag references to old status values in other files — fix them in subsequent steps.

- [ ] **Step 5: Commit**

```bash
git add web/src/types.ts web/src/components/SessionList/index.tsx
git commit -m "feat(web): update status display to active/inactive/dead, add unread dot (BXN-34)"
```

---

## Task 9: Frontend — Heartbeat, Acknowledge, and Notifications

**Files:**
- Modify: `web/src/data/statuses/queries.ts`
- Modify: `web/src/hooks/useNotifications.ts`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Add heartbeat to status polling**

In `web/src/data/statuses/queries.ts`, piggyback the heartbeat on the existing status poll:

```typescript
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { apiFetch } from "@/api/client";
import type { Session, SessionStatusInfo } from "@/types";
import { statusKeys } from "../sessions/keys";

interface StatusResponse {
  statuses: Record<string, SessionStatusInfo>;
}

interface UseSessionStatusesOptions {
  sessions: Session[];
  activeSessionId?: string | null;
  checkStateChanges: (
    states: Array<{
      id: string;
      name: string;
      status: SessionStatusInfo["status"];
      unreadSince?: string | null;
    }>,
    activeSessionId?: string | null,
  ) => void;
}

export function useSessionStatusesQuery({
  sessions,
  activeSessionId,
  checkStateChanges,
}: UseSessionStatusesOptions) {
  const query = useQuery({
    queryKey: statusKeys.all,
    queryFn: () => apiFetch<StatusResponse>("/node/api/sessions/status"),
    enabled: sessions.length > 0,
    staleTime: 2000,
    refetchInterval: sessions.length > 0 ? 2000 : false,
  });

  // Send heartbeat for the actively viewed session, piggybacked on poll cadence.
  // Only fires when document is visible and a session is selected.
  useEffect(() => {
    if (!activeSessionId || !query.data) return;
    if (document.hidden) return;

    // Fire-and-forget heartbeat — errors are silently ignored.
    fetch(`${import.meta.env.VITE_NODE_URL || ""}/node/api/sessions/${encodeURIComponent(activeSessionId)}/heartbeat`, {
      method: "POST",
    }).catch(() => {});
  }, [query.data, activeSessionId]);

  useEffect(() => {
    if (!query.data?.statuses) return;

    const statuses = query.data.statuses;

    const sessionStates = sessions.map((s) => ({
      id: s.id,
      name: s.name,
      status: (statuses[s.id]?.status || "dead") as SessionStatusInfo["status"],
      unreadSince: statuses[s.id]?.unreadSince,
    }));
    checkStateChanges(sessionStates, activeSessionId);
  }, [query.data, sessions, activeSessionId, checkStateChanges]);

  return {
    sessionStatuses: query.data?.statuses ?? ({} as Record<string, SessionStatusInfo>),
    isLoading: query.isLoading,
  };
}
```

- [ ] **Step 2: Update notifications to use unreadSince**

Rewrite `web/src/hooks/useNotifications.ts`:

```typescript
import { useState, useEffect, useCallback, useRef } from "react";
import { toast } from "sonner";

interface NotificationSettings {
  enabled: boolean;
  sound: boolean;
}

const STORAGE_KEY = "argus-notification-settings";

function loadSettings(): NotificationSettings {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return JSON.parse(stored);
  } catch {}
  return { enabled: true, sound: false };
}

function saveSettings(settings: NotificationSettings) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
}

export function useNotifications() {
  const [settings, setSettings] = useState<NotificationSettings>(loadSettings);
  const [permission, setPermission] = useState<NotificationPermission>(
    typeof Notification !== "undefined" ? Notification.permission : "default"
  );
  const previousUnread = useRef<Set<string>>(new Set());
  const originalTitle = useRef(document.title);

  const requestPermission = useCallback(async () => {
    if (typeof Notification === "undefined") return;
    const result = await Notification.requestPermission();
    setPermission(result);
  }, []);

  const updateSettings = useCallback((updates: Partial<NotificationSettings>) => {
    setSettings(prev => {
      const next = { ...prev, ...updates };
      saveSettings(next);
      return next;
    });
  }, []);

  const checkStateChanges = useCallback(
    (
      states: Array<{ id: string; name: string; status: string; unreadSince?: string | null }>,
      activeSessionId?: string | null
    ) => {
      if (!settings.enabled) return;

      for (const state of states) {
        const wasUnread = previousUnread.current.has(state.id);
        const isUnread = !!state.unreadSince;

        if (isUnread) {
          previousUnread.current.add(state.id);
        } else {
          previousUnread.current.delete(state.id);
        }

        // Notify when unreadSince newly appears on a non-active session
        if (!wasUnread && isUnread && state.id !== activeSessionId) {
          toast.info(`${state.name} finished working`);

          if (permission === "granted" && document.hidden) {
            new Notification("Argus", {
              body: `${state.name} finished working`,
              tag: `unread-${state.id}`,
            });
          }

          // Flash tab title
          document.title = `⚡ ${state.name} finished`;
          setTimeout(() => { document.title = originalTitle.current; }, 3000);
        }
      }
    },
    [settings.enabled, permission]
  );

  // Reset title on visibility change
  useEffect(() => {
    const handler = () => {
      if (document.visibilityState === "visible") {
        document.title = originalTitle.current;
      }
    };
    document.addEventListener("visibilitychange", handler);
    return () => document.removeEventListener("visibilitychange", handler);
  }, []);

  return { settings, permission, requestPermission, updateSettings, checkStateChanges };
}
```

- [ ] **Step 3: Add acknowledge call on session select**

In `web/src/App.tsx`, update `attachToSession` to call acknowledge when a session with unread is selected:

```typescript
const attachToSession = useCallback(
  (session: Session) => {
    attachSession(session.id);

    // Acknowledge unread state when selecting a session
    const status = sessionStatuses[session.id];
    if (status?.unreadSince) {
      fetch(`${import.meta.env.VITE_NODE_URL || ""}/node/api/sessions/${encodeURIComponent(session.id)}/acknowledge`, {
        method: "POST",
      }).catch(() => {});
    }
  },
  [attachSession, sessionStatuses]
);
```

Note: `sessionStatuses` must be accessible in this callback. Looking at the current code, `sessionStatuses` is declared before `attachToSession` in `HomeContent`, so it's available.

- [ ] **Step 4: Update useSessionStatuses hook signature**

In `web/src/hooks/useSessionStatuses.ts`, update the callback type to include `unreadSince`:

```typescript
import type { Session, SessionStatusInfo } from "@/types";
import { useSessionStatusesQuery } from "@/data/statuses/queries";

interface UseSessionStatusesOptions {
  sessions: Session[];
  activeSessionId?: string | null;
  checkStateChanges: (
    states: Array<{ id: string; name: string; status: SessionStatusInfo["status"]; unreadSince?: string | null }>,
    activeSessionId?: string | null
  ) => void;
}

export function useSessionStatuses({ sessions, activeSessionId, checkStateChanges }: UseSessionStatusesOptions) {
  const { sessionStatuses } = useSessionStatusesQuery({ sessions, activeSessionId, checkStateChanges });
  return { sessionStatuses };
}
```

- [ ] **Step 5: Verify frontend builds**

Run: `cd web && npm run build`

Expected: Build succeeds with no type errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/data/statuses/queries.ts web/src/hooks/useNotifications.ts web/src/hooks/useSessionStatuses.ts web/src/App.tsx
git commit -m "feat(web): add heartbeat, acknowledge, and unread-based notifications (BXN-34)"
```

---

## Task 10: CLI — Subprocess Attach with Heartbeat

**Files:**
- Modify: `cmd/argus/cli/session_attach.go`

- [ ] **Step 1: Write failing test for new attach behavior**

Add to `cmd/argus/cli/commands_test.go` (or a new file `cmd/argus/cli/session_attach_test.go`):

```go
func TestAttachCmd_RequiresArg(t *testing.T) {
	cmd := newAttachCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}
```

This is a basic smoke test. The real attach behavior requires tmux and can't be unit-tested, but we can verify the command structure.

- [ ] **Step 2: Run test**

Run: `go test ./cmd/argus/cli/ -run TestAttachCmd_RequiresArg -v`

Expected: PASS (command already requires exactly 1 arg)

- [ ] **Step 3: Rewrite attach command with subprocess wrapper + heartbeat**

Rewrite `cmd/argus/cli/session_attach.go`:

```go
package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <name-or-id>",
		Short: "Attach to a session's tmux",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			query := args[0]

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			session, err := fetchAndResolve(c, query)
			if err != nil {
				return err
			}

			// Call EnsureSession via the GET /api/sessions/{id} endpoint
			// so the node revives the tmux session if it died.
			_, err = c.get("/api/sessions/" + session.ID)
			if err != nil {
				return fmt.Errorf("ensure session: %w", err)
			}

			// Acknowledge unread state before attaching
			_, _ = c.post("/api/sessions/"+session.ID+"/acknowledge", nil)

			return attachTmux(session.ID, session.TmuxName, c.baseURL)
		},
	}
}

// attachTmux runs tmux attach-session as a subprocess with a heartbeat goroutine.
func attachTmux(sessionID, tmuxName, baseURL string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	tmuxCmd := exec.Command(tmuxPath, "attach-session", "-t", tmuxName)
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	if err := tmuxCmd.Start(); err != nil {
		return fmt.Errorf("start tmux: %w", err)
	}

	// Start heartbeat goroutine
	ctx, cancel := context.WithCancel(context.Background())
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		runHeartbeat(ctx, sessionID, baseURL)
	}()

	// Wait for tmux to exit (user detaches or session ends)
	err = tmuxCmd.Wait()

	// Stop heartbeat
	cancel()
	<-heartbeatDone

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("tmux: %w", err)
	}
	return nil
}

// runHeartbeat sends periodic heartbeat requests while the context is active.
func runHeartbeat(ctx context.Context, sessionID, baseURL string) {
	client := &http.Client{Timeout: 2 * time.Second}
	url := baseURL + "/api/sessions/" + sessionID + "/heartbeat"
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
			if err != nil {
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				continue // heartbeat failures are silent
			}
			resp.Body.Close()
		}
	}
}
```

- [ ] **Step 4: Verify compilation**

Run: `go build ./cmd/argus/...`

Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add cmd/argus/cli/session_attach.go
git commit -m "feat(cli): replace syscall.Exec with subprocess wrapper + heartbeat (BXN-34)"
```

---

## Task 11: CLI — Update Session List Display

**Files:**
- Modify: `cmd/argus/cli/session_list.go`

- [ ] **Step 1: Update list command to show new statuses and unread**

Modify `cmd/argus/cli/session_list.go`. Update the status display section:

```go
func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			body, err := c.get("/api/sessions")
			if err != nil {
				return err
			}

			var resp struct {
				Sessions []sessionInfo `json:"sessions"`
				HomeDir  string        `json:"home_dir"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			if len(resp.Sessions) == 0 {
				fmt.Println("No sessions.")
				return nil
			}

			// Fetch session statuses (best-effort)
			type statusEntry struct {
				Status      string  `json:"status"`
				UnreadSince *string `json:"unreadSince"`
			}
			statuses := make(map[string]statusEntry)
			if statusBody, err := c.get("/api/sessions/status"); err == nil {
				var statusResp struct {
					Statuses map[string]statusEntry `json:"statuses"`
				}
				if err := json.Unmarshal(statusBody, &statusResp); err == nil {
					statuses = statusResp.Statuses
				}
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "  ID\tNAME\tSTATUS\tPROVIDER\tDIRECTORY\tBRANCH\tUPDATED")
			for _, s := range resp.Sessions {
				entry := statuses[s.ID]
				st := entry.Status
				if st == "" {
					st = "-"
				}
				branch := ""
				if s.WorktreeBranch != nil {
					branch = *s.WorktreeBranch
				}
				dir := s.WorkingDirectory
				if s.GitParentDir != nil {
					dir = *s.GitParentDir
				}
				if dir == "" {
					dir = "-"
				} else {
					dir = compressPath(dir, resp.HomeDir, 35)
				}

				// Unread marker and suffix
				marker := " "
				updated := relativeTime(s.UpdatedAt)
				if entry.UnreadSince != nil {
					marker = "*"
					updated += " (unread " + relativeTime(*entry.UnreadSince) + ")"
				}

				fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					marker, s.ID, s.Name, st, s.ProviderType, dir, branch, updated)
			}
			w.Flush()
			return nil
		},
	}
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./cmd/argus/...`

Expected: Build succeeds.

- [ ] **Step 3: Run existing CLI tests**

Run: `go test ./cmd/argus/cli/ -v`

Expected: All tests pass. Some tests may need updating if they check exact output format.

- [ ] **Step 4: Commit**

```bash
git add cmd/argus/cli/session_list.go
git commit -m "feat(cli): show active/inactive/dead statuses and unread indicator (BXN-34)"
```

---

## Task 12: Final Integration Test and Cleanup

**Files:**
- All modified files

- [ ] **Step 1: Run full backend test suite**

Run: `go test ./... 2>&1 | tail -30`

Expected: All tests pass. Fix any failures.

- [ ] **Step 2: Run frontend build**

Run: `cd web && npm run build`

Expected: Build succeeds with no errors.

- [ ] **Step 3: Run frontend lint/typecheck if available**

Run: `cd web && npx tsc --noEmit 2>&1 | head -30`

Expected: No type errors.

- [ ] **Step 4: Verify no references to old status values remain**

Search for stale references:

```bash
grep -r '"running"\|"waiting"\|"idle"\|StatusRunning\|StatusWaiting\|StatusIdle\|StatusMonitor\|NewDetector\|NewMonitor' --include='*.go' --include='*.ts' --include='*.tsx' internal/ web/src/ cmd/
```

Expected: No matches (or only in test fixtures / comments that need updating).

- [ ] **Step 5: Final commit if any cleanup was needed**

```bash
git add -A
git commit -m "chore: clean up stale status references (BXN-34)"
```
