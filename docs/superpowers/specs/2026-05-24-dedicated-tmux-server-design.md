# Dedicated, Isolated tmux Server for Argus Sessions (BXN-99)

## Overview

Today every Argus tmux interaction runs against the **default** tmux server —
the same server hosting the user's personal tmux. Argus's options, hooks, and
status-bar styling leak into that shared server, the user's `~/.tmux.conf` and
key-bindings bleed into Argus sessions, and killing or restarting either side
risks the other.

Route all Argus tmux interactions through a **dedicated, isolated tmux server**
with its own socket and config file under `$ARGUS_HOME/tmux/`. A single
command-builder threads the socket through every `tmux` invocation — node and
CLI — so Argus never reads or mutates the user's personal tmux server, and vice
versa.

## Goals

- Every `tmux` call Argus makes targets a dedicated server socket, never the
  default server.
- The socket (and a seeded, user-editable `tmux.conf`) live under
  `$ARGUS_HOME/tmux/`, so the dev stack (`ARGUS_HOME` under `.dev`) is
  automatically isolated from a production instance on the same machine.
- A single command-builder is the one place the socket is threaded, shared by
  the node and the CLI attach path.
- Server-level base config (truecolor, default terminal, mouse, static
  status-bar styling) lives in the config file rather than being re-applied
  per session.

## Non-Goals

- Migrating in-flight sessions off the default server (see Migration).
- Sourcing or honoring the user's `~/.tmux.conf` inside Argus sessions — the
  isolation is the point.
- Auto-migrating an existing `tmux.conf` to new shipped defaults — the seeded
  file is owned by the user once written and is never overwritten.
- Any change to the in-session `tmux capture-pane` in `initscript.go`; commands
  run *inside* a session already target the dedicated server via `$TMUX`.

## Architecture

The chosen approach is a **single shared command-builder** in `internal/shared`,
used by both the node (`internal/node/session/tmux.go`,
`internal/node/terminal/handler.go`) and the CLI
(`cmd/argus/cli/session_attach.go`). This is one source of truth for the socket
path and reuses the existing `shared.StateDir()` / `ARGUS_HOME` conventions.

Rejected alternatives:

- **Per-package threading** — each package prepends `-S` itself. Duplicates the
  socket-path logic and risks drift, the exact problem BXN-99 wants to avoid.
- **A `TmuxServer` client struct** with methods — a larger refactor of
  `tmux.go`'s free functions for no real benefit at this call-site count.

## Socket & Config Location

New helpers in `internal/shared` (alongside `StateDir()` in `paths.go`, or a new
`shared/tmux.go`):

```go
// TmuxSocketPath returns <StateDir>/tmux/server.
func TmuxSocketPath() (string, error)

// TmuxConfigPath returns <StateDir>/tmux/tmux.conf.
func TmuxConfigPath() (string, error)

// TmuxCommand / TmuxCommandContext build an *exec.Cmd that targets the
// dedicated socket: exec.Command("tmux", "-S", <sock>, args...).
func TmuxCommand(args ...string) (*exec.Cmd, error)
func TmuxCommandContext(ctx context.Context, args ...string) (*exec.Cmd, error)
```

- Both resolve under `StateDir()`, so they honor `ARGUS_HOME` and default to
  `~/.argus/tmux/...`.
- `TmuxCommand*` is the single builder threaded through **every** call site. It
  threads only `-S <sock>`; the `-f <conf>` flag is added by the caller that
  starts the server (see Bootstrap).

## Server Bootstrap & Base Config

The dedicated server is started lazily by tmux on the first `new-session`. The
only call that can start the server is `NewSession` in `tmux.go`, so bootstrap
lives there:

1. `os.MkdirAll(<StateDir>/tmux, 0700)` — tmux creates the socket file but not
   its parent directory.
2. Seed `tmux.conf` **only when it is missing**, using
   `os.OpenFile(path, O_CREATE|O_EXCL|O_WRONLY, 0644)` so an existing file —
   including one the user has edited — is never overwritten, and concurrent
   first-creates resolve cleanly (one wins with EEXIST on the other). Only the
   node ever writes the config; the CLI needs the socket only.
3. Invoke `tmux -S <sock> -f <conf> new-session ...`. The first one starts the
   server and reads the config; `-f` is correctly ignored by later calls against
   the already-running server.
4. If seeding fails, log and pass `-f <conf>` only when the file exists;
   otherwise omit `-f` (degrade rather than block session creation).

### Base `tmux.conf`

Holds the **static** server-level config that today either leaks from, or
depends on, the user's `~/.tmux.conf`:

