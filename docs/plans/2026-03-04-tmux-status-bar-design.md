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
 Argus |                          main | ~/Workspace/repos/bxnlabs/argus
```

**Non-git sessions (no branch):**

```
 Argus |                               ~/Workspace/repos/bxnlabs/argus
```

- Left side unchanged: ` Argus | ` in Catppuccin purple
- Right side: `{branch} | {directory}` — branch in purple, divider in overlay0, directory in blue
- Branch appears first (leftmost) so that on width reduction the directory (rightmost) remains visible

### Truncation

Both values are truncated in Go before being set on tmux:

- **Directory (max 30 chars):** `$HOME` replaced with `~`, then left-truncated with `…` prefix
  (e.g., `…repos/bxnlabs/argus`) since rightmost path components are most meaningful
- **Branch (max 20 chars):** right-truncated with `…` suffix
  (e.g., `feat/some-long-bran…`) since the prefix is most meaningful

### Approach

Pass directory and branch strings into `ConfigureSession`. The caller already has
`GitParentDir` and `WorktreeBranch` from the database. Format the status-right
string in Go with truncation applied, then set it as a literal tmux option.

No tmux format variables, environment variables, or status-interval scripts needed.

## Code Changes

1. **`tmux.go`** — Change `ConfigureSession(name string)` to
   `ConfigureSession(name, dir, branch string)`. Add `truncateDir` and
   `truncateBranch` helper functions. Build `status-right` dynamically.
2. **`lifecycle.go`** — Update both call sites (line ~128 in `Create`, line ~391
   in `EnsureSession`) to pass `GitParentDir` and `WorktreeBranch`.
3. **`tmux_test.go`** — Add tests for truncation helpers.

## Decisions

- **Static display** — set once at creation, not dynamically updated
- **Approach A** — format strings in Go, bake into tmux option (simplest, no overhead)
- **Branch first** — branch | directory ordering so directory survives width reduction
- **`|` dividers** — consistent with existing Catppuccin styling
