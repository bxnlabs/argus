# Session Lifecycle Hooks

## Problem

Sessions do not inherit the environment of the shell in which they are created.
Argus runs as a long-lived daemon, so sessions inherit the daemon's environment
(`os.Environ()`) rather than the user's shell environment. Custom env vars,
tool activations (nvm, pyenv, virtualenvs), and project-specific setup are lost.

Capturing and replaying the caller's environment is fragile. Instead, introduce
**profiles** with **lifecycle hooks** that declaratively bootstrap sessions.

## Design

### Hook Directory Convention

Hooks are plain shell scripts in a well-known directory structure, resolved
relative to the computed `stateDir` (the parent directory of the database path,
typically `~/.argus`):

```
<stateDir>/profiles/<name>/hooks/
  pre_create.sh
  post_create.sh
  on_create_worktree.sh
  pre_destroy.sh
  post_destroy.sh

<stateDir>/projects/<path-encoded>/hooks/
  (same hook names)
```

Both profile and project hooks follow the `<type>/<name>/hooks/` convention.
Hook paths are always resolved via the injected `stateDir`, never hardcoded.

### Project Key Resolution

The project key (`<path-encoded>`) is derived deterministically from session
state — no additional DB column is needed:

- **Worktree-backed sessions** (have `git_parent_dir`): use
  `source.ParentKeyFromPath(gitParentDir)`
- **Local path sessions**: use `source.ParentKeyFromPath(workingDirectory)`

A new helper `ProjectKeyForSession(session)` encapsulates this logic and is
used consistently for hook resolution at create, ensure, and teardown time.

### Lifecycle Hooks

| Hook                 | When it fires                              | Execution model              | Executable bit? | Blocking? |
|----------------------|--------------------------------------------|------------------------------|-----------------|-----------|
| `pre_create`         | Before tmux session spawns                 | Subprocess from Go           | Yes             | Yes — failure aborts creation |
| `post_create`        | After shell is ready, before agent/user    | Sourced inside session shell | No              | Best-effort (guarded) |
| `on_create_worktree` | After a new git worktree is created        | Subprocess from Go           | Yes             | Yes — failure aborts creation |
| `pre_destroy`        | Before session teardown begins             | Subprocess from Go           | Yes             | Best-effort, errors logged |
| `post_destroy`       | After tmux kill + DB cleanup               | Subprocess from Go           | Yes             | Best-effort, errors logged |

Non-executable or missing hooks are silently skipped.

### Hook Resolution Order

**On create (setup) — general to specific:**

1. `default` profile (if `<stateDir>/profiles/default/hooks/` exists and no explicit profile given)
2. Named profile (if `--profile <name>` specified — replaces `default`)
3. Project hooks (`<stateDir>/projects/<project-key>/hooks/`)

