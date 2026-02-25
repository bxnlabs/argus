# Session Status Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the broken waiting/idle state logic and add animated status indicators with labels to the web UI and CLI output.

**Architecture:** Remove the dead `acknowledged` tracking from the Go status detector so that `waiting` strictly means "input prompt detected" and everything else that isn't running is `idle`. Add CSS pulse animations and text labels to the frontend status dots. Add a STATUS column to the CLI `session ls` command.

**Tech Stack:** Go (backend detector + CLI), React/TypeScript + Tailwind CSS 4 (frontend), no new dependencies.

---

### Task 1: Clean up the backend detector — remove dead acknowledge code

**Files:**
- Modify: `internal/agent/status/detector.go`

**Step 1: Remove the `acknowledged` field from `stateTracker`**

In `internal/agent/status/detector.go`, find the `stateTracker` struct (lines 30-36):

```go
type stateTracker struct {
	lastChangeTime        int64
	acknowledged          bool
	lastActivityTimestamp int64
	spikeWindowStart     *int64
	spikeChangeCount     int
}
```

Remove the `acknowledged` field:

```go
type stateTracker struct {
	lastChangeTime        int64
	lastActivityTimestamp int64
	spikeWindowStart     *int64
	spikeChangeCount     int
}
```

**Step 2: Remove initialization of `acknowledged` in `getTracker`**

Find `getTracker` (lines 85-97). The tracker initializer sets `acknowledged: true`. Remove that line:

```go
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
```

**Step 3: Replace `getIdleOrWaiting` with direct `StatusIdle` return**

Delete the `getIdleOrWaiting` method entirely (lines 195-200):

```go
// DELETE THIS ENTIRE METHOD:
func (d *Detector) getIdleOrWaiting(tracker *stateTracker) SessionStatus {
	if tracker.acknowledged {
		return StatusIdle
	}
	return StatusWaiting
}
```

In `GetStatus` (lines 202-251), replace the two call sites:

- Line 241: `return d.getIdleOrWaiting(tracker)` → `return StatusIdle`
- Line 250: `return d.getIdleOrWaiting(tracker)` → `return StatusIdle`

**Step 4: Remove `tracker.acknowledged = false` assignments**

Two places set `acknowledged = false`:
- Line 170 (inside `processSpikeDetection`): `tracker.acknowledged = false` — delete this line
- Line 222 (inside `GetStatus`, busy indicators branch): `tracker.acknowledged = false` — delete this line

**Step 5: Delete the `Acknowledge` method**

Remove lines 253-260 entirely:

```go
// DELETE THIS ENTIRE METHOD:
// Acknowledge marks a session as acknowledged (waiting → idle).
func (d *Detector) Acknowledge(sessionName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.trackers[sessionName]; ok {
		t.acknowledged = true
	}
}
```

**Step 6: Run tests to verify nothing breaks**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus && go test ./internal/agent/status/...`

Expected: All tests pass. The existing `TestCheckBusyIndicators` and `TestCheckWaitingPatterns` tests don't touch `acknowledged` at all — they test the pattern-matching functions directly.

**Step 7: Run full Go build**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus && go build ./...`

Expected: Clean build. No other Go code references `Acknowledge` or `acknowledged`.

**Step 8: Commit**

```bash
git add internal/agent/status/detector.go
git commit -m "fix: remove dead acknowledge code from status detector

The acknowledged flag was never set by any caller, causing sessions
to get stuck in 'waiting' after finishing work. Now waiting strictly
means an input prompt was detected, and idle is the resting state."
```

---

### Task 2: Add pulse-glow CSS animations

**Files:**
- Modify: `web/src/globals.css`

**Step 1: Add keyframe animations at the end of `globals.css`**

Append the following after line 214 (after the `.terminal-container` rule):

```css
/* Status indicator animations */
@keyframes pulse-glow-green {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(34, 197, 94, 0.6);
  }
  50% {
    box-shadow: 0 0 4px 2px rgba(34, 197, 94, 0.3);
  }
}

@keyframes pulse-glow-yellow {
  0%, 100% {
    box-shadow: 0 0 0 0 rgba(234, 179, 8, 0.6);
  }
  50% {
    box-shadow: 0 0 4px 2px rgba(234, 179, 8, 0.3);
  }
}

.animate-pulse-green {
  animation: pulse-glow-green 2s ease-in-out infinite;
}

.animate-pulse-yellow {
  animation: pulse-glow-yellow 2s ease-in-out infinite;
}
```

Note: Two separate keyframes are needed because `box-shadow` color must match the dot color. Green uses `rgb(34, 197, 94)` (Tailwind `green-500`), yellow uses `rgb(234, 179, 8)` (Tailwind `yellow-500`).

**Step 2: Verify the CSS loads**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus/web && npx vite build`

Expected: Build succeeds with no CSS errors.

**Step 3: Commit**

```bash
git add web/src/globals.css
git commit -m "feat: add pulse-glow CSS animations for status indicators"
```

---

### Task 3: Update the SessionList status rendering

**Files:**
- Modify: `web/src/components/SessionList/index.tsx`

**Step 1: Update `getStatusColor` to also return animation classes**

Replace the `getStatusColor` function (lines 15-30) with two functions:

```typescript
function getStatusColor(status?: string) {
  switch (status) {
    case "running":
      return "bg-green-500";
    case "waiting":
      return "bg-yellow-500";
    case "idle":
      return "bg-muted-foreground";
    case "error":
      return "bg-red-500";
    case "dead":
      return "bg-red-500/50";
    default:
      return "bg-muted-foreground/40";
  }
}

