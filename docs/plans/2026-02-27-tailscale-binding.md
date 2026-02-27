# Tailscale Interface Binding Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a `[tailscale]` config option that auto-detects Tailscale IPs and binds the server/agent to them alongside loopback.

**Architecture:** New `TailscaleConfig` struct in config package. New `internal/tailscale` package with `DetectIPs()` using the Tailscale Go client library. Refactored `listenAddrs()` to accept a flat IP slice + port, with callers building the full IP list including Tailscale addresses.

**Tech Stack:** Go stdlib `net/netip`, `tailscale.com/client/local`, existing Viper config system.

---

### Task 1: Add TailscaleConfig to config package

**Files:**
- Modify: `internal/config/config.go:14-18` (Config struct), `config.go:54-60` (defaults)
- Modify: `internal/config/config_test.go:13-25` (clearArgusEnv)

**Step 1: Write the failing test**

Add to `internal/config/config_test.go` — update `clearArgusEnv` to include the new env var, then add a test:

In `clearArgusEnv`, add `"ARGUS_TAILSCALE_ENABLED"` to the env var list:

```go
func clearArgusEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ARGUS_SERVER_PORT", "ARGUS_SERVER_BIND_ADDRESS",
		"ARGUS_AGENT_PORT", "ARGUS_AGENT_BIND_ADDRESS",
		"ARGUS_DATABASE_PATH", "ARGUS_GIT_BRANCH_PREFIX",
		"ARGUS_TAILSCALE_ENABLED",
	} {
		if v, ok := os.LookupEnv(key); ok {
			t.Cleanup(func() { os.Setenv(key, v) })
		}
		os.Unsetenv(key)
	}
}
```

Then add these tests at the end of the file:

```go
func TestTailscaleDefaults(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = true, want false")
	}
}

func TestTailscaleFromFile(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[tailscale]
enabled = true
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = false, want true")
	}
}

func TestTailscaleEnvOverride(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ARGUS_TAILSCALE_ENABLED", "true")

	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = false, want true (env override)")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestTailscale' -v`
Expected: FAIL — `cfg.Tailscale` field does not exist

**Step 3: Write minimal implementation**

In `internal/config/config.go`, add the struct and wire it up:

```go
type TailscaleConfig struct {
	Enabled bool `mapstructure:"enabled"`
}
```

Add field to `Config`:

```go
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Agent     AgentConfig     `mapstructure:"agent"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Git       GitConfig       `mapstructure:"git"`
	Tailscale TailscaleConfig `mapstructure:"tailscale"`
}
```

Add default in `Load()` after the existing defaults (after line 60):

```go
v.SetDefault("tailscale.enabled", false)
```

No validation needed — `bool` with a default is always valid.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add tailscale config section with enabled toggle"
```

---

### Task 2: Add Tailscale dependency and create detect package

**Files:**
- Create: `internal/tailscale/detect.go`
- Modify: `go.mod`, `go.sum` (via `go get`)

**Step 1: Add the Tailscale client dependency**

Run: `go get tailscale.com/client/local`

This pulls in `tailscale.com` and its transitive deps. The binary size increase is acceptable since we only import the client.

**Step 2: Write the detection function**

Create `internal/tailscale/detect.go`:

```go
package tailscale

import (
	"context"
	"net/netip"

	"tailscale.com/client/local"
)

// DetectIPs queries the local Tailscale daemon for this node's Tailscale IPs.
// Returns both IPv4 (100.x.x.x) and IPv6 (fd7a:...) addresses when available.
// Returns nil and no error if Tailscale is not running or has no IPs.
func DetectIPs(ctx context.Context) ([]netip.Addr, error) {
	status, err := local.Status(ctx)
	if err != nil {
		return nil, err
	}
	if status.Self == nil {
		return nil, nil
	}
	return status.Self.TailscaleIPs, nil
}
```

**Step 3: Verify it compiles**

Run: `go build ./internal/tailscale/`
Expected: SUCCESS (no output)

**Step 4: Commit**

```bash
git add internal/tailscale/detect.go go.mod go.sum
git commit -m "feat: add tailscale IP detection via local client"
```

---

### Task 3: Refactor listenAddrs to accept IP slice

**Files:**
- Modify: `cmd/argus/main.go:148-160` (listenAddrs function)
- Modify: `cmd/argus/main_test.go` (all tests)

**Step 1: Write the failing tests with the new signature**

Replace the entire content of `cmd/argus/main_test.go`:

```go
package main

import (
	"testing"
)

func TestListenAddrs(t *testing.T) {
	tests := []struct {
		name string
		ips  []string
		port int
		want []string
	}{
		{
			name: "single loopback",
			ips:  []string{"127.0.0.1"},
			port: 3000,
			want: []string{"127.0.0.1:3000"},
		},
		{
			name: "IPv6 loopback",
			ips:  []string{"::1"},
			port: 3000,
			want: []string{"[::1]:3000"},
		},
		{
			name: "multiple IPs",
			ips:  []string{"192.168.1.10", "127.0.0.1"},
			port: 3000,
			want: []string{"192.168.1.10:3000", "127.0.0.1:3000"},
		},
		{
			name: "with tailscale IPs",
			ips:  []string{"127.0.0.1", "100.64.0.1", "fd7a:115c:a1e0::1"},
			port: 3000,
			want: []string{"127.0.0.1:3000", "100.64.0.1:3000", "[fd7a:115c:a1e0::1]:3000"},
		},
		{
			name: "deduplicates",
			ips:  []string{"127.0.0.1", "127.0.0.1"},
			port: 3000,
			want: []string{"127.0.0.1:3000"},
		},
		{
			name: "unspecified IPv4",
			ips:  []string{"0.0.0.0"},
			port: 3000,
			want: []string{"0.0.0.0:3000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := listenAddrs(tt.ips, tt.port)
			if len(got) != len(tt.want) {
				t.Fatalf("listenAddrs(%v, %d) = %v, want %v", tt.ips, tt.port, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("listenAddrs(%v, %d)[%d] = %q, want %q", tt.ips, tt.port, i, got[i], tt.want[i])
				}
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/argus/ -run TestListenAddrs -v`
Expected: FAIL — signature mismatch

