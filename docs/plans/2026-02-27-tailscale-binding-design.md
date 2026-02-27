# Tailscale Interface Binding

## Problem

Argus binds to loopback (`127.0.0.1`) by default. To access the server or agent from other machines on a Tailscale network (tailnet), users need a way to additionally bind to their Tailscale interface IPs.

## Decision

Add a `[tailscale]` config section with an `enabled` boolean. When enabled, auto-detect the node's Tailscale IPv4 and IPv6 addresses using the Tailscale Go client library and bind to them alongside the primary and loopback listeners.

## Config

```toml
[tailscale]
enabled = false
```

- Default: `false`
- Env var: `ARGUS_TAILSCALE_ENABLED`
- When enabled, Tailscale IPs are auto-detected at startup via the local Tailscale daemon

```go
type TailscaleConfig struct {
    Enabled bool `mapstructure:"enabled"`
}
```

Added to the existing `Config` struct as `Tailscale TailscaleConfig`.

## Tailscale IP Detection

New package: `internal/tailscale/detect.go`

```go
func DetectIPs(ctx context.Context) ([]netip.Addr, error)
```

Uses `tailscale.com/client/local` to query the local Tailscale daemon:
1. `local.Status(ctx)` returns `*ipnstate.Status`
2. Extract `status.Self.TailscaleIPs` for both IPv4 (`100.x.x.x`) and IPv6 (`fd7a:...`) addresses
3. Return them, or an error if the daemon is unreachable

Dependency: `tailscale.com` (specifically `tailscale.com/client/local`)

## Listener Integration

Refactor `listenAddrs()` to accept a flat `[]string` of IPs plus port:

```go
func listenAddrs(ips []string, port int) []string
```

The caller builds the full IP list:
1. Primary bind address (from config)
2. Loopback guarantee (`127.0.0.1` if primary is non-loopback, non-unspecified)
3. Tailscale IPs (if `tailscale.enabled = true` and detection succeeds)

`listenAddrs()` formats each as `ip:port` and deduplicates. Applied to all three modes: combined, server-only, agent-only.

## Error Handling

- **Tailscale not running:** Log warning, proceed without Tailscale binding
- **No IPs detected:** Log warning, proceed without Tailscale binding
- **Bind fails on Tailscale IP:** Fatal (same as any listener failure — indicates misconfiguration)
- **Discovery file:** Always writes loopback address, unchanged

## Files Changed

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `TailscaleConfig` struct, defaults, env var binding |
| `internal/config/config_test.go` | Test Tailscale config loading |
| `internal/tailscale/detect.go` | New: `DetectIPs()` via Tailscale Go client |
| `cmd/argus/main.go` | Refactor `listenAddrs()` signature; add Tailscale detection to all modes |
| `cmd/argus/main_test.go` | Update `listenAddrs()` tests |
| `go.mod` | Add `tailscale.com` (for `tailscale.com/client/local`) |
