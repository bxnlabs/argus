# Tailscale tsnet Integration [BXN-49]

## Problem

Argus currently detects the host's Tailscale IPs via the local daemon and binds to them. This fails when the host is connected to a different tailnet than the one Argus should be reachable on. We need Argus to join a tailnet as its own independent node.

## Decision

Replace the host-IP-detection approach with `tsnet` from the Tailscale Go SDK. When Tailscale is enabled, Argus creates its own Tailscale node that joins the tailnet independently of the host. The server listens on loopback (for CLI access) and the tsnet interface only — the regular bind address is not used.

## Config

```toml
[tailscale]
enabled = false
hostname = ""     # defaults to "argus-<os.Hostname()>"
auth_key = ""     # one-time key; prefer ARGUS_TAILSCALE_AUTH_KEY env var
```

- `enabled` — gates all Tailscale behavior. Default: `false`. Env: `ARGUS_TAILSCALE_ENABLED`.
- `hostname` — device name in the tailnet. Default: `argus-<os.Hostname()>`. Env: `ARGUS_TAILSCALE_HOSTNAME`.
- `auth_key` — Tailscale auth key for initial join. Env: `ARGUS_TAILSCALE_AUTH_KEY`. Only needed on first run or re-authentication; `tsnet` persists node state across restarts. Note: if stored in `config.toml`, the key is plaintext on disk — prefer the env var for production use. `tsnet` also reads `TS_AUTHKEY` from the environment if `AuthKey` is empty; Argus will not suppress this behavior.

All new config keys (`tailscale.hostname`, `tailscale.auth_key`) must be registered via `v.SetDefault(...)` for Viper env var mapping to work correctly.

Removed fields:
- `tailnet` — no longer needed; the auth key determines which tailnet is joined.

```go
type TailscaleConfig struct {
    Enabled  bool   `mapstructure:"enabled"`
    Hostname string `mapstructure:"hostname"`
    AuthKey  string `mapstructure:"auth_key"`
}
```

## tsnet Server Wrapper

New file: `internal/tailscale/server.go` (replaces `detect.go`)

```go
type Server struct {
    ts      *tsnet.Server
    started bool
}

func New(hostname, authKey, stateDir string) *Server
func (s *Server) Up(ctx context.Context) error
func (s *Server) Listen(network, addr string) (net.Listener, error)
func (s *Server) Close() error
```

- `tsnet.Server.Dir` = `<userHomeDir>/.argus/tailscale/<hostname>/` — per-hostname isolation prevents state conflicts if multiple Argus instances run with different hostnames. Created via `os.MkdirAll(..., 0o700)` before startup.
- `tsnet.Server.Hostname` = configured hostname or `argus-<os.Hostname()>`
- `tsnet.Server.AuthKey` = from config (only consumed on first join)

### Startup Sequence

`Listen()` alone does not prove the node is authenticated or reachable — it starts the backend but returns immediately. The wrapper must call `Up(ctx)` with a bounded timeout (30 seconds) to wait for the node to reach `ipn.Running` state. The startup flow is:

1. `New(...)` — creates the wrapper and `tsnet.Server`, sets `Dir`/`Hostname`/`AuthKey`
2. `Up(ctx)` — calls `tsnet.Server.Up(ctx)`, which starts the backend and blocks until `Running`. If the node needs login and no auth key is available, this fails with a clear error. Returns error on timeout or auth failure.
3. `Listen(...)` — binds the tsnet listener for serving. Only called after `Up()` succeeds.

### Close Safety

`Close()` tracks whether `Up()` was successfully called. If the server was never started, `Close()` is a no-op. This prevents the `tsnet` invariant violation where `Close` is called before `Start`.

## Listener Integration

### Tailscale disabled (default)

`serve()` is refactored to accept `[]net.Listener` plus a `discoveryAddr string`. In the disabled path, a small helper builds standard `net.Listen` listeners from the configured bind addresses (replacing `bindIPs`/`listenAddrs`), keeping listener creation centralized rather than duplicated across call sites.

