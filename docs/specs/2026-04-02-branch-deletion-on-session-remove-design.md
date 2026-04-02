# Branch Deletion on Session Remove (BXN-27)

## Problem

When a session is deleted, the git worktree is removed but the local branch is intentionally preserved for recovery. Over time, orphaned branches accumulate. Users need a convenient way to delete the branch at session-removal time, from both the CLI and web UI.

## Design

Add an optional `deleteBranch` parameter to the session deletion flow. When requested, the branch is always force-deleted (`git branch -D`). No merge check is performed — the user explicitly opted in to branch deletion.

Only local branch deletion is in scope. Remote tracking branches are not affected.

### Ownership gating

Branch deletion is only attempted when all of the following are true:

1. `deleteBranch` is requested
2. `session.WorktreeBranch` is set
3. The worktree is Argus-managed (`IsManaged` returns true)
4. No other sessions share the same working directory (`CountSessionsByWorkingDir == 0`)

This mirrors the existing gates for worktree removal. If not eligible, branch deletion is silently skipped — the session is deleted normally.

## Changes

### Backend

#### `internal/git/worktree/manager.go`

New method:

```go
// DeleteBranch force-deletes a local branch (git branch -D).
func (m *Manager) DeleteBranch(repoDir, branch string) error
```

#### `internal/node/session/lifecycle.go`

Update `Manager.Delete` signature:

```go
func (m *Manager) Delete(id string, force, deleteBranch bool) error
```

The delete flow becomes:

1. **Preflight phase** (before any side effects):
   - Existing dirty-worktree check (gated on `force`)
   - **New — eligibility check**: If `deleteBranch` is requested, evaluate whether branch deletion is eligible: `session.WorktreeBranch` must be set, `IsManaged` must be true, and `CountSessionsByWorkingDir` must be 0. If not eligible, clear `deleteBranch` (silently skip).
   - **New — repo resolution**: If still eligible, resolve `repoDir` from `session.GitParentDir` (required). If `GitParentDir` is nil, return a user-fixable error.
2. **Destructive phase** (existing):
   - Run pre_destroy hooks
   - Kill tmux
   - Remove worktree
   - **New**: Delete branch (`git branch -D`). If this fails unexpectedly (race, repo corruption), log the error and continue to DB deletion. The branch survives — identical to today's default behavior. This is best-effort, not transactional.
   - Delete DB record

#### `internal/node/api/sessions.go`

Parse new query parameter on `DELETE /api/sessions/{id}`:

```
delete_branch=true
```

Pass to `manager.Delete(id, force, deleteBranch)`.

The success response includes a `branch_deleted` field:

```json
{"success": true, "branch_deleted": true}
```

When branch deletion was requested but skipped (ineligible or best-effort failure):

```json
{"success": true, "branch_deleted": false}
```

### CLI

#### `cmd/argus/cli/session_delete.go`

Add `--delete-branch` flag. Build query parameters using `url.Values` instead of string concatenation:

```
argus session rm my-session                    # delete session, keep branch
argus session rm my-session --delete-branch    # delete session + force-delete branch
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
- **"Delete with branch"** — deletes session and force-deletes its branch

Both use `confirm()` for confirmation. The "Delete with branch" item is styled red like the existing "Delete" item. Only shown when the session has a `worktree_branch`.

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

### `internal/git/worktree/manager_test.go`

- `DeleteBranch`: successfully deletes a branch; returns error if branch doesn't exist

### `internal/node/session/lifecycle_test.go`

- `Delete` with `deleteBranch=true`: session + worktree + branch all removed
- `Delete` with `deleteBranch=true`, shared worktree (`others > 0`): branch deletion skipped, session deleted normally
- `Delete` with `deleteBranch=true`, non-managed worktree (`IsManaged=false`): branch deletion skipped, session deleted normally
- `Delete` with `deleteBranch=true`, `GitParentDir=nil` (eligible session): returns user-fixable error, session untouched
- `Delete` with `deleteBranch=true`, `git branch -D` fails unexpectedly: error logged, DB record still deleted, `branch_deleted=false`

### `internal/node/api/sessions_test.go`

- DELETE with `delete_branch=true`: branch deleted, response includes `branch_deleted: true`
- DELETE with `delete_branch=true`, ineligible session: succeeds with `branch_deleted: false`
