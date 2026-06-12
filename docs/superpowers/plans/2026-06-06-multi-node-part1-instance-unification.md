# Multi-node Part 1 — Instance Unification & `/api` Namespace — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse Argus's `server`/`node`/`combined` run modes into one unified instance on a single port with a single `/api` namespace, so any instance is both a standalone deployment and a discoverable peer — without changing single-node UI behavior.

**Architecture:** Remove the `argus server` / `argus node` subcommands and the mode-dependent port/hostname plumbing; the root command runs the one instance. Loopback binds one configurable `port` (default 3000); Tailscale binds a fixed `:80`. The node API moves from `/node/api/*` to `/api/node/*` by making the node router prefix-agnostic (it registers `/sessions`, `/ws/...`) and mounting it under `/api/node` via `StripPrefix`. A permissive CORS middleware wraps the node API for future cross-origin access. The CLI and web client are updated to the new paths.

**Tech Stack:** Go (cobra, viper, net/http `ServeMux`, gorilla/websocket, tsnet), React/TypeScript (Vite), pnpm.

This is **Plan 1 of 3** for BXN-106. It ships green on its own (single-node app works unchanged, on new paths). Plan 2 adds the node registry + Tailscale discovery + summary endpoint; Plan 3 adds the multi-node UI. Spec: `docs/superpowers/specs/2026-06-05-multi-node-ui-design.md`.

---

## File Structure

**Modified:**
- `internal/config/config.go` — collapse `ServerConfig`/`NodeConfig` into top-level `Port` + `BindAddress`.
- `internal/config/config_test.go` — rewrite assertions for the single port.
- `cmd/argus/main.go` — remove `newServerCmd`/`newNodeCmd`; drop `mode` from `makeListeners`/`deriveHostname`; fixed tailnet `:80`; mount `/api/node`.
- `internal/node/api/router.go` — drop the `/api` segment from every route pattern; wrap the mux in CORS.
- `internal/node/api/cors.go` *(create)* — permissive CORS middleware.
- `internal/node/api/cors_test.go` *(create)* — CORS middleware test.
- `cmd/argus/cli/client.go` + `cmd/argus/cli/*.go` — node base URL `/node` → `/api/node`; drop `/api` from call paths.
- `web/src/api/client.ts` + 14 other `web/src` files — `/node/api/` → `/api/node/`, `/node/ws/` → `/api/node/ws/`.
- `web/vite.config.ts` — proxy keys + `ARGUS_SERVER_PORT` → `ARGUS_PORT`.
- `Makefile`, `argus.dev.toml.example` — `ARGUS_SERVER_PORT` → `ARGUS_PORT`.

---

## Task 1: Collapse config to a single `port` + `bind_address`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Rewrite the config tests for a single port**

Replace the body of `internal/config/config_test.go`'s `TestDefaults`, `TestLoadFromFile`, `TestEnvVarOverrides`, and `TestAutoDiscoveryMissingFileUsesDefaults` so they reference `cfg.Port` / `cfg.BindAddress` instead of `cfg.Server.*` / `cfg.Node.*`. Concretely:

```go
func TestDefaults(t *testing.T) {
	t.Setenv("ARGUS_HOME", t.TempDir())
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.BindAddress != "127.0.0.1" {
		t.Errorf("BindAddress = %q, want 127.0.0.1", cfg.BindAddress)
	}
}
```

