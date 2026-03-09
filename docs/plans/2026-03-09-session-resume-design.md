# Session Resume Design [BXN-41]

## Problem

When a session's tmux process dies (tmux failure, accidental ctrl-c), Argus recreates it via `EnsureSession` but starts a fresh provider conversation. Users lose all conversation context.

The plumbing for resumption already exists:
- `ProviderSessionID` field in the DB
- `EnsureSession` passes it to `BuildCommand` with the provider's resume flag
- All three providers (Claude, Codex, Gemini) support `--resume` / `resume`

**The gap**: The `ProviderSessionID` is only set when explicitly passed during session creation. When a provider starts a new conversation, it generates its own session ID internally — but that ID is never captured back into the Argus database.

## Solution

Capture the provider session ID from terminal output when the provider process exits, and persist it via the Argus CLI so `EnsureSession` can resume the conversation on restart.

### How providers expose their session ID

All three providers print a resume hint on exit:

**Claude:**
```
claude --resume e9ed7eb1-5fa8-40ca-b718-bc747ea4e38e
```

**Codex:**
```
codex resume 019cce43-57d3-7842-9f1d-732711edbf25
```

**Gemini:**
```
Session ID:                 defacfa6-a9ae-477a-aff3-e5e89a581431
```

### Components

#### 1. Provider struct: `SessionIDPattern`

Add a regex field to the `Provider` struct so each provider defines how to extract its session ID from terminal output.

```go
type Provider struct {
    // ... existing fields ...
    SessionIDPattern string // regex with one capture group for the session ID
}
```

Patterns:
- Claude: `claude --resume ([0-9a-f-]+)`
- Codex: `codex resume ([0-9a-f-]+)`
- Gemini: `Session ID:\s+([0-9a-f-]+)`

#### 2. Init script: post-exit capture

Remove `exec` from the init script so the bash script continues after the provider exits. After exit:

1. Capture the tmux pane content (`tmux capture-pane -p -S -100`)
2. Match the provider-specific regex to extract the session ID
3. Call `argus internal session set-provider-id <argus-session-id> <provider-session-id>` to persist it

The init script generator (`GenerateInitScript`) needs additional parameters:
- The Argus session ID (already available)
- The provider's session ID regex pattern

```bash
# Start the agent (no exec — script continues after exit)
<agent_command>

# Capture provider session ID from terminal output
PANE_CONTENT=$(tmux capture-pane -p -S -100 2>/dev/null)
PROVIDER_ID=$(echo "$PANE_CONTENT" | grep -oE '<pattern>' | tail -1)

if [ -n "$PROVIDER_ID" ]; then
  argus internal session set-provider-id '<argus-session-id>' "$PROVIDER_ID"
fi
```

#### 3. CLI: `argus internal session set-provider-id`

New internal CLI command tree:

```
argus internal                              # Internal commands (not user-facing)
  session                                   # Internal session operations
    set-provider-id <session-id> <value>    # Persist provider session ID
```

The command:
1. Reads the discovery file to find the agent API
2. Sends a PATCH to `/api/sessions/<id>` with `{"provider_session_id": "<value>"}`

#### 4. API PATCH handler: accept `provider_session_id`

The existing PATCH handler at `/api/sessions/{id}` only accepts `name`. Extend it to also accept `provider_session_id` and call `manager.Update()` with the `db.SessionUpdate` struct (which already supports the field).

#### 5. `EnsureSession`: no changes needed

Already reads `ProviderSessionID` from the DB and passes it to `BuildCommand`. Once the ID is persisted by the init script, resumption works automatically.

### Data flow

```
Provider starts (new session)
  → Provider generates internal session ID
  → Provider runs, user works
  → Provider exits (ctrl-c, /quit, crash)
  → Provider prints session ID to terminal
  → Init script continues (no exec)
  → Init script captures tmux pane content
  → Init script extracts session ID via regex
  → Init script calls: argus internal session set-provider-id <sess-id> <provider-id>
  → CLI PATCHes API → DB updated with provider_session_id
  → tmux session ends

Later: user opens session again (or EnsureSession triggers)
  → EnsureSession reads ProviderSessionID from DB
  → BuildCommand adds --resume <provider-id>
  → Provider resumes previous conversation
```

### Edge cases

- **Regex doesn't match** (provider crashed without printing): No-op. Session starts fresh next time. This is the same behavior as today.
- **Argus agent is down when init script runs**: CLI call fails silently. Session starts fresh. No worse than current behavior.
- **Provider already has a stored session ID**: Overwritten with the latest. The most recent session ID is always the correct one to resume.
- **Shell sessions**: No provider command, no init script post-exit logic. Unaffected.
- **Session created with explicit `ResumeSessionID`**: Works as before during creation. On subsequent exits, the captured ID overwrites the original — which is correct since the provider may have started a new session.

### Files to modify

| File | Change |
|------|--------|
| `internal/agent/provider/provider.go` | Add `SessionIDPattern` to `Provider` struct |
| `internal/agent/provider/claude.go` | Add Claude's regex pattern |
| `internal/agent/provider/codex.go` | Add Codex's regex pattern |
| `internal/agent/provider/gemini.go` | Add Gemini's regex pattern |
| `internal/agent/session/initscript.go` | Remove `exec`, add post-exit capture logic, accept pattern param |
| `internal/agent/session/lifecycle.go` | Pass provider pattern to init script generator |
| `internal/agent/api/sessions.go` | Extend PATCH handler to accept `provider_session_id` |
| `cmd/argus/cli/cli.go` | Register `internal` command tree |
| `cmd/argus/cli/internal_cmd.go` | New: `argus internal` parent command |
| `cmd/argus/cli/internal_session_set_provider_id.go` | New: `set-provider-id` command |
