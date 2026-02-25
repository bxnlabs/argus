# Session Status Improvements

## Problem

The session status detection system has two issues:

1. **Logic bug:** The `waiting` state conflates "agent needs user input" with "agent stopped working." The `acknowledged` flag that was supposed to distinguish these is dead code — `Acknowledge()` is never called, so sessions that finish work get stuck showing `waiting` forever.

2. **Bland visuals:** Status indicators are static 6px colored dots with no animation, no labels, and no presence in CLI output. Users can't tell at a glance what a color means without learning the mapping.

## Design

### State Model

Four states, determined purely by terminal content and tmux activity. No acknowledge mechanism.

| Status | Detection | Color | Dot Animation | Label |
|---|---|---|---|---|
| `running` | Busy indicators, spinners, or sustained tmux activity | `green-500` | Pulsing glow | "Running" |
| `waiting` | Input prompt patterns (`[Y/n]`, `Allow?`, etc.) in last 5 lines | `yellow-500` | Pulsing glow | "Needs input" |
| `idle` | Activity stopped, no prompts detected | `muted-foreground` | Static | "Idle" |
| `dead` | Tmux session no longer exists | `red-500/50` | Static | "Dead" |
| `error` | API fetch failure (frontend-only) | `red-500` | Static | "Error" |

Lifecycle: `idle` -> `running` -> `waiting` -> `running` -> `idle`

### Backend Changes

**File: `internal/agent/status/detector.go`**

- Remove `acknowledged` field from `stateTracker`
- Remove `Acknowledge()` method (dead code)
- Replace `getIdleOrWaiting()` with direct `StatusIdle` return — when the cooldown expires and no busy indicators or waiting patterns are found, the session is idle
- No new API endpoints or status constants

Everything else stays the same: `checkBusyIndicators`, `checkWaitingPatterns`, spike detection, cooldown logic, `patterns.go`.

### Frontend Changes

**File: `web/src/components/SessionList/index.tsx`**

Replace the inline status dot with a `StatusIndicator` component:

- Dot: `h-1.5 w-1.5 rounded-full` with color from `getStatusColor()`
- Label: `text-xs text-muted-foreground` with human-readable status
- Layout: `[dot] Running · 5 min ago` (dot, label, middle-dot separator, timestamp)

Status label mapping:

| Status | Label |
|---|---|
| `running` | "Running" |
| `waiting` | "Needs input" |
| `idle` | "Idle" |
| `dead` | "Dead" |
| `error` | "Error" |

**File: `web/src/globals.css`**

Add keyframe animations:

- `pulse-glow`: Box-shadow expansion/contraction with the status color, ~2s cycle. Applied to `running` and `waiting` dots.
- Idle, dead, and error dots remain static.

### CLI Changes

**File: `cmd/argus/cli/session_list.go`**

Add a STATUS column to `session ls` output by fetching `/api/sessions/status` alongside the existing `/api/sessions` call.

```
ID          NAME              STATUS     PROVIDER     UPDATED
abc123def   My Session        running    claude       just now
def456ghi   Testing Session   waiting    shell        5 min ago
ghi789jkl   Analysis Work     idle       codex        2 hours ago
```

### What's Not Changing

- `patterns.go` (busy indicators, waiting patterns, spinner chars)
- Spike detection and cooldown logic
- Notification system (`useNotifications.ts`) — `running -> waiting` transition still works
- Status polling interval (2s)
- Status API endpoint structure