For `TestLoadFromFile`, change the TOML fixture to a top-level `port = 4000` / `bind_address = "0.0.0.0"` (delete the `[server]`/`[node]` tables) and assert `cfg.Port == 4000`, `cfg.BindAddress == "0.0.0.0"`. For `TestEnvVarOverrides`, set `t.Setenv("ARGUS_PORT", "9999")` and assert `cfg.Port == 9999`. For `TestAutoDiscoveryMissingFileUsesDefaults`, assert `cfg.Port == 3000` and `cfg.BindAddress == "127.0.0.1"`. Leave the Tailscale/notifications/explicit-missing-file tests unchanged.

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./internal/config/ -run 'TestDefaults|TestLoadFromFile|TestEnvVarOverrides|TestAutoDiscovery' -v`
Expected: FAIL — build error, `cfg.Port`/`cfg.BindAddress` undefined.

- [ ] **Step 3: Collapse the config struct + defaults + validation**

In `internal/config/config.go`, replace the `ServerConfig`/`NodeConfig` fields and types with single top-level fields:

```go
type Config struct {
	Port          int                 `mapstructure:"port"`
	BindAddress   string              `mapstructure:"bind_address"`
	Git           GitConfig           `mapstructure:"git"`
	Tailscale     TailscaleConfig     `mapstructure:"tailscale"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
}
```

Delete the `ServerConfig` and `NodeConfig` type declarations. In `Load`, replace the four `server.*`/`node.*` defaults with:

```go
	v.SetDefault("port", 3000)
	v.SetDefault("bind_address", "127.0.0.1")
```

Update the env-var comment to read `// Environment variables: ARGUS_PORT, etc.` In `validate`, replace the four server/node validation calls with:

```go
	if err := validatePort("port", cfg.Port); err != nil {
		return err
	}
	if err := validateIP("bind_address", cfg.BindAddress); err != nil {
		return err
	}
```

- [ ] **Step 4: Run the config tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "refactor(config): collapse server/node ports into single port"
```

---

## Task 2: Update dev tooling + docs for `ARGUS_PORT`

**Files:**
- Modify: `web/vite.config.ts:7`
- Modify: `Makefile:21`
- Modify: `argus.dev.toml.example:6`

- [ ] **Step 1: Point Vite at `ARGUS_PORT`**

In `web/vite.config.ts` line 7, change:

```ts
  const apiPort = process.env.ARGUS_PORT ?? "3000";
```

- [ ] **Step 2: Update the Makefile dev env var**

In `Makefile` line 21, change `ARGUS_SERVER_PORT` to `ARGUS_PORT`:

```make
dev dev-api dev-web: export ARGUS_PORT = 3100
```

- [ ] **Step 3: Update the example config comment**

In `argus.dev.toml.example` line 6, change the comment text `ARGUS_SERVER_PORT=3100` to `ARGUS_PORT=3100`.

- [ ] **Step 4: Verify no stale references remain**

Run: `grep -rn "ARGUS_SERVER_PORT\|ARGUS_NODE_PORT\|server\.port\|node\.port" Makefile web/vite.config.ts argus.dev.toml.example internal/config/config.go`
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add web/vite.config.ts Makefile argus.dev.toml.example
git commit -m "chore: rename ARGUS_SERVER_PORT to ARGUS_PORT"
```

---

## Task 3: Add a permissive CORS middleware to the node API

**Files:**
- Create: `internal/node/api/cors.go`
- Create: `internal/node/api/cors_test.go`

- [ ] **Step 1: Write the failing CORS test**

Create `internal/node/api/cors_test.go`:

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_ReflectsOrigin(t *testing.T) {
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("Origin", "http://pane.example:80")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://pane.example:80" {
		t.Errorf("Allow-Origin = %q, want reflected origin", got)
	}
}

