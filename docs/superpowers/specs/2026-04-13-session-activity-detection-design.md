# Session Activity Detection, Unread Status & Notifications

**Linear Issue:** BXN-34
**Date:** 2026-04-13
**Status:** Design approved

## Overview

Replace the current keyword/regex-based session activity detection with frame-diffing: capture tmux pane content periodically and compare consecutive frames to infer activity. This eliminates dependency on specific CLI UI patterns, fixes false negatives on narrow viewports, and works identically across all runtimes (Claude Code, Codex, Gemini, shell).

Layer an unread status system on top: when a session stops being active and no client is currently observing it, mark it unread. Provide visible cues in the web UI and CLI. A pluggable `Notifier` interface is defined for future notification backends (Slack, email, webhooks); no concrete implementations ship initially. Notification delivery, deduplication, and preferences will be specified when the first concrete notifier is implemented.

## State Model

Three session states, plus an orthogonal unread flag:

| State | Meaning | Visual (Web) | Visual (CLI) |
|-------|---------|--------------|--------------|
| `active` | Frames are changing between captures | Green dot + pulse | `active` label |
| `inactive` | Frames have stabilized (N consecutive identical frames) | Gray dot | `inactive` label |
| `dead` | Tmux session no longer exists | Red dot (faded) | `dead` label |

**Unread** is tracked via a nullable `unread_since` timestamp on the session, independent of state. **Observation** is tracked via a `last_viewed_at` timestamp, updated by a ~2s heartbeat while a client (web or CLI) is actively viewing the session.

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

**Set** on `active -> inactive` transition, but only when the session is not currently being observed. Specifically: set `unread_since = now()` when `last_viewed_at < last_activity_timestamp` (no client has seen the recent activity). If `last_viewed_at >= last_activity_timestamp`, the transition is already observed and unread is not set.

**Cleared** on:
1. Client calls the acknowledge endpoint (web on session select, CLI before attach)
2. Session transitions `inactive -> active` (activity resumed)
3. Session becomes `dead`

Note: the heartbeat endpoint does **not** clear `unread_since` — it only updates `last_viewed_at`. The heartbeat's role is preventive: by keeping `last_viewed_at` fresh, it prevents the watcher from setting `unread_since` on the next `active -> inactive` transition. To clear an existing unread state, the client must call acknowledge.

## Architecture

### SessionWatcher (per-session goroutine)

Each session gets a dedicated `SessionWatcher` goroutine that owns its capture loop and state machine. Replaces the current `Monitor` + `Detector`.

**Capture cycle (every 2 seconds):**

1. Capture pane dimensions via `tmux display-message -t <name> -p '#{pane_width}x#{pane_height}'` (pre-capture)
2. Capture pane content via `tmux capture-pane -t <name> -p -J`
3. Capture pane dimensions again (post-capture)
4. **If pre-capture and post-capture dimensions differ** -> discard frame (resize in progress), reset stability counter, sleep and retry
5. Compare to previous frame:
   - **Dimensions differ from previous baseline** -> store new frame as baseline, skip comparison
   - **Content differs** -> state = `active`, record `last_activity_timestamp`, clear `unread_since` if set
   - **Content identical** -> increment stability counter
     - If counter >= stability threshold (2 consecutive identical frames, ~4 seconds) -> state = `inactive`
     - If transitioning from `active` AND `last_viewed_at < last_activity_timestamp` -> set `unread_since = now()` (not currently observed)
     - If transitioning from `active` AND `last_viewed_at >= last_activity_timestamp` -> do not set `unread_since` (client already saw the activity)
6. **Dead detection:** Run `tmux has-session -t <name>`.
   - If explicit "not found" -> state = `dead`, clear `unread_since`, stop loop
   - If capture/display commands fail but `has-session` succeeds or returns ambiguous error -> preserve prior state, log the error. Never transition to `dead` on ambiguous tmux errors.
7. Persist `unread_since` transitions to DB (single `UPDATE sessions SET unread_since = ? WHERE id = ?`)
8. Call `TouchSession(id, now)` on state change or while `active` (preserves `updated_at` freshness for session list ordering)
9. Write activity state to shared snapshot (for API reads)
10. Sleep 2 seconds, repeat

**Key design decisions:**
- New sessions start as `inactive` with `unread_since = NULL` (no previous frame to compare against, not yet unread)
- Stability threshold of 2 consecutive identical frames (~4s) avoids false inactivity from momentary pauses between tool calls
- Atomic resize guard: dimensions captured before and after content; frame discarded if they differ, preventing false activity from mid-resize captures
- Unread is observation-aware: watcher only sets `unread_since` when `last_viewed_at < last_activity_timestamp`, so sessions being actively observed never become unread
- Dead detection requires positive `tmux has-session` confirmation; ambiguous tmux errors (timeouts, server issues) preserve prior state and are logged, never triggering a `dead` transition
- Watcher persists `unread_since` transitions to DB and calls `TouchSession` to maintain `updated_at` freshness (preserving session list ordering)
- Argus sessions are single-pane; manual pane/window splits after attach are unsupported

### WatcherManager

Manages the lifecycle of all `SessionWatcher` goroutines:

