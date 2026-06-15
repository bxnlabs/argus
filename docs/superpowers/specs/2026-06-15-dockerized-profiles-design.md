# Dockerized Profiles (BXN-111)

## Goal

Let a profile run its session's agent inside a Docker container defined by a
`docker-compose` file, instead of directly on the host. The container becomes a
drop-in replacement for the provider CLI binary: the agent's TUI runs in the
container while everything else (tmux, terminal streaming, status, hooks that
orchestrate host resources) stays on the host, unchanged.

### Motivation

The driving use case is **per-client isolation**. Each client has a separate
Tailscale network, and switching the host between them is a growing hassle. A
dockerized profile lets each client's environment — its tailnet membership,
toolchain, and service stack — live in a compose stack, so "switch client"
becomes "pick that client's profile" instead of reconfiguring the host. The same
mechanism also delivers reproducible per-profile environments, agent sandboxing,
and prod/CI parity.

Argus stays **Tailscale-agnostic**: the profile's compose file is where the user
wires up the client's tailnet (e.g. a `tailscale` sidecar), the image, and any
service stack. Argus only manages the compose lifecycle and runs the agent inside
the stack's container.

## Background

Today a "profile" is a directory under `$ARGUS_HOME/profiles/<name>/` whose
`hooks/` subdir holds lifecycle scripts (`pre_create.sh`, `post_create.sh`,
`on_create_worktree.sh`, `pre_destroy.sh`). A session may be assigned a profile;
its hooks run alongside project-level hooks at create/destroy
(`internal/node/session/hooks.go`).

A session runs a provider CLI (`claude`/`codex`/`gemini`) or a shell inside a
**tmux session** on Argus's dedicated tmux server, in a working directory that is
usually a git worktree. The agent is launched by a generated bash **init script**
(`internal/node/session/initscript.go`): it shows a banner, exports `PATH`,
`source`s the `post_create` hooks, runs the agent command, then captures the
provider session ID from the pane via `tmux capture-pane`. `post_create` is the
only hook that is `source`d (not executed) — specifically so its environment
setup is inherited by the agent process.

There is no Docker code in the repo today.

## Design decisions

These were resolved during brainstorming and drive the rest of the spec:

1. **Stack scope: shared per profile.** All sessions of a profile share one
   long-lived compose stack (one tailnet node per client). Sessions `exec` into
   it rather than each spawning their own stack.
2. **Lifecycle: lazy up, manual down.** The stack comes up on the first session
   that needs it and stays up across session deletes. Teardown is explicit
   (`argus profile down <name>` or `docker compose down`).
3. **Execution model: tmux on host + `docker compose exec`.** The agent runs via
   `docker compose exec` from the host tmux pane. tmux, `capture-pane`, terminal
   streaming, and provider-session-ID capture are untouched — they only ever talk
   to the host tmux server. The *only* thing that changes is the command string
   inside the pane.
4. **Mounts & identity: mount the full host home (and the state dir, if outside
   home) at identical paths, and run as the host user/group.** Because home maps
   1:1, host path == container path everywhere — worktrees, profile hooks, and
   provider credentials all resolve identically, with no per-session mount
   juggling and no separate auth wiring. Files the agent writes are owned by the
   host user, not root.
5. **Hooks: the `source`d hook runs where the agent runs; executed hooks run on
   the host.** `post_create` runs **inside the container** (sourced into the
   in-container shell that launches the agent), preserving its
   "configures-the-agent's-env" semantic. `pre_create`, `on_create_worktree`, and
   `pre_destroy` are executed subprocesses with no env-crossing relationship to
   the agent; they run on the host, which is the superset environment (a host
   hook can always `docker compose exec` into the container, but not vice-versa).

### Approaches considered and rejected

- **One-off `docker compose run` per session** — conflicts with the shared-stack
  decision; N containers and N tailnet nodes for N sessions.
- **tmux inside the container** — would fork Argus's entire host-tmux terminal,
  status, and capture pipeline. Far more invasive for no benefit.
- **All hooks in the container** — `on_create_worktree`/`pre_create`/`pre_destroy`
  are executed subprocesses; the host is strictly more capable for them, and
  running setup in-container forces stack-up-first and loses host context.
- **All hooks on the host (no in-container `post_create`)** — drops
  `post_create`'s primary effect (the agent's env never sees its exports) and
  fractures the init script's single-shell-context invariant.