func TestCORS_PreflightShortCircuits(t *testing.T) {
	called := false
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodOptions, "/sessions", nil)
	req.Header.Set("Origin", "http://pane.example")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if called {
		t.Error("preflight must not reach the wrapped handler")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/node/api/ -run TestCORS -v`
Expected: FAIL — `undefined: CORS`.

- [ ] **Step 3: Implement the middleware**

Create `internal/node/api/cors.go`:

```go
package api

import "net/http"

// CORS reflects the request Origin and answers preflight requests. The trust
// boundary is the network (loopback / tailnet), so any origin that can reach
// the node is allowed — this only makes the browser's same-origin policy stop
// blocking the multi-node pane from calling a remote node's API.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/node/api/ -run TestCORS -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/api/cors.go internal/node/api/cors_test.go
git commit -m "feat(node): add permissive CORS middleware"
```

---

## Task 4: Make the node router prefix-agnostic and CORS-wrapped

The router currently registers `/api/...` and `/ws/...`, and is mounted at `/node/`. We move the outer namespace to the mount (Task 5), so the router drops its `/api` segment and serves resources at the root (`/sessions`, `/git/status`, `/ws/...`).

**Files:**
- Modify: `internal/node/api/router.go`
- Test: `internal/node/api/` (existing handler tests)

- [ ] **Step 1: Drop `/api` from every route pattern**

In `internal/node/api/router.go`, remove the `/api` segment from each `mux.HandleFunc` pattern (the `/ws/...` patterns are already correct). The full set becomes:

```go
	mux.HandleFunc("GET /info", handleInfo)

	mux.HandleFunc("GET /sessions", sh.list)
	mux.HandleFunc("POST /sessions", sh.create)
	mux.HandleFunc("GET /sessions/{id}", sh.get)
	mux.HandleFunc("PATCH /sessions/{id}", sh.update)
	mux.HandleFunc("DELETE /sessions/{id}", sh.delete)
	mux.HandleFunc("PUT /sessions/{id}/profile", sh.setProfile)
	mux.HandleFunc("DELETE /sessions/{id}/profile", sh.detachProfile)

	mux.HandleFunc("GET /profiles", sh.listProfiles)

	mux.HandleFunc("GET /git/status", gh.status)
	mux.HandleFunc("GET /git/diff", gh.diff)
	mux.HandleFunc("GET /git/working-diff", gh.workingDiff)
	mux.HandleFunc("GET /git/history", gh.history)
	mux.HandleFunc("GET /git/history/{hash}", gh.commitDetail)
	mux.HandleFunc("GET /git/history/{hash}/full-diff", gh.commitFullDiff)
	mux.HandleFunc("GET /git/compare/branches", gh.compareBranches)
	mux.HandleFunc("GET /git/compare", gh.compare)
	mux.HandleFunc("GET /git/file-content", gh.fileContent)
	mux.HandleFunc("GET /git/file-lines", gh.fileLines)
	mux.HandleFunc("GET /git/check", gh.check)
	mux.HandleFunc("GET /git/branches", gh.branches)
	mux.HandleFunc("POST /git/fetch", gh.fetch)

	mux.HandleFunc("GET /git/review", rh.get)
	mux.HandleFunc("POST /git/review", rh.post)
	mux.HandleFunc("DELETE /git/review", rh.delete)

	mux.HandleFunc("GET /files", fh.list)
	mux.HandleFunc("GET /files/meta", fh.meta)
	mux.HandleFunc("GET /files/content", fh.readContent)
	mux.HandleFunc("PUT /files/content", fh.writeContent)
	mux.HandleFunc("GET /files/search", fh.search)
	mux.HandleFunc("POST /files/upload", fh.upload)

	mux.HandleFunc("GET /code-search", srch.search)
	mux.HandleFunc("GET /code-search/available", srch.available)

	mux.HandleFunc("GET /github/repos", ghub.listRepos)

	mux.HandleFunc("GET /sessions/status", handleStatus(deps.WatcherManager, deps.Database))

	mux.HandleFunc("POST /sessions/{id}/heartbeat", hb.heartbeat)
	mux.HandleFunc("POST /sessions/{id}/acknowledge", hb.acknowledge)
	mux.HandleFunc("POST /sessions/{id}/unread", hb.markUnread)
	mux.HandleFunc("POST /sessions/{id}/read", hb.markRead)
```

(Keep the surrounding handler-construction lines and the `/ws/sessions/{id}` + `/ws/terminal` registrations exactly as they are.)

- [ ] **Step 2: Wrap the returned handler in CORS**

At the end of `NewRouter`, change the final `return mux` to:

```go
	return CORS(mux)
```

- [ ] **Step 3: Fix any in-package tests that hit `/api/...` paths**

Run: `grep -rn '"/api/\|/api/sessions\|/api/git\|/api/files\|/api/info\|/api/profiles\|/api/code-search\|/api/github' internal/node/api/*_test.go`
For each match, drop the `/api` segment from the request path (e.g. `httptest.NewRequest("GET", "/api/sessions", …)` → `"/sessions"`). Leave `/ws/...` paths unchanged.

- [ ] **Step 4: Run the node API tests to verify they pass**

Run: `go test ./internal/node/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/api/router.go internal/node/api/
git commit -m "refactor(node): make router prefix-agnostic, wrap in CORS"
```

---

## Task 5: Remove split subcommands; unify listeners on one port + fixed tailnet :80

**Files:**
- Modify: `cmd/argus/main.go`
- Test: `cmd/argus/main_test.go` (existing)

- [ ] **Step 1: Delete `newServerCmd` and `newNodeCmd`; trim the command tree**

In `cmd/argus/main.go`, delete the entire `newServerCmd` and `newNodeCmd` functions. In `newRootCmd`, remove the `web` import usage for the server command and change the `AddCommand` block to drop both:

```go
	rootCmd.AddCommand(
		newMigrateCmd(),
		cli.NewSessionCmd(),
		cli.NewInternalCmd(),
		cli.NewToolsCmd(),
	)
```

Rename `runCombined` to `runInstance` (its body changes in Step 3). The root command's `RunE` becomes `return runInstance(cmd.Context())`.

- [ ] **Step 2: Drop the `mode` parameter from `makeListeners` and `deriveHostname`; fix tailnet port at 80**

Change `deriveHostname` to take only the prefix (delete the `mode` parameter and the `if mode == "server" || mode == "node"` suffix block):

```go
func deriveHostname(prefix string) string {
	hostname := prefix
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil {
			return ""
		}
		hostname = "argus-" + sanitizeDNSCompliantHostname(h)
	}
	return sanitizeDNSCompliantHostname(hostname)
}
```

Change `makeListeners`'s signature to drop `mode` (`func makeListeners(ctx context.Context, tsCfg config.TailscaleConfig, bindAddress string, port int) (...)`), update the `deriveHostname(tsCfg.HostnamePrefix, mode)` call to `deriveHostname(tsCfg.HostnamePrefix)`, and change the tsnet HTTP listener to bind the fixed well-known port instead of `port`:

```go
	const tailnetHTTPPort = 80
	// tsnet listeners are userspace on this node's own tailnet IP, so binding
	// :80 needs no privilege and cannot collide with the host's port 80.
	tsLn, err := tsServer.Listen("tcp", ":"+strconv.Itoa(tailnetHTTPPort))
```

(The loopback listener still uses `port`; `discoveryAddr` stays the loopback address.)

- [ ] **Step 3: Rewrite `runInstance` to mount `/api/node` + SPA**

The body of the renamed `runInstance` becomes:

```go
func runInstance(ctx context.Context) error {
	listeners, discoveryAddr, tsFQDN, tsCloser, err := makeListeners(ctx, cfg.Tailscale, cfg.BindAddress, cfg.Port)
	if err != nil {
		return err
	}
	if tsCloser != nil {
		defer func() {
			if err := tsCloser(); err != nil {
				log.Printf("warning: tailscale shutdown: %v", err)
			}
		}()
	}

	var baseURL string
	if tsFQDN != "" {
		baseURL = "http://" + tsFQDN
	}

	nodeHandler, cleanup, err := node.Setup(cfg, baseURL)
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/api/node/", http.StripPrefix("/api/node", nodeHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	return serve(listeners, discoveryAddr, mux, "argus")
}
```

- [ ] **Step 4: Update `main_test.go` for the removed commands/signatures**

Run: `grep -n "newServerCmd\|newNodeCmd\|deriveHostname\|makeListeners\|runCombined\|\"server\"\|\"node\"" cmd/argus/main_test.go`
For each: delete tests asserting the `server`/`node` subcommands exist; update `deriveHostname(...)` calls to the single-arg form; update `makeListeners(...)` calls to drop the `mode` argument; rename any `runCombined` reference to `runInstance`. If a test asserts the tailnet listener binds the configured port, change its expectation to `80`.

- [ ] **Step 5: Build + run the command tests**

Run: `go build ./... && go test ./cmd/... -v`
Expected: PASS, clean build.

- [ ] **Step 6: Commit**

```bash
git add cmd/argus/main.go cmd/argus/main_test.go
git commit -m "refactor(cmd): single unified instance, drop server/node modes, fixed tailnet :80"
```

---

## Task 6: Migrate the CLI client to `/api/node`

The CLI talks to the local node over loopback. Its base URL ends in `/node` and its call paths start with `/api/`; both move so requests land on `/api/node/<resource>`.

**Files:**
- Modify: `cmd/argus/cli/client.go:70`
- Modify: `cmd/argus/cli/session_attach.go:97` and all `cmd/argus/cli/*.go` call sites

- [ ] **Step 1: Point the client base URL at `/api/node`**

In `cmd/argus/cli/client.go` line ~70, change:

```go
		baseURL: "http://" + info.Address + "/api/node",
```

Also update the doc comment on the `baseURL` field (line ~58) to `// e.g. "http://127.0.0.1:3000/api/node"`.

- [ ] **Step 2: Drop `/api` from every CLI call path**

In `cmd/argus/cli/*.go` (non-test), replace each call-path literal that starts with `/api/` so it starts with `/` instead. The affected call sites are:
`c.post("/api/sessions"...)`, `c.patch("/api/sessions/"...)`, `c.get("/api/sessions/"...)`, `c.post("/api/sessions/"+...+"/acknowledge"...)`, `baseURL + "/api/sessions/" + sessionID + "/heartbeat"`, `c.delete("/api/sessions/"...)`, `c.get("/api/sessions")`, `c.get("/api/sessions/status")`, `c.get("/api/git/status?"...)`, `c.get("/api/git/compare/branches?"...)`, `c.put("/api/sessions/"+...+"/profile"...)`, `c.delete("/api/sessions/"+...+"/profile")`.

Each becomes the same string with `/api/sessions` → `/sessions` and `/api/git` → `/git`.

- [ ] **Step 3: Verify no `/api/` call paths remain in the CLI**

Run: `grep -rn '"/api/\|+ "/api/\|baseURL + "/api/' cmd/argus/cli/`
Expected: no output.

- [ ] **Step 4: Build + run the CLI tests**

Run: `go build ./... && go test ./cmd/argus/cli/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/argus/cli/
git commit -m "refactor(cli): move node calls to /api/node namespace"
```

---

## Task 7: Migrate the web client to `/api/node`

The web data layer uses `/node/api/...` and `/node/ws/...` — a clean prefix swap to `/api/node/...` and `/api/node/ws/...` (the `/api` segment stays; only the order changes).

**Files:**
- Modify (15): `web/src/api/client.ts`, `web/src/App.tsx`, `web/src/types.ts`, `web/src/data/{files/queries.ts,files/mutations.ts,git/queries.ts,git/mutations.ts,git/file-lines.ts,review/queries.ts,sessions/queries.ts,statuses/queries.ts,github/queries.ts}`, `web/src/components/FileExplorer/{FileTree.tsx,useFileEditor.ts}`, `web/src/components/Terminal/index.tsx`
- Modify: `web/vite.config.ts` (proxy keys)

- [ ] **Step 1: Swap the path prefixes across `web/src`**

Apply two literal replacements across all `web/src` files: `/node/ws/` → `/api/node/ws/`, then `/node/api/` → `/api/node/`. (Order matters only in that both are independent substrings; do the `ws` one first to keep it explicit.) Note `web/src/api/client.ts`'s `getNodeWsUrl` builds the `/node/ws/...` path string — it is covered by the same `/node/ws/` → `/api/node/ws/` swap.

- [ ] **Step 2: Update the Vite dev proxy keys**

In `web/vite.config.ts`, replace the proxy block so the keys match the new paths (the more specific `ws` key first):

```ts
      proxy: {
        "/api/node/ws": {
          target: wsTarget,
          ws: true,
        },
        "/api/node": {
          target: apiTarget,
          changeOrigin: true,
        },
      },
```

- [ ] **Step 3: Verify no stale `/node/api` or `/node/ws` remain**

Run: `grep -rn "/node/api\|/node/ws" web/src web/vite.config.ts`
Expected: no output.

- [ ] **Step 4: Run the web tests + typecheck**

Run: `cd web && pnpm test run && pnpm exec tsc --noEmit`
Expected: PASS, no type errors.

- [ ] **Step 5: Commit**

```bash
git add web/src web/vite.config.ts
git commit -m "refactor(web): move node API/WS calls to /api/node namespace"
```

---

## Task 8: Full build + smoke gate

**Files:** none (verification only)

- [ ] **Step 1: Full Go build, vet, and test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: clean build, no vet errors, all tests PASS.

- [ ] **Step 2: Web build**

Run: `cd web && pnpm build`
Expected: build succeeds (emits `internal/web/dist`).

- [ ] **Step 3: Manual smoke — single instance still works on new paths**

Run (in a scratch home so prod state is untouched):
```bash
ARGUS_HOME=$(mktemp -d) ARGUS_PORT=3199 go run ./cmd/argus &
sleep 2
curl -fsS http://127.0.0.1:3199/api/node/sessions | head -c 200; echo
curl -fsS http://127.0.0.1:3199/ -o /dev/null -w "SPA %{http_code}\n"
curl -fsS -X OPTIONS -H "Origin: http://x" -H "Access-Control-Request-Method: GET" \
  http://127.0.0.1:3199/api/node/sessions -o /dev/null -w "preflight %{http_code}\n"
kill %1
```
Expected: `/api/node/sessions` returns a JSON sessions payload; `SPA 200`; `preflight 204`.

- [ ] **Step 4: Confirm the old subcommands are gone**

Run: `go run ./cmd/argus server 2>&1 | head -3; go run ./cmd/argus node 2>&1 | head -3`
Expected: cobra "unknown command" errors for both.

- [ ] **Step 5: Commit any test/smoke fixups**

```bash
git add -A
git commit -m "test: green build after instance unification" --allow-empty
```

---

## Self-Review

**Spec coverage (Plan 1 scope):**
- Unified instance / remove split commands → Task 5. ✅
- Single `port` config, drop `node.port` → Task 1; env var rename → Task 2. ✅
- Fixed tailnet `:80` → Task 5 Step 2. ✅
- `/api/node/*` namespace (router prefix-agnostic + mount) → Tasks 4, 5. ✅
- CLI + web migration → Tasks 6, 7. ✅
- CORS middleware → Tasks 3, 4. ✅ (WS `CheckOrigin` already returns `true` — no change needed.)
- Registry, discovery, summary, UI → **deferred to Plans 2 & 3** (not in scope).

**Placeholder scan:** none — every step has concrete code/commands. Mechanical mass-replacements (Tasks 6/7) give the exact transformation plus a verification `grep` that must return empty.

**Type/name consistency:** `runCombined`→`runInstance` (Task 5) used consistently; `CORS` defined in Task 3, referenced in Task 4; config fields `Port`/`BindAddress` defined in Task 1, consumed in Task 5; `ARGUS_PORT` consistent across Tasks 1, 2.

**Note for executor:** `deriveHostname` previously appended `-server`/`-node` suffixes so split processes got distinct Tailscale hostnames. With one instance per machine that distinction is gone; if you run an instance alongside a legacy one on the same host, set `tailscale.hostname_prefix` to disambiguate.
