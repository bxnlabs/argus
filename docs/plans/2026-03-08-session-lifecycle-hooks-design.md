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

Hooks are plain shell scripts in a well-known directory structure:

```
~/.argus/profiles/<name>/hooks/
  pre_create.sh
  post_create.sh
  on_create_worktree.sh
  pre_destroy.sh
  post_destroy.sh

~/.argus/projects/<path-encoded>/hooks/
  (same hook names)
```

Profiles live alongside project state under `~/.argus/`. Both profile and project
hooks follow the `<type>/<name>/hooks/` convention.

### Lifecycle Hooks

| Hook                 | When it fires                              | Execution model       | Executable bit required? | Blocking? |
|----------------------|--------------------------------------------|-----------------------|--------------------------|-----------|
| `pre_create`         | Before tmux session spawns                 | Subprocess from Go    | Yes                      | Yes — failure aborts session creation |
| `post_create`        | After shell is ready, before agent/user    | Sourced inside session shell | No                | No — best-effort, errors logged |
| `on_create_worktree` | After a new git worktree is created        | Subprocess from Go    | Yes                      | Yes — failure aborts session creation |
| `pre_destroy`        | Before session is killed                   | Subprocess from Go    | Yes                      | No — best-effort, errors logged |
| `post_destroy`       | After tmux + DB cleanup                    | Subprocess from Go    | Yes                      | No — best-effort, errors logged |

Non-executable or missing hooks are silently skipped.

### Hook Resolution Order

**On create (setup) — general to specific:**

1. `default` profile (if `~/.argus/profiles/default/hooks/` exists and no explicit profile given)
2. Named profile (if `--profile <name>` specified — replaces `default`)
3. Project hooks (`~/.argus/projects/<path-encoded>/hooks/`)

**On teardown — specific to general (LIFO, like Go's `defer`):**

1. Project hooks
2. Named profile (or `default`)

### Execution Model

**`post_create`** is the only hook that is **sourced** rather than executed as a
subprocess. It must run inside the session's shell to modify the environment
(export vars, activate tools, etc.).

**Subprocess hooks** (`pre_create`, `on_create_worktree`, `pre_destroy`,
`post_destroy`) receive session context via environment variables:

- `ARGUS_SESSION_ID` — session identifier
- `ARGUS_WORKING_DIR` — session working directory
- `ARGUS_AGENT_TYPE` — agent type (claude, codex, gemini, shell)
- `ARGUS_WORKTREE_PATH` — worktree path (for `on_create_worktree`)
- `ARGUS_PROFILE` — active profile name

### Init Script Changes

The existing init script mechanism is extended to source `post_create` hooks
before launching the agent command.

**Agent sessions (claude, codex, gemini):**

```bash
# sourced hooks — profile first, then project
source ~/.argus/profiles/default/hooks/post_create.sh
source ~/.argus/projects/--repo--/hooks/post_create.sh
exec claude --session-id ...
```

**Shell sessions:**

Today, shell sessions have no init script — tmux starts a bare default shell.
This changes to a lightweight wrapper:

```bash
source ~/.argus/profiles/default/hooks/post_create.sh
source ~/.argus/projects/--repo--/hooks/post_create.sh
exec $SHELL -l
```

### Profile Selection

- `--profile <name>` flag on `argus new` and in the `CreateOptions` API
- When no profile is specified, `default` is used if it exists
- When a named profile is specified, it replaces `default` (not additive)
- Profile name is stored in the DB session record for teardown hook resolution

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
- New `hooks.go` — hook discovery (resolve profile + project hooks for a given
  hook name), subprocess execution, env var injection
- `lifecycle.go` — call `pre_create` before `NewSession`, `on_create_worktree`
  after worktree creation in `resolveSourceToCWD`, teardown hooks in `Delete`
- `initscript.go` — extend init script template to source `post_create` hooks;
  generate init scripts for shell sessions too

**CLI (`cmd/argus/cli/`):**
- `session_create.go` — add `--profile` flag, pass to API

**API (`internal/agent/api/`):**
- `sessions.go` — accept `profile` field in create request, pass to manager

**Database:**
- Add `profile` column to sessions table (nullable string) for teardown resolution

**No frontend changes required** for the initial implementation. Profile
selection in the web UI can be added later.
