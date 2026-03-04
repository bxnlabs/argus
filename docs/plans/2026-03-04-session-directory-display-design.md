# Session Directory & Branch Display

## Problem

Sessions lack directory context in both the UI and CLI. Users can't tell at a glance which repository or directory a session is working in. For git worktree sessions, the `working_directory` points to the managed worktree path (under `~/.argus/projects/.../worktrees/`), which is meaningless to the user — they need to see the parent repository path instead.

## Design

### API: Compute `git_parent_dir` at display time

The `/api/sessions` handler adds a computed `git_parent_dir` field to each session in the JSON response:

- **Worktree sessions** (`worktree_branch != null`): call `git.FindMainRepo(working_directory)` to resolve the git parent repository root. Returns the raw absolute path.
- **Non-worktree sessions**: `git_parent_dir` is `null`. Clients use `working_directory` directly.
- **Error fallback**: if `FindMainRepo` fails (deleted path, git unavailable), `git_parent_dir` is `null`.

No database schema change. The field is computed per-request in a response wrapper type.

```json
{
  "id": "sess_abc123",
  "working_directory": "/Users/jeevb/.argus/projects/.../worktrees/my-session",
  "worktree_branch": "feat/new-feature",
  "git_parent_dir": "/Users/jeevb/Workspace/repos/bxnlabs/argus"
}
```

### Path compression (client-side)

Both CLI and web UI apply the same algorithm, implemented independently in Go and TypeScript:

1. Replace `$HOME` prefix with `~`
2. If path length exceeds a threshold:
   - Keep the first segment after `~` or `/`
   - Keep the last 2 path segments
   - Join with `/.../`
   - e.g. `/Users/jeevb/Workspace/repos/bxnlabs/argus` -> `~/Workspace/.../bxnlabs/argus`
3. If still too long: `~/.../last-two-segments`

The threshold may differ between CLI (wider) and web UI (narrower sidebar). CSS `truncate` is the final safety net in the web UI.

### CLI: New DIRECTORY column

Column order: `ID | NAME | STATUS | PROVIDER | DIRECTORY | BRANCH | UPDATED`

```
ID         NAME            STATUS    PROVIDER  DIRECTORY                    BRANCH              UPDATED
sess_abc   my-feature      running   claude    ~/Workspace/.../argus        feat/new-feature    2 min ago
sess_def   shell-session   idle      shell     ~/tmp/scratch                                    5 min ago
```

- Worktree sessions: display compressed `git_parent_dir`
- Non-worktree sessions: display compressed `working_directory`
- Fallback: `-`

### Web UI: Four-line session item

```
+------------------------------------+
| my-feature-session              ... |
| * Running . 2 min ago              |
| ~/Workspace/.../bxnlabs/argus      |  <- line 3: directory
| > feat/new-feature                  |  <- line 4: branch (worktree only)
+------------------------------------+
```

- **Line 3** (always shown): compressed directory path. Worktree sessions show `git_parent_dir`, others show `working_directory`. Styled `text-muted-foreground text-xs` with CSS `truncate`.
- **Line 4** (conditional): shown only when `worktree_branch` is non-null. Prefixed with a branch indicator. Same muted/xs styling with `truncate`.

TypeScript `Session` type gets a new field: `git_parent_dir: string | null`.

## Files to modify

- `internal/agent/api/sessions.go` — add `git_parent_dir` computation to session list/get responses
- `cmd/argus/cli/session_list.go` — add DIRECTORY column, implement path compression
- `cmd/argus/cli/resolve.go` — add `GitParentDir` to `sessionInfo` struct
- `web/src/types.ts` — add `git_parent_dir` field to `Session` interface
- `web/src/components/SessionList/index.tsx` — add lines 3-4 for directory/branch display
- `web/src/lib/utils.ts` (or new file) — path compression utility function
