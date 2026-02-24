# CLI & Terminal Co-existence Design

## Problem

Argus needs to co-exist with terminal-centric development workflows. Developers should be able to view and connect to Argus tmux sessions using the `argus` CLI, step away from their workstations, pick up via the Argus desktop or mobile client, and continue on their workstation when they return.

## Approach

Thin CLI layer (Approach A). The `argus` binary gains a `session` subcommand group. CLI commands are HTTP clients to the running agent API. The `attach` command bypasses the API for terminal I/O and `exec`s directly into tmux.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        tmux server                           │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │ session: foo  │  │ session: bar  │  │ session: baz  │       │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │
└─────────┼─────────────────┼─────────────────┼────────────────┘
          │                 │                 │
     ┌────┴────┐       ┌───┴────┐       ┌───┴────┐
     │ attach  │       │  PTY   │       │  PTY   │
     │ (exec)  │       │ bridge │       │ bridge │
     └────┬────┘       └───┬────┘       └───┬────┘
          │                │                │
   ┌──────┴──────┐   ┌────┴────────────────┴────┐
   │  CLI user   │   │      Argus Agent          │
   │  terminal   │   │  (HTTP API + WebSocket)   │
   └─────────────┘   └────┬────────────────┬────┘
                          │                │
                     ┌────┴────┐     ┌────┴────┐
                     │  Web /  │     │ Mobile  │
                     │ Desktop │     │  App    │
                     └─────────┘     └─────────┘
```

- CLI attaches directly to tmux (no agent in the I/O path)
- CLI uses the agent API only for session discovery and CRUD
- Web/desktop/mobile go through the agent's WebSocket-PTY bridge
- All clients can connect simultaneously (tmux native multiplexing)

## Agent Discovery

The agent writes `~/.argus/agent.json` on startup:

```json
{"pid": 12345, "address": "127.0.0.1:3000"}
```

The file is removed on graceful shutdown. The agent always serves at the `/agent` prefix in both combined and standalone modes.

The CLI reads this file to locate the agent. API calls go to `http://{address}/agent/api/...`.

### Graceful Degradation

| Scenario | Behavior |
|----------|----------|
| No discovery file | Error: "Argus agent is not running." with start instructions |
| Stale file (PID dead) | Error: "Agent not running (stale state detected, cleaning up)." Removes stale file |
| PID alive but unreachable | Error: "Cannot reach Argus agent at {address}." |
| `attach` when tmux session dead | Handled by existing `EnsureSession` logic (revives dead sessions) |

All errors go to stderr with exit code 1.

## CLI Commands

### `argus session list`

Lists all sessions in a table.

```
NAME            PROVIDER   STATUS    UPDATED
my-session      claude     running   2 min ago
api-work        codex      idle      1 hour ago
debug-auth      shell      stopped   3 days ago
```

API: `GET /agent/api/sessions`

### `argus session create`

Creates a new session.

```
$ argus session create --name my-session
Created session "my-session" (claude)
  ID:  sess_abc123
  Dir: /home/user/project
```

Flags:
- `--name` (required) display name
- `--provider` (default: `claude`) agent type
- `--model` model override
- `--dir` (default: `.`) working directory
- `--auto-approve` enable auto-approve
- `--prompt` initial prompt to send after creation

API: `POST /agent/api/sessions`

### `argus session attach`

Attaches to a session's tmux. Resolves the session via API, then `exec`s into tmux.

```
$ argus session attach my-session
# exec: tmux attach-session -t claude-sess_abc123

$ argus session attach --cc my-session
# exec: tmux -CC attach-session -t claude-sess_abc123
```

The `--cc` flag enables tmux control mode for terminal emulators that support it (e.g., iTerm2). No auto-detection; explicit opt-in only.

API: `GET /agent/api/sessions` (to resolve name/ID to tmux name)

### `argus session delete`

```
$ argus session delete my-session
Deleted session "my-session"
```

API: `DELETE /agent/api/sessions/{id}`

### `argus session rename`

```
$ argus session rename my-session new-name
Renamed session "my-session" → "new-name"
```

API: `PATCH /agent/api/sessions/{id}`

### Session Resolution

`attach`, `delete`, and `rename` accept a session name or session ID. Resolution order:

1. Exact name match
2. ID prefix match

If ambiguous (multiple matches), error with matching sessions listed.

## Package Structure

```
cmd/argus/
  main.go              # Add "session" case to dispatch
  cli/
    cli.go             # Entry point: parse subcommand, dispatch
    client.go          # HTTP client: discovery file, API calls, error handling
    session_list.go    # list command
    session_create.go  # create command
    session_attach.go  # attach command (resolve + exec tmux)
    session_delete.go  # delete command
    session_rename.go  # rename command
```

### Changes to Existing Code

1. `cmd/argus/main.go` -- add `"session"` case to call `cli.Run(os.Args[2:])`
2. `internal/agent/setup.go` -- write `~/.argus/agent.json` on startup, remove on cleanup
3. Agent-only mode (`runAgent`) -- already mounts at `/agent/` prefix, no changes needed

### CLI-Side Types

Lightweight response structs in the CLI package that mirror the JSON responses from the API. No imports from `internal/`.

## tmux Control Mode

The `--cc` flag on `argus session attach` passes `-CC` to tmux:

```
# Regular:   exec tmux attach-session -t <tmux_name>
# Control:   exec tmux -CC attach-session -t <tmux_name>
```

No auto-detection. Explicit flag only.

## Simultaneous Access

tmux natively supports multiple attached clients. The CLI user and web/mobile clients can be connected to the same session simultaneously. Input from any client is sent to the session. The CLI uses `tmux attach-session` (without `-d`) to avoid detaching other clients.