## What makes a profile "dockerized"

A profile is container-backed when its directory contains a compose file:

```
$ARGUS_HOME/profiles/<name>/
  docker-compose.yml      # presence ⇒ dockerized profile
  hooks/                  # lifecycle hooks (unchanged on disk)
```

- Detection: `docker-compose.yml` or `compose.yaml` present in the profile dir.
- The agent runs in a service named **`agent`** by convention. v1 is
  convention-only (no per-profile config file); an explicit override can be added
  later if needed.
- The compose file is the user's responsibility: it declares the image (with the
  provider CLIs installed), any Tailscale sidecar, the host mounts (using the env
  vars Argus injects, below), and a keep-alive `agent` service (e.g.
  `command: sleep infinity`).

## Compose invocation

Argus invokes compose with a stable, per-profile project name and the profile's
compose file:

```
docker compose -p argus-<name> -f <profile>/docker-compose.yml <subcommand>
```

When invoking, Argus injects a fixed set of env vars the compose file consumes
for its mounts and identity:

- `ARGUS_HOST_HOME` — host `$HOME`; the compose file mounts it at the same path.
- `ARGUS_STATE_DIR` — Argus state root; mounted at the same path (only matters
  when the state dir lives outside home).
- `ARGUS_UID` / `ARGUS_GID` — host user/group.

A minimal compose file therefore looks like:

```yaml
services:
  agent:
    image: my-client-image          # provider CLIs preinstalled
    user: "${ARGUS_UID}:${ARGUS_GID}"
    volumes:
      - ${ARGUS_HOST_HOME}:${ARGUS_HOST_HOME}
      - ${ARGUS_STATE_DIR}:${ARGUS_STATE_DIR}
    command: sleep infinity         # keep-alive; agent runs via exec
  # + a tailscale sidecar for the client's tailnet, etc.
```

`docker compose exec` additionally passes `--user $ARGUS_UID:$ARGUS_GID` so the
agent process is the host user regardless of the image default.

## Lifecycle

- **Up:** the first session create (or revival) for a dockerized profile runs
  `docker compose -p argus-<name> up -d`, after confirming the stack is not
  already running (`docker compose ps`). Idempotent and serialized per profile
  (see Concurrency). Also exposed as `argus profile up <name>`.
- **Per session:** the tmux pane runs the host init wrapper, whose agent step is
  `docker compose -p argus-<name> exec -w <cwd> -u UID:GID -e <ARGUS_*…> agent
  <inner-init>`.
- **Down:** never automatic. `argus profile down <name>` (or plain
  `docker compose down`) tears the stack down.

Lazy-up lives in the create/respawn path, so `EnsureSession` (session revival)
and `ChangeProfile` (switching *to* a dockerized profile) both trigger it.
Switching *away* leaves the old stack running (manual down).

## Execution model: the init script

The **host** init wrapper (written to a host temp dir, run by tmux — same as
today) keeps the banner before and the `tmux capture-pane` provider-session-ID
capture after. Its agent step is wrapped in `docker compose exec`. The **inner**
init script — export `PATH`, `source` the `post_create` hooks, run the agent —
runs *inside* the container.

So the existing init-script body is split at exactly one seam:

```
host wrapper (host tmux pane):
  banner
  docker compose -p argus-<name> exec -w <cwd> -u UID:GID -e <env> agent \
      bash <inner-init-path>
  tmux capture-pane … → argus internal session set-provider-id …

inner init (in container):
  export PATH="$HOME/.local/bin:$PATH"
  source <profile post_create>   # host path, visible via the home mount
  source <project post_create>
  <agent command>                # e.g. claude --foo
```

Because home and the state dir are mounted at identical paths, the `post_create`
hook files are readable in-container at their host paths, and the inner init
script can be written under a mounted location (the state dir) and `exec`'d
in-container by path — avoiding shell-quoting the whole script through
`docker compose exec`. For a **non-dockerized** profile the inner init runs
inline on the host exactly as today (no behavior change).

Provider-session-ID capture is unchanged: the agent's output reaches the host
pane through the exec PTY, so `tmux capture-pane` on the host sees it after the
`exec` returns, and the `argus internal session set-provider-id` call runs on the
host binary as before.

### Shell sessions

A `shell` provider under a dockerized profile runs its interactive
`$SHELL -l` (sourcing `post_create` first, if any) in the container via
`docker compose exec` — i.e. a containerized client shell. Consistent with the
"only the agent's TUI runs in the container" model.