### Tailscale enabled

The caller builds two listeners and passes them to `serve()`:

1. **Loopback listener** — `net.Listen("tcp", "127.0.0.1:<port>")` for CLI and local browser access
2. **tsnet listener** — `tsServer.Listen("tcp", ":<port>")` for tailnet access

`serve()` accepts `[]net.Listener` instead of `[]string` addresses. Callers create listeners; `serve()` just serves on them. All three call sites (`runCombined`, `newServerCmd` RunE, `newAgentCmd` RunE) must build listeners before calling `serve()`.

The discovery address is passed as a separate `discoveryAddr string` parameter to `serve()`, not derived from listener slice position. This avoids a brittle ordering contract.

Applied to all three modes: combined, server-only, agent-only.

## Shutdown

During graceful shutdown (SIGINT/SIGTERM):

1. HTTP server shutdown (existing 5-second timeout) — this closes listeners and drains in-flight HTTP requests
2. `tsServer.Close()` — tears down the tsnet node cleanly (blocks until backend shuts down)

Note: `http.Server.Shutdown` does not drain hijacked connections (WebSockets). Graceful WebSocket/terminal session shutdown is a pre-existing gap, not introduced by this change, and is out of scope.

Node state persists in `~/.argus/tailscale/<hostname>/` so the node reconnects without re-auth on restart. Deleting the state directory discards node identity and forces re-registration.

## Config Validation

- No config-level validation for `auth_key` — whether it's required depends on runtime state (first run vs. subsequent). Validation is deferred to `tsnet` `Up()`, which produces a clear error if auth fails.
- `hostname` needs no validation — empty string triggers the `argus-<hostname>` default at runtime.
- Remove the existing `tailscale.tailnet` validation rule.

## Error Handling

| Scenario | Behavior |
|----------|----------|
| Auth key missing on first run (no persisted state) | `Up()` fails with timeout → fatal error with clear message |
| Auth key missing on subsequent runs | Fine — `tsnet` reuses persisted state in `Dir` |
| `Up()` times out or auth fails | Fatal error (Tailscale was explicitly enabled) |
| tsnet `Listen()` fails | Fatal error |
| Loopback bind fails | Fatal error (same as today) |

## Removed Code

| What | Why |
|------|-----|
| `internal/tailscale/detect.go` | Replaced by `server.go` |
| `internal/tailscale/detect_test.go` | Tests for removed code |
| `tailscale.tailnet` config field + validation | Auth key determines tailnet |
| `tailscaleIPs()` in `main.go` | Replaced by tsnet listener creation |
| `bindIPs()` helper in `main.go` | No longer needed — listeners built directly |
| `listenAddrs()` helper in `main.go` | No longer needed — callers create `net.Listener` directly |
| `tailscale.com/client/local` usage | Replaced by `tailscale.com/tsnet` |

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Update `TailscaleConfig` — remove `Tailnet`, add `Hostname`, `AuthKey`; add `SetDefault` for new keys; update validation |
| `internal/config/config_test.go` | Update tests for new fields, remove tailnet validation tests |
| `internal/tailscale/detect.go` | Delete |
| `internal/tailscale/detect_test.go` | Delete |
| `internal/tailscale/server.go` | New: `tsnet.Server` wrapper with `Up`/`Listen`/`Close` and started-state tracking |
| `cmd/argus/main.go` | Refactor `serve()` to accept `[]net.Listener` + `discoveryAddr`; add listener-building helper for disabled path; replace `tailscaleIPs()`/`bindIPs()` with tsnet startup + listener creation; add tsnet shutdown |
| `cmd/argus/main_test.go` | Tests for listener-building helper, discovery address passing, disabled-path behavior |
| `go.mod` / `go.sum` | `tsnet` import (already in `tailscale.com` module) |
