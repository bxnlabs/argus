# Session Activity Detection, Unread Status & Notifications

**Linear Issue:** BXN-34
**Date:** 2026-04-13
**Status:** Design approved

## Overview

Replace the current keyword/regex-based session activity detection with frame-diffing: capture tmux pane content periodically and compare consecutive frames to infer activity. This eliminates dependency on specific CLI UI patterns, fixes false negatives on narrow viewports, and works identically across all runtimes (Claude Code, Codex, Gemini, shell).

Layer an unread status system on top: when a session stops being active, mark it unread. Provide visible cues in the web UI and CLI, and fire notifications (via a pluggable notifier interface) when sessions remain unread for 10+ minutes.

## State Model

Three session states, plus an orthogonal unread flag:

| State | Meaning | Visual (Web) | Visual (CLI) |
|-------|---------|--------------|--------------|
| `active` | Frames are changing between captures | Green dot + pulse | `active` label |
| `inactive` | Frames have stabilized (N consecutive identical frames) | Gray dot | `inactive` label |
| `dead` | Tmux session no longer exists | Red dot (faded) | `dead` label |

**Unread** is tracked via a nullable `unread_since` timestamp on the session, independent of state.

### State Transitions

```
            frames differ
inactive ──────────────────► active
    ^                           |
    |   N stable frames         |
    └───────────────────────────┘

any state ──(tmux session gone)──► dead
```

### Unread Lifecycle

**Set** on `active -> inactive` transition (always, unconditionally).

**Cleared** on:
1. User views the session in the web UI (frontend calls acknowledge endpoint)
2. User runs `argus session attach` (CLI calls acknowledge endpoint)
3. Session transitions `inactive -> active` (activity resumed)
4. Session becomes `dead`

## Architecture

### SessionWatcher (per-session goroutine)

Each session gets a dedicated `SessionWatcher` goroutine that owns its capture loop and state machine. Replaces the current `Monitor` + `Detector`.

**Capture cycle (every 2 seconds):**

1. Capture pane content via `tmux capture-pane -t <name> -p -J`
2. Capture pane dimensions via `tmux display-message -t <name> -p '#{pane_width}x#{pane_height}'`
3. Compare to previous frame:
   - **Dimensions differ from previous** -> store new frame as baseline, skip comparison (resize in progress)
   - **Content differs** -> state = `active`, clear `unread_since` if set
   - **Content identical** -> increment stability counter
     - If counter >= stability threshold (2 consecutive identical frames, ~4 seconds) -> state = `inactive`
     - If transitioning from `active` -> set `unread_since = now()`
4. If tmux session is gone -> state = `dead`, clear `unread_since`, stop loop
5. Write state to shared snapshot (for API reads)
6. Sleep 2 seconds, repeat

**Key design decisions:**
- New sessions start as `inactive` with `unread_since = NULL` (no previous frame to compare against, not yet unread)
- Stability threshold of 2 consecutive identical frames (~4s) avoids false inactivity from momentary pauses between tool calls
- Dimension check prevents false activity detection from pane resizes / TUI redraws
- Watcher always sets `unread_since` on transition; clearing is handled by acknowledge endpoints (no "currently viewed" check needed in the watcher)

### WatcherManager

Manages the lifecycle of all `SessionWatcher` goroutines:

- **On startup** — queries all existing sessions from the DB, starts a watcher for each non-dead session
- **On session create** — starts a new watcher
- **On session delete** — stops and cleans up the watcher
- **On shutdown** — cancels all watcher contexts, waits for goroutines to exit

Each watcher gets a `context.Context` derived from the manager's context for clean cascading cancellation.

### Shared Snapshot

Same concept as the current implementation: an in-memory map of session ID -> state, protected by `sync.RWMutex`. The API reads from it, watchers write to it. No change to the frontend polling pattern.

## Notifier Interface

A pluggable interface for notification backends. Ships with no concrete implementations initially; Slack/email/webhooks are added later.

```go
type Notification struct {
    SessionID   string
    SessionName string
    UnreadSince time.Time
    State       SessionState
}

type Notifier interface {
    Notify(ctx context.Context, notification Notification) error
}
```

### NotificationManager

Runs a periodic check (every 60 seconds):

1. Query all sessions where `unread_since IS NOT NULL AND unread_since <= now() - 10m`
2. Check deduplication: skip if `last_notified_at >= unread_since`
3. Call each registered `Notifier`
4. Set `last_notified_at = now()` on the session

**Deduplication:** A `last_notified_at` column on the session tracks when the last notification was sent. Notification fires when `unread_since IS NOT NULL AND unread_since <= now() - 10m AND (last_notified_at IS NULL OR last_notified_at < unread_since)`. When `unread_since` is cleared and set again later (new unread period), notifications can fire again.

## Database Changes

### Schema

Add two columns to the `sessions` table:

- **`unread_since`** (`TEXT`, nullable) — ISO timestamp. NULL = read, non-null = unread since that time.
- **`last_notified_at`** (`TEXT`, nullable) — ISO timestamp. Last notification sent for the current unread period.

### Removals

- The `waiting` status concept
- `patterns.go` (all regex/keyword patterns)
- The existing `Detector` struct
- The existing `Monitor` struct

## API Changes

### Modified Endpoints

**`GET /api/sessions/status`** — Add `unreadSince` (nullable ISO timestamp) to the per-session status entry.

### New Endpoints

**`POST /api/sessions/{id}/acknowledge`** — Sets `unread_since = NULL`. Called by:
- Frontend when user selects/views a session
- CLI on `argus session attach`

## Frontend Changes

### Session List — Unread Indicator

A small blue dot on session list items where `unreadSince` is non-null. Separate from the status dot (green/gray/red).

### Status Display Updates

| Old | New |
|-----|-----|
| "Running" (green pulse) | "Active" (green pulse) |
| "Needs input" (yellow pulse) | *removed* |
| "Idle" (gray) | "Inactive" (gray) |
| "Dead" (red faded) | "Dead" (red faded) |

### Acknowledge on View

When the user selects a session with `unreadSince` set, the frontend calls `POST /api/sessions/{id}/acknowledge` optimistically — update local state immediately, API call in background.

### Notification Hook Updates

`useNotifications.ts` fires browser notifications based on `unreadSince` appearing (session becomes unread while the document is hidden), replacing the old `running -> waiting` logic.

## CLI Changes

### `argus session attach`

Calls `POST /api/sessions/{id}/acknowledge` to clear `unread_since` before/after attaching.

### `argus session list`

Shows new states and unread indicator:

```
  sess_abc  my-feature    active     /home/user/project
* sess_def  bugfix        inactive   /home/user/other     (unread 5m ago)
  sess_ghi  refactor      dead       /home/user/old
```

`*` marker and `(unread Xm ago)` suffix for sessions with `unread_since` set.