## Working-directory constraint

Any session whose cwd is under the mounted home (or state dir) just works —
essentially all of them. A session whose cwd is outside both mounted roots cannot
be seen by the container; Argus **rejects** attaching such a session to a
dockerized profile (at create and at `ChangeProfile`) with a clear error rather
than silently running on the host.

## Components

### Backend

**`internal/node/docker/` (new package)** — a thin wrapper over the compose CLI,
behind an interface so the lifecycle/command logic is unit-testable with a fake:

- `Compose` interface: `Up(project, file string, env)`, `Down(project, file)`,
  `IsUp(project) (bool, error)`, and `ExecCommand(...) []string` /
  `ExecArgs(...)` to build the `docker compose exec` argv (project, file, cwd,
  user, env, service, inner command).
- Detection helper: `IsDockerProfile(stateDir, profileName) (composeFile string,
  ok bool)`.
- Env assembly: `ComposeEnv(home, stateDir string) []string` producing
  `ARGUS_HOST_HOME`, `ARGUS_STATE_DIR`, `ARGUS_UID`, `ARGUS_GID`.

**`internal/node/session/` changes**

- `initscript.go`: factor the existing body into an **inner** init (PATH +
  `source post_create` + agent + nothing host-specific) and a **host wrapper**
  (banner + agent-step + capture). For dockerized profiles the agent step is the
  `docker compose exec … bash <inner-path>` line and the inner script is written
  to a mounted path; otherwise the inner runs inline (today's behavior).
- `lifecycle.go`: `Create`, `respawnTmux`, and `ChangeProfile` consult
  `IsDockerProfile`. When true they (a) ensure the stack is up (lazy-up,
  serialized per profile), (b) enforce the cwd-under-mount constraint, and
  (c) build the docker-wrapped tmux command. `pre_create`, `on_create_worktree`,
  and `pre_destroy` continue to run on the host unchanged.
- A per-profile lock (alongside the existing per-session locks) serializes
  `up`/`down` so concurrent creates for the same profile don't race.

**Profile listing** — `ListProfiles` already enumerates profile dirs; extend the
returned shape (or add a sibling call) to flag which profiles are dockerized, for
the CLI/UI.

### CLI

`cmd/argus/cli/session_profile.go` (or a new `profile` top-level group):

- `argus profile up <name>` — bring the stack up.
- `argus profile down <name>` — tear the stack down.
- `argus profile status <name>` — show whether the stack is up.
- `argus profile ls` — list profiles, annotating which are dockerized.

### API / UI

The existing profile selector (`NewSessionDialog`, `SessionInfoDialog`) keeps
working as-is. Dockerized profiles get a small badge in the selector and session
info. No new UI flows for v1; stack up/down is CLI-driven in v1.

## Error handling

- Compose failures (`up`, `exec`, image build) surface as session-create errors
  with stderr attached, consistent with how `pre_create` hook failures abort
  create today.
- Selecting a dockerized profile when the Docker daemon is unreachable fails fast
  with a clear message.
- cwd outside the mounted roots → explicit validation error at create /
  `ChangeProfile`.
- `pre_destroy` remains best-effort; it runs on the host, so it is unaffected by
  stack state.

## Concurrency

- Per-profile lock serializes stack `up`/`down`, so simultaneous creates for the
  same profile bring the stack up exactly once.
- `docker compose up -d` is idempotent; `IsUp` short-circuits when the stack is
  already running.

## Testing

- Compose interactions sit behind the `Compose` interface; lifecycle and
  command-building logic is unit-tested with a fake (no daemon required).
- Golden-style tests for the constructed argv and inner/host init scripts: mounts,
  `-w`, `--user`, project name, env injection, and the host-wrapper/inner split
  for both dockerized and non-dockerized profiles.
- One opt-in integration test, guarded by an env flag and `docker` availability
  (mirroring the tmux-test-safety isolation pattern): bring up a trivial stack,
  `exec` a command end-to-end, tear it down.

## Out of scope (v1)

- Per-profile config for a non-default agent service name (convention `agent`
  only).
- Refcount/auto teardown of stacks (manual down only).
- UI controls for stack up/down (CLI only).
- Argus building or managing images, tailnet config, or credentials — all owned
  by the user's compose file.
- Dockerized profiles for sessions whose cwd is outside the mounted roots
  (rejected, not supported).
