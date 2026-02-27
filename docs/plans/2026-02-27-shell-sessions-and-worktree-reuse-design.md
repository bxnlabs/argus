# Shell Sessions & Worktree Reuse

## Problem

1. Shell sessions (`agent_type = "shell"`) create git worktrees unnecessarily. A plain terminal doesn't need branch isolation.
2. When a worktree for the target branch already exists (inside or outside `~/.argus`), Argus creates a duplicate with a `-2` suffix instead of reusing it.
3. Agent types are bare strings scattered across the codebase, making it easy to introduce typos.

## Design

### 1. Agent Type Enum (Go)

Introduce a typed `AgentType` in `internal/agent/provider`:

```go
type AgentType string

const (
    AgentClaude AgentType = "claude"
    AgentCodex  AgentType = "codex"
    AgentGemini AgentType = "gemini"
    AgentShell  AgentType = "shell"
)
```

Changes:
- `Provider.ID` → `AgentType` instead of `string`
- `providers` map key → `AgentType`
- `IsValid`, `Get`, `BuildCommand`, `All` accept/return `AgentType`
- `CreateOptions.AgentType` → `provider.AgentType`
- `Session.AgentType` stays `string` in the DB model (SQLite stores strings); conversion happens at the boundary
- Frontend `AgentType` union type is already correct — no TS changes needed

### 2. Shell Sessions Skip Worktree Creation

`resolveSourceToCWD` gains an `agentType provider.AgentType` parameter. When `agentType == provider.AgentShell`:

| Source | Behavior |
|--------|----------|
| Empty | Home directory (unchanged) |
| Local path, not in git repo | Use path directly (unchanged) |
| Local path, inside git repo | Use resolved path directly. No worktree. `WorktreeBranch` = nil |
| Remote URL | Clone or reuse existing clone, use clone dir as cwd. No worktree. `WorktreeBranch` = nil |

For the remote case, extract clone-or-fetch logic from `worktree.Manager.CreateForRemoteRepo` into a separate method `EnsureClone(src *source.Source) (cloneDir string, err error)` so shell sessions can clone without creating a worktree.

### 3. Reuse Existing Worktrees

Add `FindWorktree(repoDir, branch string) (string, error)` to `worktree.Manager`:

- Runs `git worktree list` in `repoDir`
- `repoDir` can be the main repo root OR an existing worktree path (both work with `git worktree list`)
- Parses output to find a worktree checked out on the target branch
- Returns worktree path if found, empty string if not

Detection logic (using awk-style parsing of `git worktree list` output):
```
git worktree list | awk -v branch="[<branch>]" '$3 == branch { print $1; exit }'
```

#### In `createWorktree`

After computing the branch name via `uniqueBranch`, before `git worktree add`:
1. Call `FindWorktree(repoDir, branch)`
2. If found → return existing path + branch (skip creation)
3. If not → create as before

#### In `resolveSourceToCWD`

For non-shell agent sessions with a local source, after `findGitRoot`:
- Check if the resolved path itself is already a worktree (via `FindWorktree` — check if the path appears in `git worktree list` output)
- If it is → use it directly, extract the branch from the worktree list output, skip creating a new worktree

## What stays unchanged

- Worktree deletion on session delete — current behavior preserved
- Worktree cleanup on failed session creation — current rollback preserved
- Confirmation dialog in UI (`useSessions.ts`) — untouched
- `force` flag on `Delete` — stays as-is
- Frontend types — `AgentType` union already correct

## Key files

| File | Change |
|------|--------|
| `internal/agent/provider/provider.go` | `AgentType` enum, update signatures |
| `internal/agent/provider/{claude,codex,gemini,shell}.go` | Use `AgentType` constants for `ID` |
| `internal/agent/session/lifecycle.go` | Pass `agentType` to `resolveSourceToCWD`, shell skip logic |
| `internal/worktree/manager.go` | `FindWorktree`, `EnsureClone`, reuse logic in `createWorktree` |
| `internal/agent/db/models.go` | `AgentType` field stays string (DB boundary) |
| `internal/agent/api/sessions.go` | Convert string → `AgentType` at API boundary |
| `cmd/argus/cli/session_create.go` | Use `AgentType` constant for default |
| Tests | Update string literals to use constants |
