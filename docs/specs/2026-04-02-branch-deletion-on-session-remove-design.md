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

Rather than relying on `git branch -d` (which checks against ambient `HEAD` — unreliable for Argus branches that have no upstream) or `git merge-base --is-ancestor` (which fails for squash-merged branches since the original commits are never ancestors of the squash commit), the merge check compares trees directly:

1. Resolve the repo's default branch via `git.DefaultBranch(repoDir)` (already exists)
2. Run `git diff <defaultBranch> <branch> --quiet`
3. If the diff is non-empty (exit code 1), the branch has changes not yet in the default branch

This works correctly for all merge strategies — regular merge, squash-and-merge, cherry-pick, and rebase. It answers the question "are the branch's changes already in the default branch?" regardless of how they got there.

**Implementation note:** `git diff --quiet` uses exit code 1 to signal differences (not stderr). The existing `git.Output` helper treats any non-zero exit as an error. `IsBranchMerged` must use `exec.Command` directly and treat exit code 1 as `merged=false, err=nil`, reserving errors for actual command failures (exit code 128, etc.).

## Changes

### Backend

#### `internal/git/worktree/manager.go`

New sentinel error and methods:

```go
var ErrBranchNotMerged = errors.New("branch has unmerged commits")

// IsBranchMerged checks whether branch's changes are already in the repo's default branch.
// Uses git diff --quiet; exit code 1 means differences exist (returns false, nil).
func (m *Manager) IsBranchMerged(repoDir, branch string) (bool, error)

// DeleteBranch force-deletes a local branch (git branch -D).
// Callers are responsible for checking merge status beforehand.
func (m *Manager) DeleteBranch(repoDir, branch string) error
```

`IsBranchMerged` runs `git diff <defaultBranch> <branch> --quiet`. `DeleteBranch` always uses `git branch -D` — the merge safety decision is made by the caller (the preflight in `Manager.Delete`), not by git.

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
   - **New — eligibility check**: If `deleteBranch` is requested, first evaluate whether branch deletion is eligible: `session.WorktreeBranch` must be set, `IsManaged` must be true, and `CountSessionsByWorkingDir` must be 0. If not eligible, clear `deleteBranch` (silently skip — the session is deleted normally).
   - **New — repo resolution**: If still eligible, resolve `repoDir` from `session.GitParentDir` (required). If `GitParentDir` is nil, return a user-fixable error.
   - **New — merge check**: If not `forceBranch`, run `IsBranchMerged`. If not merged, return `ErrBranchNotMerged`. Session remains fully intact.
2. **Destructive phase** (existing):
   - Run pre_destroy hooks
   - Kill tmux
   - Remove worktree
   - **New**: Delete branch (`git branch -D`). If this fails unexpectedly (race, repo corruption), log the error and continue to DB deletion. The branch survives — identical to today's default behavior. This is best-effort, not transactional.
   - Delete DB record

The preflight phase catches all user-fixable errors (unmerged branch, missing GitParentDir) before any side effects. The only remaining failure mode in the destructive phase is unexpected infrastructure errors during `git branch -D`, which are handled best-effort.

#### `internal/node/api/sessions.go`

Parse new query parameters on `DELETE /api/sessions/{id}`:

```
delete_branch=true
force_branch=true
```

Pass to `manager.Delete(id, force, DeleteOpts{...})`. Map `ErrBranchNotMerged` to HTTP 409 Conflict with a descriptive message including the branch name and default branch name.

The success response includes a `branch_deleted` field:

```json
{"success": true, "branch_deleted": true}
```

When branch deletion was requested but skipped (ineligible or best-effort failure):

```json
{"success": true, "branch_deleted": false}
```

This allows clients to distinguish "session deleted + branch deleted" from "session deleted + branch preserved."

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

The CLI reads `branch_deleted` from the response and prints an appropriate message:

```
Deleted session "my-session"
Deleted branch "jeev/my-feature"
```

Or when the branch was preserved:

```
Deleted session "my-session"
Branch "jeev/my-feature" was not deleted (not eligible or could not be removed)
```

### Web UI

#### `web/src/components/SessionList/index.tsx`

Add a second context menu item for sessions with a `worktree_branch`:

- **"Delete"** — existing behavior, deletes session only
- **"Delete with branch"** — deletes session and its branch

Both use `confirm()` for confirmation. The "Delete with branch" item is styled red like the existing "Delete" item. Only shown when the session has a `worktree_branch`.

When the API returns a 409 (unmerged branch), the web UI shows a specific message: "Branch `<name>` was not deleted because it has unmerged commits. Use the CLI with `--force-branch` to delete it, or merge the branch first."

The web UI sends `delete_branch=true` without `force_branch=true`, so it gets safe-by-default branch deletion.

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

- `IsBranchMerged`: branch with no diff against default returns true; branch with unmerged changes returns false; handles exit code 1 correctly (not an error); works correctly for squash-merged branches
- `DeleteBranch`: successfully deletes a branch; returns error if branch doesn't exist

### `internal/node/session/lifecycle_test.go`

- `Delete` with `deleteBranch=true`, merged branch: session + worktree + branch all removed
- `Delete` with `deleteBranch=true`, unmerged branch, `forceBranch=false`: returns `ErrBranchNotMerged`, session/worktree/tmux untouched
- `Delete` with `deleteBranch=true`, unmerged branch, `forceBranch=true`: all removed
- `Delete` with `deleteBranch=true`, shared worktree (`others > 0`): branch deletion skipped, session deleted normally
- `Delete` with `deleteBranch=true`, non-managed worktree (`IsManaged=false`): branch deletion skipped, session deleted normally
- `Delete` with `deleteBranch=true`, `GitParentDir=nil` (eligible session): returns user-fixable error, session untouched
- `Delete` with `deleteBranch=true`, `git branch -D` fails unexpectedly: error logged, DB record still deleted, `branch_deleted=false`

### `internal/node/api/sessions_test.go`

- DELETE with `delete_branch=true`: branch deleted, response includes `branch_deleted: true`
- DELETE with `delete_branch=true`, unmerged branch: returns 409 with branch name and default branch in message
- DELETE with `delete_branch=true&force_branch=true`, unmerged branch: succeeds
- DELETE with `delete_branch=true`, ineligible session: succeeds with `branch_deleted: false`
