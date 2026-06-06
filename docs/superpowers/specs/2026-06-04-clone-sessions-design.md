# Clone Sessions (BXN-107)

## Goal

Let a user clone an existing Argus session so two CLI instances run against the
same context — the same repo, worktree, and profile — as independent agents
working in parallel.

## Background

Argus sessions already support sharing a single worktree:

- `resolveSourceToCWD` (`internal/node/session/lifecycle.go`) detects when a
  source path is an existing git worktree and *reuses* it: no new branch is
  created, `worktreeCreated` is false (so `on_create_worktree` hooks are
  skipped), and `branchCreated` is false on the reusing session.
- The DB layer has `CountSessionsByWorkingDir` and branch-ownership transfer
  (`TransferBranchOwnership`) specifically to manage multiple sessions on one
  worktree, including correct branch cleanup when the last session is deleted.

The data model is therefore already in place. Cloning is a thin convenience
layer: create a new session whose source is the original's working directory,
copying its settings, with its own tmux name and a fresh CLI instance.

## Approach

Add a `Manager.Clone(id)` method that loads the source session and delegates to
the existing `Create` path with `Source` set to the source's working directory.
Reusing `Create` gives correct worktree reuse, hook execution, git-metadata
resolution, and branch-ownership semantics for free.

Two alternatives were rejected:

- **Pure-frontend** (reconstruct a New-Session call client-side): leaks
  worktree-reuse logic into the client; the one-click UX makes the dialog
  unnecessary.
- **Generalize `Create` with a `CloneFromSessionID` field**: muddies the create
  path for no benefit.

## Behavior

Cloning a session:

- Starts a **fresh CLI conversation** — `provider_session_id` is not copied.
- Copies: `provider_type`, `model`, `system_prompt`, `auto_approve`, `profile`.
- Sets the source as `Source = src.WorkingDirectory`, so a worktree-backed
  session reuses the same worktree and branch (no new branch).
- Names the clone `"<name> (copy)"`.
- Is offered for **all** session types. For non-worktree sources (plain local
  path, home directory, shell), `Create` reuses the same working directory —
  still a valid independent instance.

## Components

### Backend

**`Manager.Clone(id string) (*db.Session, error)`** — `internal/node/session/lifecycle.go`

1. Load the source session via `m.db.GetSession(id)`. If nil, return
   `fmt.Errorf("%w: %s", ErrNotFound, id)`.
2. Build `CreateOptions`:
   - `Source: src.WorkingDirectory`
   - `ProviderType: src.ProviderType`
   - `Model: src.Model`
   - `SystemPrompt: src.SystemPrompt`
   - `AutoApprove: src.AutoApprove`
   - `Profile: src.Profile`
   - `Name: src.Name + " (copy)"`
   - `ResumeSessionID` and `Branch` left empty/zero.
3. Delegate to `m.Create(opts)` and return its result.

**`sessionHandler.clone`** — `internal/node/api/sessions.go`

- Reads `id` from the path.
- Calls `h.manager.Clone(id)`.
- On error: `ErrNotFound` → 404, `ErrInvalidInput` → 400, else 500 (mirrors
  `create`).
- On success: arm the watcher via `h.watcherManager.EnsureWatching(...)` and
  respond `201 {"session": sess}` — identical to `create`.

**Router** — `internal/node/api/router.go`

- `mux.HandleFunc("POST /api/sessions/{id}/clone", sh.clone)` alongside the
  other `/api/sessions/{id}` routes.

### Frontend

**`useCloneSession`** — `web/src/data/sessions/queries.ts`

- Mirrors `useCreateSession`: `POST /node/api/sessions/${sessionId}/clone`,
  invalidate the sessions list on success, return the created session so the
  caller can select it.

**Clone menu item** — `web/src/components/SessionList/index.tsx`

- A `DropdownMenuItem` (Copy icon from lucide) placed after Rename / Change
  profile, before Info/Delete.
- `onClick` stops propagation and invokes the clone handler with `session.id`.
- On success the new session is selected, matching the create flow.
- The handler/prop is threaded the same way as `onRenameSession` /
  `onDeleteSession` (callback prop into `SessionList`, wired at the workspace
  level where other session mutations are wired).

## Edge cases

- **Source worktree removed externally**: `Create`'s source resolution fails
  with `ErrInvalidInput` → 400 surfaced to the user. Acceptable.
- **Cloning a clone**: yields `"Foo (copy) (copy)"`. Acceptable; no smart
  numbering (YAGNI).
- **Branch ownership**: the clone has `branch_created = false`, so it never
  owns or deletes the shared branch. If the original is deleted first while the
  clone remains, existing `TransferBranchOwnership` logic moves ownership to the
  surviving sibling — no new code needed.

## Testing

**`internal/node/session/lifecycle_test.go`**
- Clone copies `provider_type`, `model`, `system_prompt`, `auto_approve`,
  `profile`; leaves `provider_session_id` empty.
- Clone of a worktree-backed session reuses the worktree: same
  `working_directory`, same `worktree_branch`, `branch_created = false` on the
  clone.
- Name is suffixed with `" (copy)"`.
- Clone of a missing session returns `ErrNotFound`.

**`internal/node/api/sessions_test.go`**
- `POST /api/sessions/{id}/clone` returns 201 with the new session for a valid
  source.
- Returns 404 for an unknown source id.

**Frontend (`web/src/components/SessionList/index.test.tsx`)**
- Clone menu item renders and fires the clone callback with the session id
  (follow existing menu-item test patterns).

## Out of scope

- Resuming/forking the original agent conversation into the clone.
- Pre-filled New-Session dialog or per-clone setting overrides (provider/model
  changes at clone time).
- Smart clone naming / de-duplication of `(copy)` suffixes.
