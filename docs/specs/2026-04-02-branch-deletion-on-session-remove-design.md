# Branch Deletion on Session Remove (BXN-27)

## Problem

When a session is deleted, the git worktree is removed but the local branch is intentionally preserved for recovery. Over time, orphaned branches accumulate. Users need a convenient way to delete the branch at session-removal time, from both the CLI and web UI.

## Design

Add an optional `deleteBranch` parameter to the session deletion flow. Branch deletion is safe by default — it refuses to delete branches with unmerged commits unless `force` is also set. This mirrors the existing `force` pattern used for dirty worktree checks.

Only local branch deletion is in scope. Remote tracking branches are not affected.

## Changes

### Backend

#### `internal/git/worktree/manager.go`

New sentinel error and method:

```go
var ErrBranchNotMerged = errors.New("branch has unmerged commits")

func (m *Manager) DeleteBranch(repoDir, branch string, force bool) error
```

- `force=false`: runs `git branch -d <branch>` — fails with `ErrBranchNotMerged` if the branch has commits not merged into the upstream or HEAD.
- `force=true`: runs `git branch -D <branch>` — deletes unconditionally.

#### `internal/node/session/lifecycle.go`

Update `Manager.Delete` signature:

```go
func (m *Manager) Delete(id string, force, deleteBranch bool) error
```

After the worktree is removed (existing line 375-378), if `deleteBranch` is true and `session.WorktreeBranch` is set:

1. Resolve the parent repo directory from `session.GitParentDir` (falling back to `git.FindMainRepo` on the working directory if nil).
2. Call `m.wt.DeleteBranch(repoDir, *session.WorktreeBranch, force)`.

**Error handling:** Branch deletion runs after worktree removal and tmux kill. If it fails (e.g., unmerged branch with `force=false`), the session and worktree are already gone but the branch is preserved — identical to today's behavior. The error is returned to the caller so it can be surfaced to the user.

#### `internal/node/api/sessions.go`

Parse new query parameter on `DELETE /api/sessions/{id}`:

```
delete_branch=true
```

Pass to `manager.Delete(id, force, deleteBranch)`. Map `ErrBranchNotMerged` to HTTP 409 Conflict with a descriptive message.

### CLI

#### `cmd/argus/cli/session_delete.go`

Add `--delete-branch` boolean flag. When set, append `&delete_branch=true` to the endpoint URL.

Combined with existing `--force` flag to force-delete unmerged branches:

```
argus session rm my-session                        # delete session, keep branch
argus session rm my-session --delete-branch        # delete session + branch (fails if unmerged)
argus session rm my-session --delete-branch --force # delete session + branch (even if unmerged)
```

### Web UI

#### `web/src/components/SessionList/index.tsx`

Add a second context menu item for sessions with a `worktree_branch`:

- **"Delete"** — existing behavior, deletes session only
- **"Delete with branch"** — deletes session and its branch

Both use `confirm()` for confirmation. The "Delete with branch" item is styled red like the existing "Delete" item.

**Note:** The web UI always sends `force=true`, so "Delete with branch" from the UI will force-delete even unmerged branches. This is consistent with the existing UI behavior (force=true for dirty worktree checks). The CLI is the surface where users get the safe-by-default unmerged check.

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

## Testing

- **`internal/git/worktree/manager_test.go`**: Test `DeleteBranch` — merged branch deletes cleanly, unmerged branch fails without force, succeeds with force.
- **`internal/node/session/lifecycle.go`**: Test `Delete` with `deleteBranch=true` — verify branch is removed after worktree cleanup; verify unmerged branch returns error when `force=false`.
- **`internal/node/api/sessions.go`**: Test DELETE endpoint with `delete_branch=true` query param, verify 409 on unmerged branch.