- **On startup** — queries all sessions from the DB, starts a watcher for each. Watchers that find their tmux session already gone will immediately transition to `dead` and exit on the first capture cycle. (Activity state is not persisted to DB, so startup cannot filter by liveness.)
- **On session create** — starts a new watcher
- **On session delete** — stops and cleans up the watcher
- **On shutdown** — cancels all watcher contexts, waits for goroutines to exit
- **`EnsureWatching(sessionID)`** — idempotent method that guarantees a watcher is running for the given session. Called on every successful `EnsureSession` return (not just after tmux recreation). This handles session revival: when `EnsureSession` recreates a dead tmux session, the watcher is automatically restarted. Covers both revival paths: `GET /api/sessions/{id}` and WebSocket terminal attach.

Each watcher gets a `context.Context` derived from the manager's context for clean cascading cancellation.

### Shared Snapshot

In-memory map of session ID -> activity state (`active`/`inactive`/`dead`), protected by `sync.RWMutex`. Watchers write activity state to it. The snapshot contains **activity state only** — `unread_since`, `last_viewed_at`, and other persistent fields are authoritative in the DB and are never mirrored into the snapshot.

`GET /api/sessions/status` composes the response by reading activity state from the snapshot and `unread_since` from the DB.

## Notifier Interface

A pluggable interface for future notification backends (Slack, email, webhooks). No concrete implementations ship initially.

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

Notification delivery semantics (timing thresholds, deduplication, per-backend delivery tracking, rate limiting, and user preferences) will be specified when the first concrete notifier is implemented.

## Database Changes

### Schema

Add two columns to the `sessions` table:

- **`unread_since`** (`TEXT`, nullable) — Timestamp in SQLite `datetime()` format (e.g. `2026-04-13 12:00:00`). NULL = read, non-null = unread since that time.
- **`last_viewed_at`** (`TEXT`, nullable) — Timestamp in SQLite `datetime()` format. Last time a client (web or CLI) confirmed observation of this session. Updated by heartbeat.

All new timestamp columns use the same SQLite `datetime()` format as existing columns (`created_at`, `updated_at`). Comparisons use SQLite datetime functions, not raw string ordering.

### Removals

- The `waiting` status concept
- `patterns.go` (all regex/keyword patterns)
- The existing `Detector` struct
- The existing `Monitor` struct

## API Changes

### Modified Endpoints

**`GET /api/sessions/status`** — Two changes:
1. Add `unreadSince` (nullable ISO timestamp) to the per-session status entry. Read from DB, composed with activity state from snapshot.
2. **Breaking:** Status values change from `running`/`waiting`/`idle`/`dead` to `active`/`inactive`/`dead`. The `waiting` status is removed. Frontend and CLI must be updated in lockstep with this backend change.

### New Endpoints

**`POST /api/sessions/{id}/acknowledge`** — Sets `unread_since = NULL` and updates `last_viewed_at = now()`. Idempotent. Inherits the existing local-only node API trust model (no additional auth).

**`POST /api/sessions/{id}/heartbeat`** — Updates `last_viewed_at = now()`. Called at ~2s intervals by clients actively observing a session. Lightweight — performs a single DB update. Clients:
- **Web:** Piggybacked on the existing status poll interval while the session is the visible active tab and the document is visible
- **CLI:** Sent by the attach subprocess wrapper (see CLI Changes)

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

### Observation Heartbeat

While a session is the visible active tab and the document is visible, the frontend sends a heartbeat to `POST /api/sessions/{id}/heartbeat` piggybacked on the existing ~2s status poll cadence. This updates `last_viewed_at` on the server, suppressing unread transitions for sessions the user is actively watching. The heartbeat only fires for the currently visible session, not all mounted terminal components.

When the user first selects a session with `unreadSince` set, the frontend calls `POST /api/sessions/{id}/acknowledge` to clear the unread state immediately.

### Browser Notifications

`useNotifications.ts` fires browser notifications when `unreadSince` appears on a session while the document is hidden (user is in another tab/app), replacing the old `running -> waiting` logic. These are immediate — distinct from any future backend notifications which may use delayed thresholds.

## CLI Changes

### `argus session attach`

Replaces the current `syscall.Exec`-based attach with a subprocess wrapper that maintains a heartbeat:

1. Resolve session and confirm `tmux` binary exists
2. Call `POST /api/sessions/{id}/acknowledge` to clear `unread_since`
3. Start `tmux attach-session -t <name>` as a subprocess with `Stdin`/`Stdout`/`Stderr` connected to the terminal (tmux is the foreground process and receives terminal signals directly — no explicit signal forwarding needed)
4. Spawn a heartbeat goroutine that calls `POST /api/sessions/{id}/heartbeat` every ~2s to update `last_viewed_at`
5. `cmd.Wait()` blocks until tmux exits (user detaches or session ends)
6. Cancel heartbeat context, propagate tmux exit code

**Implementation constraints:**
- Heartbeat HTTP requests must be time-bounded (short timeout) so a slow/failed call does not delay process exit
- The heartbeat goroutine must not write to stdout/stderr while tmux owns the terminal
- If the Go wrapper is killed (SIGKILL), the tmux session stays alive and the heartbeat stops — server correctly treats `last_viewed_at` as stale within ~2s (fails safe)

### `argus session list`

Shows new states and unread indicator:

```
  sess_abc  my-feature    active     /home/user/project
* sess_def  bugfix        inactive   /home/user/other     (unread 5m ago)
  sess_ghi  refactor      dead       /home/user/old
```

`*` marker and `(unread Xm ago)` suffix for sessions with `unread_since` set.