**On teardown — specific to general (LIFO, like Go's `defer`):**

1. Project hooks
2. Named profile (or `default`)

### Execution Model

#### Sourced hooks (`post_create`)

`post_create` is the only hook that is **sourced** rather than executed as a
subprocess. It must run inside the session's shell to modify the environment
(export vars, activate tools, etc.).

Each source is wrapped with error guards to prevent hook failures from killing
the session:

```bash
set +e
source "/path/to/profile/hooks/post_create.sh" 2>&1 || true
source "/path/to/project/hooks/post_create.sh" 2>&1 || true
set -e
```

Calling `exit` inside a `post_create` hook is unsupported and will terminate
the init script. This is documented but not enforced.

All hook paths in generated scripts are always quoted to handle paths with spaces.

#### Subprocess hooks

Subprocess hooks (`pre_create`, `on_create_worktree`, `pre_destroy`,
`post_destroy`) are executed directly via `exec.CommandContext` with a default
timeout of 30 seconds to prevent hanging the daemon. Timeout errors are surfaced
to the caller.

Hooks are self-contained scripts — they run in the daemon's process environment,
not a login shell. If a hook needs specific shell setup, it must source it
explicitly.

Context variables are passed via environment:

- `ARGUS_SESSION_ID` — session identifier
- `ARGUS_WORKING_DIR` — session working directory
- `ARGUS_AGENT_TYPE` — agent type (claude, codex, gemini, shell)
- `ARGUS_WORKTREE_PATH` — worktree path (for `on_create_worktree`)
- `ARGUS_PROFILE` — active profile name

### Create Lifecycle Sequence

The exact execution order during session creation:

```
1. Validate inputs (agent type, profile name)
2. Run pre_create hooks (profile then project)
   → failure aborts, no cleanup needed
3. Resolve source → working directory + worktree
   → sets up rollback defer for worktree cleanup
4. If new worktree was created:
   Run on_create_worktree hooks (profile then project)
   → failure triggers worktree rollback
5. Generate init script (with post_create hooks baked in)
6. Spawn tmux session
7. Configure tmux styling
8. Insert DB record
```

The `on_create_worktree` hook runs in `Create()` after the rollback defer is
installed, not inside `resolveSourceToCWD()`. This ensures worktree cleanup on
hook failure.

### Destroy Lifecycle Sequence

```
1. Look up session from DB
2. Run pre_destroy hooks (project first, then profile — LIFO)
   → best-effort, errors logged
3. Kill tmux session
4. Remove worktree (if managed, no other sessions reference it)
5. Delete DB record
6. Run post_destroy hooks (project first, then profile — LIFO)
   → best-effort, errors logged
```

`pre_destroy` runs while both the tmux session and worktree are still alive,
so hooks can access session state and working directory contents.

### Init Script Changes

#### Agent sessions (claude, codex, gemini)

The existing `GenerateInitScript` is extended to source `post_create` hooks
before launching the agent command:

```bash
#!/bin/bash
rm -f -- "$0"
# ... banner and PATH setup ...

# Source post_create hooks (errors are non-fatal)
set +e
source "/path/to/profile/hooks/post_create.sh" 2>&1 || true
source "/path/to/project/hooks/post_create.sh" 2>&1 || true
set -e

exec claude --session-id ...
```

#### Shell sessions

A new `GenerateShellInitScript` provides a minimal wrapper without the agent
banner, `sleep`, or agent-specific PATH manipulation:

```bash
#!/bin/bash
rm -f -- "$0"

# Source post_create hooks (errors are non-fatal)
set +e
source "/path/to/profile/hooks/post_create.sh" 2>&1 || true
source "/path/to/project/hooks/post_create.sh" 2>&1 || true
set -e

exec $SHELL -l
```

Shell sessions that have no applicable hooks skip the init script entirely
(preserving current behavior of tmux starting a bare default shell).

### Profile Selection

- `--profile <name>` flag on `argus new` and in the `CreateOptions` API
- When no profile is specified, `default` is used if it exists
- When a named profile is specified, it replaces `default` (not additive)
- If `--profile <name>` is specified and the profile directory does not exist,
  return a validation error. The implicit `default` profile is silently skipped
  if absent.
- Profile name is stored in the DB session record for teardown hook resolution

#### Profile name validation

Profile names must match `[a-zA-Z0-9_-]+`. Names containing `/`, `..`, or
other path separators are rejected to prevent path traversal. Validation
happens at the API layer before any filesystem access.

#### Backward compatibility

Existing sessions (created before the `profile` column exists) have `NULL`
profile. `NULL` means "legacy/no profile" — no profile hooks run on teardown
or `EnsureSession` recreate. Only newly created sessions get a resolved profile.

### Example Usage

```
# Create a profile
mkdir -p ~/.argus/profiles/work/hooks

cat > ~/.argus/profiles/work/hooks/post_create.sh << 'EOF'
export GITHUB_TOKEN="$(gh auth token)"
export NVM_DIR="$HOME/.nvm"
[ -s "$NVM_DIR/nvm.sh" ] && source "$NVM_DIR/nvm.sh"
nvm use 18
EOF

# Create project hooks
mkdir -p ~/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/hooks

cat > ~/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/hooks/on_create_worktree.sh << 'EOF'
#!/bin/bash
cd "$ARGUS_WORKTREE_PATH"
npm install
cp ../.env .env
git submodule update --init
EOF
chmod +x ~/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/hooks/on_create_worktree.sh
```

### Changes Required

**Go daemon (`internal/agent/session/`):**
- New `hooks.go` — hook discovery (`ResolveHooks`, `ProjectKeyForSession`),
  subprocess execution with `exec.CommandContext` + 30s timeout, env var
  injection, profile name validation
- `lifecycle.go` — inject `stateDir` into `Manager`; call `pre_create` before
  `NewSession`; run `on_create_worktree` in `Create()` after rollback defer
  (not in `resolveSourceToCWD`); reorder `Delete` to run `pre_destroy` before
  tmux kill; call `post_destroy` after DB delete
- `initscript.go` — extend `GenerateInitScript` to accept hook paths and source
  them with error guards; add `GenerateShellInitScript` for shell sessions
- `lifecycle.go:EnsureSession` — regenerate init script with hooks on recreate

**CLI (`cmd/argus/cli/`):**
- `session_create.go` — add `--profile` flag, pass to API

**API (`internal/agent/api/`):**
- `sessions.go` — accept `profile` field in create request, validate profile
  name, pass to manager

**Database:**
- Add `profile` column to sessions table (nullable TEXT) for teardown resolution
- Migration in `migrations.go`, schema update in `schema.go`
- Update `sessionColumns`, `scanSession`, `CreateSession` in `sessions.go`
- Update `Session` model in `models.go`

**No frontend changes required** for the initial implementation. Profile
selection in the web UI can be added later.
