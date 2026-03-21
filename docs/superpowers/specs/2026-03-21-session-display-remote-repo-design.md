# Design: Show Remote Repo in Session List

## Problem

The session card in the sidebar shows the working directory as a compressed filesystem path (e.g., `~/…/gitrepo`). This is not informative — users care about *which repo* a session is for, not where it lives on disk.

## Solution

Store the git remote origin URL on every git-backed session. The frontend parses `org/repo` from it and displays that instead of the filesystem path.

## Backend Changes

### Database

- Add nullable `git_remote_url TEXT` column to the `sessions` table via migration.
- Existing sessions get `NULL` — they continue showing the compressed path.

### Session Creation (`internal/node/session/lifecycle.go`)

- Applies to **all sessions where `cwd` is inside a git repo**, regardless of provider type (claude, codex, gemini, shell) or whether a worktree was created.
- After resolving the working directory and `git_parent_dir`, run `git remote get-url origin` on the repo directory (`git_parent_dir` if set, otherwise `cwd`).
- Store the result (full URL — HTTPS or SSH) in the new column. If the command fails (no remote, not a git repo), leave it `NULL`.

### Backfill (`internal/node/session/lifecycle.go`)

- Add `BackfillGitRemoteURL()` following the existing `BackfillGitParentDir()` pattern.
- For sessions with `git_remote_url IS NULL` that have a `working_directory` inside a git repo, resolve and populate the remote URL.
- Best-effort: silently skip sessions whose directories no longer exist or have no remote.

### Model (`internal/node/db/models.go`)

- Add `GitRemoteURL *string json:"git_remote_url"` to `Session`.

### DB Layer (`internal/node/db/sessions.go`)

- Include `git_remote_url` in INSERT, SELECT, and UPDATE queries.

### Git Utility (`internal/git/utils.go`)

- Add `RemoteURL(dir string) (string, error)` — wraps `git remote get-url origin`.

## Frontend Changes

### Types (`web/src/types.ts`)

- Add `git_remote_url: string | null` to `Session`.

### Utility (`web/src/lib/utils.ts`)

- Add `parseRepoFromRemoteURL(url: string): string | null` that extracts `org/repo` from:
  - `https://github.com/org/repo.git` or `https://github.com/org/repo`
  - `git@github.com:org/repo.git` or `git@github.com:org/repo`
  - Other hosts (e.g., `gitlab.com`) — same patterns.
- Returns `null` if the URL doesn't match expected patterns.

### Session Card (`web/src/components/SessionList/index.tsx`)

- On the directory line (line 247-258): if `session.git_remote_url` is present and `parseRepoFromRemoteURL` returns a value, display `org/repo` with `FolderGit2` icon.
- Otherwise, fall back to current `compressPath` behavior.

## No Changes To

- Branch line display (stays as-is with `GitBranch` icon).
- Session creation API contract (`git_remote_url` is derived server-side, not user-supplied).
- Non-git sessions (no remote URL, keep showing path).

## Edge Cases

- **No remote configured:** `git remote get-url origin` fails — store `NULL`, fall back to path display.
- **Non-GitHub hosts:** The parser is generic (handles any `host/org/repo` pattern), so GitLab, Bitbucket, etc. work the same way.
- **Remote URL changes after session creation:** The stored URL reflects the remote at creation time. This is consistent with how `git_parent_dir` works.