```tmux
# Argus tmux defaults — seeded once. Edit to customize; Argus won't overwrite.
set -g default-terminal "tmux-256color"
set -ga terminal-overrides ",*:Tc"   # truecolor
set -g mouse on

# Static status-bar styling
set -g status-style "bg=#1e1e2e,fg=#cdd6f4"
set -g status-left "#[fg=#cba6f7,bold] Argus #[fg=#6c7086]| "
set -g status-left-length 20
set -g status-right-length 110
set -g status-position bottom
```

## Per-Session Styling

`ConfigureSession` is slimmed to set only the **dynamic** `status-right` (the
per-session ID / dir / branch built by `buildStatusRight`). The static options
above move into `tmux.conf`. This is the cleanup BXN-99 calls for: status-bar
styling currently applied per session via repeated `set-option` calls becomes
server-level config, leaving only the genuinely per-session value applied at
runtime.

`buildStatusRight` and the `escapeTmuxLiteral` escaping are unchanged.

## Call-Site Changes

| Location | Change |
|----------|--------|
| `internal/node/session/tmux.go` (~10 sites) | Replace every `exec.Command("tmux", ...)` / `exec.CommandContext(ctx, "tmux", ...)` with `shared.TmuxCommand(...)` / `shared.TmuxCommandContext(ctx, ...)`. `NewSession` additionally writes the config and passes `-f <conf>`. `ConfigureSession` sets only `status-right`. |
| `internal/node/terminal/handler.go:326` (web attach) | `shared.TmuxCommand("attach-session", "-t", tmuxName)`. |
| `cmd/argus/cli/session_attach.go:59` (CLI attach) | Build via `shared.TmuxCommand("attach-session", "-t", tmuxName)`, then set Stdin/Stdout/Stderr. CLI never starts the server, so `-S` alone suffices. |
| `internal/node/session/initscript.go` | **No change.** In-session `tmux capture-pane` targets the dedicated server via `$TMUX`. |

## Migration

**Auto-revive, no migration code.** After the switch, `HasSession` queries the
dedicated socket, so any session Argus previously created on the default server
reads as dead. `EnsureSession` then recreates it from its DB record on the
dedicated server on next access (resuming the provider conversation where the
provider supports it). The old default-server processes keep running until
manually killed.

- No migration code is added.
- The PR/release notes document that lingering Argus sessions on the default
  server can be killed manually (`tmux kill-server` against the default socket,
  or `tmux kill-session`).

## Testing Strategy

- **Unit** (`internal/shared`): `TmuxSocketPath` / `TmuxConfigPath` resolve under
  `StateDir()` and honor `ARGUS_HOME` (and the default `~/.argus`);
  `TmuxCommand` emits an arg vector beginning `tmux -S <sock> ...`.
- **Unit** (session package): the generated `tmux.conf` contains the expected
  directives (`default-terminal`, truecolor override, `mouse on`, status
  styling); `ConfigureSession` issues only the `status-right` option.
- **Integration** (`internal/node/session/tmux_test.go`, gated on `hasTmux()`):
  set `ARGUS_HOME` to a temp dir, create a session via `NewSession`, assert it
  appears on the dedicated socket (`ListSessions`) and **not** on the default
  server; tear down with `tmux -S <sock> kill-server`. Update the existing
  capture-pane wrap test's raw `tmux new-session` helper to target the socket.

## Risks & Tradeoffs

- **Socket path length.** Unix domain sockets cap path length (~104 chars). A
  deep `ARGUS_HOME` could exceed it; the default `~/.argus/tmux/server` is well
  within range.
- **Config changes need a fresh server.** A long-running dedicated server keeps
  its config until restarted (`kill-server`); an edited `tmux.conf` is read only
  on the next cold start.
- **Shipped defaults don't auto-propagate.** Because the seed never overwrites
  an existing file, a future release with updated defaults won't change a
  user's `tmux.conf`; adopting new defaults means editing the file or deleting
  it to re-seed.
- **Loss of personal tmux config inside Argus sessions.** The user's prefix key
  and bindings no longer apply inside Argus sessions — the intended isolation;
  the base config restores the essentials.

## File Changes

### New / Modified Files

| File | Change |
|------|--------|
| `internal/shared/tmux.go` (new) | `TmuxSocketPath`, `TmuxConfigPath`, `TmuxCommand`, `TmuxCommandContext`, base-config template + atomic write helper. |
| `internal/shared/tmux_test.go` (new) | Path resolution and command-builder unit tests. |
| `internal/node/session/tmux.go` | Route all calls through the shared builder; bootstrap + `-f` in `NewSession`; slim `ConfigureSession` to `status-right`. |
| `internal/node/session/tmux_test.go` | Socket-isolation integration assertions; update raw-tmux test helper. |
| `internal/node/terminal/handler.go` | Web attach via shared builder. |
| `cmd/argus/cli/session_attach.go` | CLI attach via shared builder. |
