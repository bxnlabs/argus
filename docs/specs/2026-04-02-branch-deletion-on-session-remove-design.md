# Branch Deletion on Session Remove (BXN-27)

## Problem

When a session is deleted, the git worktree is removed but the local branch is intentionally preserved for recovery. Over time, orphaned branches accumulate. Users need a convenient way to delete the branch at session-removal time, from both the CLI and web UI.

## Design

Add an optional `deleteBranch` parameter to the session deletion flow. Branch deletion is safe by default — it refuses to delete branches with unmerged commits unless force-branch is also set. The safety check uses an explicit ancestry test against the repo's default branch, not `git branch -d` semantics.

Only local branch deletion is in scope. Remote tracking branches are not affected.

### Safety model

Two independent force controls:

- **`force`** (existing) — controls dirty worktree checks
- **`forceBranch`** (new) — controls unmerged branch deletion

These are separate because the web UI hardcodes `force=true` for worktree checks but should still get safe-by-default branch deletion. The CLI exposes both independently.

### Ownership gating

Branch deletion is only attempted when all of the following are true:

1. `deleteBranch` is requested
2. `session.WorktreeBranch` is set
3. The worktree is Argus-managed (`IsManaged` returns true)
4. No other sessions share the same working directory (`CountSessionsByWorkingDir == 0`)

This mirrors the existing gates for worktree removal.

### Merge safety check

Rather than relying on `git branch -d` (which checks against ambient `HEAD` — unreliable for Argus branches that have no upstream), the merge check uses an explicit ancestry test:

1. Resolve the repo's default branch via `git.DefaultBranch(repoDir)` (already exists)
2. Run `git merge-base --is-ancestor <branch> <defaultBranch>`
3. If the branch is not an ancestor of the default branch, it has unmerged commits

This gives a stable, deterministic result regardless of what's checked out in the parent repo.

## Changes

### Backend

#### `internal/git/worktree/manager.go`

New sentinel error and methods:

```go
var ErrBranchNotMerged = errors.New("branch has unmerged commits")

// IsBranchMerged checks whether branch is an ancestor of the repo's default branch.
func (m *Manager) IsBranchMerged(repoDir, branch string) (bool, error)

// DeleteBranch force-deletes a local branch (git branch -D).
// Callers are responsible for checking merge status beforehand.
func (m *Manager) DeleteBranch(repoDir, branch string) error
```

`IsBranchMerged` runs `git merge-base --is-ancestor <branch> <defaultBranch>`. `DeleteBranch` always uses `git branch -D` — the merge safety decision is made by the caller (the preflight in `Manager.Delete`), not by git.

#### `internal/node/session/lifecycle.go`

Update `Manager.Delete` signature:

```go
func (m *Manager) Delete(id string, force bool, opts DeleteOpts) error

type DeleteOpts struct {
    DeleteBranch bool
    ForceBranch  bool
}
```

The delete flow becomes:

1. **Preflight phase** (before any side effects):
   - Existing dirty-worktree check (gated on `force`)
   - **New**: If `deleteBranch` is requested, resolve `repoDir` from `session.GitParentDir` (required — no fallback to `FindMainRepo` on the working dir). If `GitParentDir` is nil, return a user-fixable error.
   - **New**: If `deleteBranch` and not `forceBranch`, run `IsBranchMerged`. If not merged, return `ErrBranchNotMerged`.
   - **New**: Gate on `IsManaged` and `CountSessionsByWorkingDir == 0` (same as worktree removal). If other sessions share the worktree, skip branch deletion silently.
2. **Destructive phase** (existing):
   - Run pre_destroy hooks
   - Kill tmux
   - Remove worktree
   - **New**: Delete branch (unconditional `git branch -D`, since preflight already validated)
   - Delete DB record

Because all safety checks happen in the preflight phase, there is no partial-failure state. If any check fails, the session remains fully intact.

#### `internal/node/api/sessions.go`

Parse new query parameters on `DELETE /api/sessions/{id}`:

```
delete_branch=true
force_branch=true
```

Pass to `manager.Delete(id, force, DeleteOpts{...})`. Map `ErrBranchNotMerged` to HTTP 409 Conflict with a descriptive message including the branch name.

### CLI

#### `cmd/argus/cli/session_delete.go`

Add two new flags:

- `--delete-branch` — request branch deletion
- `--force-branch` — force-delete even if unmerged

Build query parameters using `url.Values` instead of string concatenation:

```
argus session rm my-session                                    # delete session, keep branch
argus session rm my-session --delete-branch                    # delete session + branch (fails if unmerged)
argus session rm my-session --delete-branch --force-branch     # delete session + branch (even if unmerged)
argus session rm my-session --force --delete-branch            # force dirty worktree + safe branch delete
```

### Web UI

#### `web/src/components/SessionList/index.tsx`

Add a second context menu item for sessions with a `worktree_branch`:

- **"Delete"** — existing behavior, deletes session only
- **"Delete with branch"** — deletes session and its branch

Both use `confirm()` for confirmation. The "Delete with branch" item is styled red like the existing "Delete" item. Only shown when the session has a `worktree_branch`.

The web UI sends `delete_branch=true` without `force_branch=true`, so it gets safe-by-default branch deletion. Unmerged branches are preserved with an error surfaced to the user.

#### `web/src/hooks/useSessions.ts`

Update `deleteSession` callback to accept an optional `deleteBranch` parameter:

```ts
const deleteSession = useCallback(
  async (sessionId: string, deleteBranch?: boolean) => { ... }
);
```

Update `onDeleteSession` prop type in `SessionListProps` accordingly.

#### `web/src/data/sessions/queries.ts`

Update `useDeleteSession` mutation to accept `{ sessionId, deleteBranch }`:

```ts
mutationFn: ({ sessionId, deleteBranch }: { sessionId: string; deleteBranch?: boolean }) =>
  apiFetch(`/node/api/sessions/${sessionId}?force=true${deleteBranch ? "&delete_branch=true" : ""}`, {
    method: "DELETE",
  }),
```

Note: `force=true` controls worktree-dirty only. `force_branch` is not sent, so unmerged branches are still protected.

## Testing

### `internal/git/worktree/manager_test.go`

- `IsBranchMerged`: branch merged into default returns true; branch with unmerged commits returns false
- `DeleteBranch`: successfully deletes a branch; returns error if branch doesn't exist

### `internal/node/session/lifecycle_test.go`

- `Delete` with `deleteBranch=true`, merged branch: session + worktree + branch all removed
- `Delete` with `deleteBranch=true`, unmerged branch, `forceBranch=false`: returns `ErrBranchNotMerged`, session/worktree/tmux untouched
- `Delete` with `deleteBranch=true`, unmerged branch, `forceBranch=true`: all removed
- `Delete` with `deleteBranch=true`, shared worktree (`others > 0`): branch deletion silently skipped
- `Delete` with `deleteBranch=true`, non-managed worktree (`IsManaged=false`): branch deletion skipped
- `Delete` with `deleteBranch=true`, `GitParentDir=nil`: returns user-fixable error

### `internal/node/api/sessions_test.go`

- DELETE with `delete_branch=true`: branch deleted on success
- DELETE with `delete_branch=true`, unmerged branch: returns 409 with branch name in message
- DELETE with `delete_branch=true&force_branch=true`, unmerged branch: succeeds
