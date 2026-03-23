# Design: Show Remote Repo in Session List

## Problem

The session card in the sidebar shows the working directory as a compressed filesystem path (e.g., `~/…/gitrepo`). This is not informative — users care about *which repo* a session is for, not where it lives on disk.

## Solution

Store the git remote origin URL on every git-backed session. The frontend parses `org/repo` from it and displays that instead of the filesystem path.

## Backend Changes

### Database

- Add nullable `git_remote_url TEXT` column to the `sessions` table:
  - Add the column to the `CREATE TABLE` statement in `schema.go`.
  - Add an `add_git_remote_url` migration in `migrations.go` (`ALTER TABLE sessions ADD COLUMN git_remote_url TEXT`).
  - Add a seed entry in `db.go` `seedMigrations()` so fresh databases skip the migration.
- Existing sessions get `NULL` — they continue showing the compressed path.

### Session Creation (`internal/node/session/lifecycle.go`)

- Applies to **all sessions where `cwd` is inside a git repo**, regardless of provider type (claude, codex, gemini, shell) or whether a worktree was created.
- After resolving the working directory and `git_parent_dir`, run `git remote get-url origin` on the repo directory (`git_parent_dir` if set, otherwise `cwd`).
- Strip any userinfo (credentials/tokens) from the URL before storing. For HTTPS URLs, remove the `user:pass@` portion. SSH URLs (`git@host:org/repo`) are safe as-is.
- Store the sanitized URL in the new column. If the command fails (no remote, not a git repo), leave it `NULL`.
- Only the `origin` remote is checked. Repos with remotes named differently get `NULL`.

### Backfill (`internal/node/session/lifecycle.go`)

- Add `BackfillGitRemoteURL()` following the existing `BackfillGitParentDir()` pattern.
- Add `ListSessionsForGitRemoteBackfill()` in `sessions.go` — query sessions with `git_remote_url IS NULL AND (git_parent_dir IS NOT NULL OR worktree_branch IS NOT NULL)`. This targets only sessions known to be in git repos, avoiding perpetual re-probing of non-git sessions.
- For each candidate, resolve the remote URL from `git_parent_dir` (when set) or `working_directory`.
- Best-effort: silently skip sessions whose directories no longer exist or have no remote.
- Call `BackfillGitRemoteURL()` from `setup.go` after `BackfillGitParentDir()`.

### Model (`internal/node/db/models.go`)

- Add `GitRemoteURL *string json:"git_remote_url"` to `Session`.

### DB Layer (`internal/node/db/sessions.go`)

- Add `git_remote_url` to `sessionColumns` (used by SELECT queries) and the `CreateSession` INSERT.
- Add a dedicated `SetGitRemoteURL(id, url string)` setter for backfill, matching the `SetGitParentDir` pattern.
- Do **not** add `git_remote_url` to `SessionUpdate` — it is derived server-side, not user-updatable.

### Git Utility (`internal/git/utils.go`)

- Add `RemoteURL(dir string) (string, error)` — wraps `git remote get-url origin`, trims whitespace from output.

## Frontend Changes

### Types (`web/src/types.ts`)

- Add `git_remote_url: string | null` to `Session`.

### Utility (`web/src/lib/utils.ts`)

- Add `parseRepoFromRemoteURL(url: string): string | null` that extracts the repo path from:
  - `https://github.com/org/repo.git` or `https://github.com/org/repo` → `org/repo`
  - `git@github.com:org/repo.git` or `git@github.com:org/repo` → `org/repo`
  - Other hosts follow the same patterns. For URLs with deeper path segments (e.g., GitLab subgroups like `gitlab.com/group/subgroup/repo.git`), return the full path after the host (`group/subgroup/repo`).
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
- **Non-origin remotes:** Only the `origin` remote is checked. Repos using a different remote name get `NULL` and fall back to path display.
- **Non-GitHub hosts:** The parser extracts the path after the host for any provider. Standard `host/org/repo` URLs produce `org/repo`. GitLab subgroup URLs produce the full path (e.g., `group/subgroup/repo`).
- **Embedded credentials:** HTTPS URLs with userinfo (`https://user:token@host/...`) are sanitized before storage — the userinfo portion is stripped.
- **Remote URL changes after session creation:** The stored URL reflects the remote at creation time. This is consistent with how `git_parent_dir` works.
