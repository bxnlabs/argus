# Tmux Status Bar Cleanup

## Problem

The current tmux status bar shows the session ID (`#S` = `claude-sess_abc123`) and
time (`%H:%M`), neither of which is useful. The session ID is opaque and the time
is already visible in the OS menu bar.

## Design

Replace the right side of the status bar with the git branch and working directory,
set statically at session creation time from database fields.

### Layout

**Git sessions:**

```
 Argus |          sess_m2abc12_xyz789 | main | ~/Workspace/.../bxnlabs/argus
```

**Non-git sessions (no branch):**

```
 Argus |                    sess_m2abc12_xyz789 | ~/Workspace/.../bxnlabs/argus
```

- Left side unchanged: ` Argus | ` in Catppuccin purple
- Right side: `{session_id} | {branch} | {directory}` — ID in subtext, branch in purple, divider in overlay0, directory in blue
- Session ID leftmost (least important), directory rightmost (survives width reduction)

### Truncation

Values are truncated in Go before being set on tmux:

- **Session ID:** shown in full (e.g., `sess_m2abc12_xyz789`)
- **Directory (max 30 chars):** uses `compressPath` (shared package) — `$HOME` replaced
  with `~`, then smart segment compression with `/.../` (e.g., `~/Workspace/.../bxnlabs/argus`)
- **Branch (max 20 chars):** right-truncated with `…` suffix
  (e.g., `feat/some-long-bran…`) since the prefix is most meaningful

### Approach

Pass session ID, directory, branch, and home strings into `ConfigureSession`.
The caller already has `GitParentDir`, `WorktreeBranch`, and session ID from
the database. Format the status-right string in Go, then set it as a literal
tmux option. Reuse `compressPath` from the shared package for directory display.

No tmux format variables, environment variables, or status-interval scripts needed.

## Code Changes

1. **`shared/pathutil.go`** — Move `compressPath` from `cli` to `shared` package.
   Add `TruncateRight` helper.
2. **`cli/pathutil.go`** — Delegate to `shared.CompressPath`.
3. **`tmux.go`** — Change `ConfigureSession(name string)` to
   `ConfigureSession(name, sessionID, dir, branch, home string)`. Add
   `buildStatusRight` that composes session ID, branch, and directory.
4. **`lifecycle.go`** — Update both call sites (line ~128 in `Create`, line ~391
   in `EnsureSession`) to pass all fields.
5. **Tests** — Move `compressPath` tests to shared, add `TruncateRight` and
   `buildStatusRight` tests.

## Decisions

- **Static display** — set once at creation, not dynamically updated
- **Approach A** — format strings in Go, bake into tmux option (simplest, no overhead)
- **Ordering** — session ID | branch | directory — directory rightmost survives width reduction
- **`|` dividers** — consistent with existing Catppuccin styling
- **Reuse `compressPath`** — moved from cli to shared package for smart path compression
- **Full session ID** — shown untruncated as the canonical identifier for CLI commands