**Step 3: Refactor listenAddrs**

Replace `listenAddrs` in `cmd/argus/main.go` (lines 148-160):

```go
// listenAddrs formats each IP with the given port into host:port addresses,
// deduplicating any repeated entries.
func listenAddrs(ips []string, port int) []string {
	portStr := strconv.Itoa(port)
	seen := make(map[string]bool)
	var addrs []string
	for _, ip := range ips {
		addr := net.JoinHostPort(ip, portStr)
		if !seen[addr] {
			seen[addr] = true
			addrs = append(addrs, addr)
		}
	}
	return addrs
}
```

**Step 4: Update all callers**

The callers need to build the IP list themselves. For now, replicate the existing behavior without Tailscale (that comes in Task 4).

Add a helper function `bindIPs` above `listenAddrs`:

```go
// bindIPs builds the list of IPs to bind to. It always includes the primary
// bind address. If the primary is a specific non-loopback IP, it appends
// 127.0.0.1 so the CLI can always reach via loopback.
func bindIPs(bindAddr string, extra ...string) []string {
	ips := []string{bindAddr}
	if ip := net.ParseIP(bindAddr); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
		ips = append(ips, "127.0.0.1")
	}
	ips = append(ips, extra...)
	return ips
}
```

Update the three call sites:

In `newServerCmd` (line 70):
```go
return serve(listenAddrs(bindIPs(cfg.Server.BindAddress), cfg.Server.Port), mux, "argus server", nil)
```

In `newAgentCmd` (line 94):
```go
return serve(listenAddrs(bindIPs(cfg.Agent.BindAddress), cfg.Agent.Port), mux, "argus agent", func(a string) {
```

In `runCombined` (line 143):
```go
return serve(listenAddrs(bindIPs(cfg.Server.BindAddress), cfg.Server.Port), mux, "argus", func(a string) {
```

**Step 5: Run all tests to verify they pass**

Run: `go test ./cmd/argus/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add cmd/argus/main.go cmd/argus/main_test.go
git commit -m "refactor: listenAddrs accepts IP slice, add bindIPs helper"
```

---

### Task 4: Wire Tailscale detection into all three modes

**Files:**
- Modify: `cmd/argus/main.go` (runCombined, newServerCmd RunE, newAgentCmd RunE)

**Step 1: Add the Tailscale detection wiring**

Add the import at the top of `cmd/argus/main.go`:

```go
import (
	// ... existing imports ...
	ts "github.com/bxnlabs/argus/internal/tailscale"
)
```

Add a helper function that detects Tailscale IPs and returns them as strings:

```go
// tailscaleIPs returns Tailscale IPs as strings if Tailscale is enabled in config.
// Logs warnings and returns nil on failure — never fatal.
func tailscaleIPs() []string {
	if !cfg.Tailscale.Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addrs, err := ts.DetectIPs(ctx)
	if err != nil {
		log.Printf("warning: tailscale enabled but detection failed: %v", err)
		return nil
	}
	if len(addrs) == 0 {
		log.Printf("warning: tailscale enabled but no IPs found")
		return nil
	}
	strs := make([]string, len(addrs))
	for i, a := range addrs {
		strs[i] = a.String()
	}
	return strs
}
```

Update the three call sites to pass Tailscale IPs via `bindIPs`:

In `newServerCmd` RunE:
```go
return serve(listenAddrs(bindIPs(cfg.Server.BindAddress, tailscaleIPs()...), cfg.Server.Port), mux, "argus server", nil)
```

In `newAgentCmd` RunE:
```go
return serve(listenAddrs(bindIPs(cfg.Agent.BindAddress, tailscaleIPs()...), cfg.Agent.Port), mux, "argus agent", func(a string) {
```

In `runCombined`:
```go
return serve(listenAddrs(bindIPs(cfg.Server.BindAddress, tailscaleIPs()...), cfg.Server.Port), mux, "argus", func(a string) {
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/argus/`
Expected: SUCCESS

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 4: Commit**

```bash
git add cmd/argus/main.go
git commit -m "feat: bind to tailscale IPs when tailscale.enabled is true"
```

---

### Task 5: Manual verification

**Step 1: Verify with Tailscale disabled (default)**

Run: `go run ./cmd/argus/ &`
Expected log: `argus listening on 127.0.0.1:3000` (no tailscale addresses)

Stop: `kill %1`

**Step 2: Verify with Tailscale enabled**

Set env: `ARGUS_TAILSCALE_ENABLED=true go run ./cmd/argus/ &`

If Tailscale is running:
Expected log: `argus listening on 127.0.0.1:3000`, `argus listening on 100.x.x.x:3000`, `argus listening on [fd7a:...]:3000`

If Tailscale is NOT running:
Expected log: `warning: tailscale enabled but detection failed: ...`, then `argus listening on 127.0.0.1:3000` (graceful fallback)

Stop: `kill %1`