function getStatusAnimation(status?: string) {
  switch (status) {
    case "running":
      return "animate-pulse-green";
    case "waiting":
      return "animate-pulse-yellow";
    default:
      return "";
  }
}

function getStatusLabel(status?: string) {
  switch (status) {
    case "running":
      return "Running";
    case "waiting":
      return "Needs input";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    case "error":
      return "Error";
    default:
      return "";
  }
}
```

**Step 2: Update the status indicator rendering**

Find the inline status indicator (lines 197-207):

```tsx
<div className="mt-0.5 flex items-center gap-1.5">
  <div
    className={cn(
      "h-1.5 w-1.5 flex-shrink-0 rounded-full",
      getStatusColor(status?.status)
    )}
  />
  <span className="text-muted-foreground text-xs">
    {formatRelativeTime(session.updated_at)}
  </span>
</div>
```

Replace with:

```tsx
<div className="mt-0.5 flex items-center gap-1.5">
  <div
    className={cn(
      "h-1.5 w-1.5 flex-shrink-0 rounded-full",
      getStatusColor(status?.status),
      getStatusAnimation(status?.status)
    )}
  />
  <span className="text-muted-foreground text-xs">
    {getStatusLabel(status?.status)}
    {getStatusLabel(status?.status) && " · "}
    {formatRelativeTime(session.updated_at)}
  </span>
</div>
```

This renders as: `● Running · 5m ago` or `● Idle · just now`. The label and middle-dot separator only appear when there's a status; the fallback (no status data yet) shows just the timestamp.

**Step 3: Verify the build**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus/web && npx vite build`

Expected: Build succeeds, no TypeScript errors.

**Step 4: Commit**

```bash
git add web/src/components/SessionList/index.tsx
git commit -m "feat: add animated status dots and labels to session list

Status indicators now show a text label (Running, Needs input, Idle,
Dead, Error) alongside the colored dot. Running and waiting dots
pulse with a colored glow animation."
```

---

### Task 4: Add STATUS column to CLI `session ls`

**Files:**
- Modify: `cmd/argus/cli/session_list.go`

**Step 1: Add a status info struct and fetch status data**

Replace the entire `newListCmd` function in `cmd/argus/cli/session_list.go` (lines 13-55) with:

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
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			if len(resp.Sessions) == 0 {
				fmt.Println("No sessions.")
				return nil
			}

			// Fetch session statuses (best-effort — don't fail if unavailable)
			statuses := make(map[string]string)
			if statusBody, err := c.get("/api/sessions/status"); err == nil {
				var statusResp struct {
					Statuses map[string]struct {
						Status string `json:"status"`
					} `json:"statuses"`
				}
				if err := json.Unmarshal(statusBody, &statusResp); err == nil {
					for id, s := range statusResp.Statuses {
						statuses[id] = s.Status
					}
				}
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSTATUS\tPROVIDER\tUPDATED")
			for _, s := range resp.Sessions {
				st := statuses[s.ID]
				if st == "" {
					st = "-"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.ID, s.Name, st, s.AgentType, relativeTime(s.UpdatedAt))
			}
			w.Flush()
			return nil
		},
	}
}
```

Key decisions:
- Status fetch is best-effort: if it fails (e.g., status detector not configured), the column shows `-` instead of failing the command.
- STATUS column is placed after NAME and before PROVIDER for visual grouping.

**Step 2: Build the CLI**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus && go build ./...`

Expected: Clean build.

**Step 3: Commit**

```bash
git add cmd/argus/cli/session_list.go
git commit -m "feat: add STATUS column to session ls output

The ls command now fetches session statuses and displays them
alongside name and provider. Status fetch is best-effort so the
command still works if the status detector is unavailable."
```

---

### Task 5: Manual verification

**Step 1: Start the agent and verify the web UI**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus && go run ./cmd/argus`

Open the web UI. Verify:
- Idle sessions show a grey static dot with "Idle · Xm ago"
- Running sessions show a green pulsing dot with "Running · Xm ago"
- Sessions waiting for input show a yellow pulsing dot with "Needs input · Xm ago"
- The pulse animation is smooth and subtle (not distracting)

**Step 2: Verify the CLI**

In another terminal:

Run: `argus session ls`

Expected output includes the STATUS column:
```
ID          NAME              STATUS     PROVIDER     UPDATED
...         ...               idle       claude       just now
```

**Step 3: Verify state transitions**

1. Create a session and leave it idle → should show "Idle" (grey, static)
2. Start a task in the session → should transition to "Running" (green, pulsing)
3. Wait for the agent to ask a question → should show "Needs input" (yellow, pulsing)
4. Answer the question → should transition back to "Running" then "Idle"

**Step 4: Run all Go tests**

Run: `cd /Users/jeevb/Workspace/repos/bxnlabs/argus && go test ./...`

Expected: All tests pass.
