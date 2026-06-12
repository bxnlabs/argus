# Multi-node Part 2 — Node Registry, Discovery & Summary — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give an Argus instance a server-side registry of peer nodes — its own local node, manually-added nodes (persisted in SQLite), and nodes discovered via Tailscale tags — exposed at `/api/nodes`, plus a lightweight `/api/node/summary` endpoint each node serves for at-a-glance attention/activity counts.

**Architecture:** A new `internal/registry` package merges three node sources behind one `GET /api/nodes` endpoint, deduped by normalized URL with precedence `local > manual > discovered`. Manual nodes live in a new `nodes` table in the existing single SQLite DB (added via the established schema + migration framework). Discovery reads tailnet peers through a new `tailscale.Server.Peers()` method, filtered by a configurable tag. The summary endpoint is a condensed version of the existing status handler. Wiring hoists DB ownership into `node.Setup` (now returns the `*db.DB`) and threads a discovery function from `runInstance` (which owns the tsnet server).

**Tech Stack:** Go (net/http `ServeMux`, modernc SQLite, tsnet / `LocalClient().Status`).

This is **Plan 2 of 3** for BXN-106. It builds on Plan 1 (unified instance, `/api/node/*` namespace, CORS). It ships green: the registry + summary APIs work and are tested, but the UI is unchanged (Plan 3 consumes them). Spec: `docs/superpowers/specs/2026-06-05-multi-node-ui-design.md`.

**Prerequisite:** Plan 1 merged. Key Plan-1 outcomes this plan depends on: `cmd/argus/main.go` has `runInstance` calling `node.Setup(cfg, baseURL) (http.Handler, func(), error)` and `makeListeners(ctx, tsCfg, bindAddress, port) (listeners, discoveryAddr, tsFQDN, tsCloser, err)`; the node router (`internal/node/api/router.go`) registers prefix-less routes (`/sessions`, …) and returns `CORS(mux)`.

---

## File Structure

**Created:**
- `internal/node/db/nodes.go` — manual-node CRUD on `*db.DB`.
- `internal/node/db/nodes_test.go` — CRUD + migration tests.
- `internal/registry/registry.go` — `Node`, `Source`, `Service.List` merge/dedup.
- `internal/registry/registry_test.go` — merge/dedup/precedence tests.
- `internal/registry/handler.go` — `/api/nodes` HTTP handlers.
- `internal/registry/handler_test.go` — endpoint tests.
- `internal/node/api/summary.go` — `/summary` handler.
- `internal/node/api/summary_test.go` — summary count tests.

**Modified:**
- `internal/node/db/schema.go` — add `nodes` table.
- `internal/node/db/migrations.go` — add `add_nodes_table` migration.
- `internal/node/db/db.go` — seed `add_nodes_table` on fresh DBs.
- `internal/config/config.go` — add `tailscale.discovery_tag`.
- `internal/tailscale/server.go` — add `Peers(ctx)`.
- `internal/node/api/router.go` — register `/summary`.
- `internal/node/setup.go` — return the `*db.DB`.
- `cmd/argus/main.go` — `makeListeners` returns the `*ts.Server`; `runInstance` mounts the registry + wires discovery.

---

## Task 1: Add the `nodes` table (schema + migration + seed)

**Files:**
- Modify: `internal/node/db/schema.go`
- Modify: `internal/node/db/migrations.go`
- Modify: `internal/node/db/db.go`
- Test: `internal/node/db/nodes_test.go` (created here)

- [ ] **Step 1: Write a failing migration test**

Create `internal/node/db/nodes_test.go`:

```go
package db

import (
	"path/filepath"
	"testing"
)

func TestNodesTablePresentOnFreshDB(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Fresh DB must not report pending migrations (add_nodes_table seeded).
	if err := d.CheckMigrations(); err != nil {
		t.Fatalf("CheckMigrations on fresh DB: %v", err)
	}

	var count int
	if err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='nodes'`,
	).Scan(&count); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("nodes table count = %d, want 1", count)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/node/db/ -run TestNodesTablePresentOnFreshDB -v`
Expected: FAIL — `CheckMigrations` reports `add_nodes_table` pending (no table yet).

- [ ] **Step 3: Add the table to the base schema**

In `internal/node/db/schema.go`, add this block to the `schema` string (after the `notifications` table, before `_migrations`):

```sql
-- Peer node registry (manually-added nodes; local/discovered are computed)
CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  url TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

- [ ] **Step 4: Add the migration**

In `internal/node/db/migrations.go`, append to the `allMigrations` slice (after `add_user_marked_unread_at`):

```go
	{"add_nodes_table", func(d *DB) error {
		_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  url TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
)`)
		return err
	}},
```

- [ ] **Step 5: Seed the migration on fresh DBs**

In `internal/node/db/db.go`'s `seedMigrations`, extend the fresh-DB block (the `if priorMigrations == 0 && allColumnsPresent {` block that seeds `create_notifications_table`) to also seed the nodes table. Replace that single `INSERT OR IGNORE` with both:

```go
	if priorMigrations == 0 && allColumnsPresent {
		for _, name := range []string{"create_notifications_table", "add_nodes_table"} {
			if _, err := d.sql.Exec(
				`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`, name,
			); err != nil {
				return err
			}
		}
	}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `go test ./internal/node/db/ -run TestNodesTablePresentOnFreshDB -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/node/db/schema.go internal/node/db/migrations.go internal/node/db/db.go internal/node/db/nodes_test.go
git commit -m "feat(db): add nodes registry table + migration"
```

---

## Task 2: Manual-node CRUD on the DB

**Files:**
- Create: `internal/node/db/nodes.go`
- Test: `internal/node/db/nodes_test.go` (extend)

- [ ] **Step 1: Write failing CRUD tests**

Append to `internal/node/db/nodes_test.go`:

```go
import "context" // add to the existing import block

func TestManualNodeCRUD(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()
	ctx := context.Background()

	if err := d.AddManualNode(ctx, "n1", "gpu-box", "http://gpu-box:80"); err != nil {
		t.Fatalf("AddManualNode: %v", err)
	}
	nodes, err := d.ListManualNodes(ctx)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("ListManualNodes = %v, %v; want 1 node", nodes, err)
	}
	if nodes[0].Name != "gpu-box" || nodes[0].URL != "http://gpu-box:80" {
		t.Errorf("node = %+v", nodes[0])
	}

	if err := d.RenameManualNode(ctx, "n1", "gpu-1"); err != nil {
		t.Fatalf("RenameManualNode: %v", err)
	}
	nodes, _ = d.ListManualNodes(ctx)
	if nodes[0].Name != "gpu-1" {
		t.Errorf("after rename name = %q, want gpu-1", nodes[0].Name)
	}

	if err := d.DeleteManualNode(ctx, "n1"); err != nil {
		t.Fatalf("DeleteManualNode: %v", err)
	}
	nodes, _ = d.ListManualNodes(ctx)
	if len(nodes) != 0 {
		t.Errorf("after delete len = %d, want 0", len(nodes))
	}
}

func TestAddManualNodeRejectsDuplicateURL(t *testing.T) {
	d, _ := Open(filepath.Join(t.TempDir(), "test.db"))
	defer d.Close()
	ctx := context.Background()
	_ = d.AddManualNode(ctx, "n1", "a", "http://dup:80")
	if err := d.AddManualNode(ctx, "n2", "b", "http://dup:80"); err == nil {
		t.Error("expected duplicate-URL insert to fail")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/node/db/ -run 'TestManualNodeCRUD|TestAddManualNodeRejectsDuplicateURL' -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement the CRUD methods**

Create `internal/node/db/nodes.go`:

```go
package db

import "context"

// ManualNode is a user-added peer node persisted in the registry.
type ManualNode struct {
	ID        string
	Name      string
	URL       string
	CreatedAt string
}

// ListManualNodes returns all manually-added nodes, oldest first.
func (d *DB) ListManualNodes(ctx context.Context) ([]ManualNode, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, name, url, created_at FROM nodes ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ManualNode
	for rows.Next() {
		var n ManualNode
		if err := rows.Scan(&n.ID, &n.Name, &n.URL, &n.CreatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// AddManualNode inserts a node. The UNIQUE(url) constraint surfaces duplicates
// as an error.
func (d *DB) AddManualNode(ctx context.Context, id, name, url string) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO nodes (id, name, url) VALUES (?, ?, ?)`, id, name, url)
	return err
}

// RenameManualNode updates a node's display name.
func (d *DB) RenameManualNode(ctx context.Context, id, name string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE nodes SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteManualNode removes a node by id.
func (d *DB) DeleteManualNode(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/node/db/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/db/nodes.go internal/node/db/nodes_test.go
git commit -m "feat(db): manual-node registry CRUD"
```

---

## Task 3: Tailscale `Peers()` + `discovery_tag` config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/tailscale/server.go`

- [ ] **Step 1: Add the `discovery_tag` config field + default**

In `internal/config/config.go`, add to `TailscaleConfig`:

```go
	DiscoveryTag string `mapstructure:"discovery_tag"`
```

and in `Load`'s defaults:

```go
	v.SetDefault("tailscale.discovery_tag", "tag:argus-node")
```

(No new validation needed.)

- [ ] **Step 2: Add `Peers()` to the Tailscale wrapper**

In `internal/tailscale/server.go`, add a `Peer` type and method. **Verify the `ipnstate.PeerStatus` field shape against the pinned tailscale version before relying on it** (the spec flags this as the one spike — `go doc tailscale.com/ipn/ipnstate.PeerStatus`). Expected shape:

```go
// Peer is a discovered tailnet peer relevant to node discovery.
type Peer struct {
	DNSName string   // e.g. "gpu-box.tailnet.ts.net."
	Tags    []string // ACL tags, e.g. ["tag:argus-node"]
	Online  bool
}

// Peers returns the current tailnet peers (excluding self). Must be called
// after Up(). Returns nil when the server hasn't started.
func (s *Server) Peers(ctx context.Context) ([]Peer, error) {
	if !s.started {
		return nil, nil
	}
	lc, err := s.ts.LocalClient()
	if err != nil {
		return nil, err
	}
	st, err := lc.Status(ctx)
	if err != nil {
		return nil, err
	}
	var peers []Peer
	for _, ps := range st.Peer {
		var tags []string
		if ps.Tags != nil {
			for i := range ps.Tags.Len() {
				tags = append(tags, ps.Tags.At(i))
			}
		}
		peers = append(peers, Peer{
			DNSName: ps.DNSName,
			Tags:    tags,
			Online:  ps.Online,
		})
	}
	return peers, nil
}
```

Add `"context"` to the imports if not present. **If `s.ts.LocalClient()` returns a single value in the pinned version** (older tsnet), use `lc := s.ts.LocalClient()` without the error. **If `ps.Tags` is a `*[]string`** rather than a `views.Slice`, iterate it directly. Confirm via `go doc tailscale.com/tsnet.Server.LocalClient` and adjust.

- [ ] **Step 3: Build to verify it compiles**

Run: `go build ./internal/tailscale/ ./internal/config/`
Expected: clean build. (If it fails, the `ipnstate` field shape differs — adjust per Step 2's notes.)

- [ ] **Step 4: Commit**

```bash
git add internal/config/config.go internal/tailscale/server.go
git commit -m "feat(tailscale): Peers() enumeration + discovery_tag config"
```

---

## Task 4: Registry merge service

**Files:**
- Create: `internal/registry/registry.go`
- Test: `internal/registry/registry_test.go`

- [ ] **Step 1: Write failing merge tests**

Create `internal/registry/registry_test.go`:

```go
package registry

import (
	"context"
	"testing"
)

type fakeDB struct{ nodes []ManualNode }

func (f *fakeDB) ListManualNodes(context.Context) ([]ManualNode, error) { return f.nodes, nil }

func TestList_MergesSourcesWithPrecedence(t *testing.T) {
	db := &fakeDB{nodes: []ManualNode{{ID: "m1", Name: "manual-gpu", URL: "http://gpu-box:80"}}}
	self := Node{ID: "self", Name: "this", URL: "", Source: SourceLocal, Self: true, dedupKey: "http://laptop:80"}
	discover := func(context.Context) ([]Node, error) {
		return []Node{
			{ID: "d1", Name: "gpu-box", URL: "http://gpu-box:80", Source: SourceDiscovered, dedupKey: "http://gpu-box:80"},
			{ID: "d2", Name: "laptop", URL: "http://laptop:80", Source: SourceDiscovered, dedupKey: "http://laptop:80"},
			{ID: "d3", Name: "ci", URL: "http://ci:80", Source: SourceDiscovered, dedupKey: "http://ci:80"},
		}, nil
	}
	svc := New(db, self, discover)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Expect: self (local) + manual-gpu (manual wins over discovered d1) + ci.
	// laptop (d2) is dropped as a duplicate of self.
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(got), got)
	}
	bySource := map[Source]int{}
	for _, n := range got {
		bySource[n.Source]++
	}
	if bySource[SourceLocal] != 1 || bySource[SourceManual] != 1 || bySource[SourceDiscovered] != 1 {
		t.Errorf("source counts = %v, want 1 each", bySource)
	}
	for _, n := range got {
		if n.URL == "http://gpu-box:80" && n.Source != SourceManual {
			t.Errorf("gpu-box should be manual, got %s", n.Source)
		}
	}
}

func TestList_DiscoveryErrorDegrades(t *testing.T) {
	db := &fakeDB{nodes: []ManualNode{{ID: "m1", Name: "a", URL: "http://a:80"}}}
	self := Node{ID: "self", Source: SourceLocal, Self: true}
	discover := func(context.Context) ([]Node, error) { return nil, context.DeadlineExceeded }
	svc := New(db, self, discover)

	got, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List should not error when discovery fails: %v", err)
	}
	if len(got) != 2 { // self + manual
		t.Errorf("len = %d, want 2", len(got))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/registry/ -v`
Expected: FAIL — package/types undefined.

- [ ] **Step 3: Implement the service**

Create `internal/registry/registry.go`:

```go
package registry

import (
	"context"
	"net/url"
	"strings"
)

// Source identifies how a node entered the registry.
type Source string

const (
	SourceLocal      Source = "local"
	SourceManual     Source = "manual"
	SourceDiscovered Source = "discovered"
)

// Node is a registry entry as returned to the client.
type Node struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"` // empty == same-origin (the local node)
	Source Source `json:"source"`
	Self   bool   `json:"self"`

	// dedupKey is the normalized URL used to collapse duplicates across
	// sources. For the local node it is its own tailnet URL (so a discovered
	// copy of self is dropped); for others it is normalize(URL). Not serialized.
	dedupKey string `json:"-"`
}

// ManualNode mirrors db.ManualNode without importing the db package.
type ManualNode struct {
	ID   string
	Name string
	URL  string
}

// DB is the persistence dependency (satisfied by *db.DB).
type DB interface {
	ListManualNodes(ctx context.Context) ([]ManualNode, error)
}

// DiscoverFunc returns discovered nodes (already shaped as registry.Node with a
// dedupKey set). It is best-effort: an error degrades to local+manual.
type DiscoverFunc func(ctx context.Context) ([]Node, error)

// Service merges the three node sources.
type Service struct {
	db       DB
	self     Node
	discover DiscoverFunc
}

// New builds a registry service. discover may be nil (no Tailscale).
func New(db DB, self Node, discover DiscoverFunc) *Service {
	return &Service{db: db, self: self, discover: discover}
}

// normalize lowercases the host and strips a trailing slash so that
// "http://Gpu-Box:80/" and "http://gpu-box:80" collapse.
func normalize(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.ToLower(raw)
	}
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

// List returns local + manual + discovered, deduped by normalized URL with
// precedence local > manual > discovered.
func (s *Service) List(ctx context.Context) ([]Node, error) {
	seen := map[string]bool{}
	var out []Node

	// 1. Local (self) always first.
	self := s.self
	if self.dedupKey != "" {
		seen[self.dedupKey] = true
	}
	out = append(out, self)

	// 2. Manual.
	manual, err := s.db.ListManualNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range manual {
		key := normalize(m.URL)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Node{ID: m.ID, Name: m.Name, URL: m.URL, Source: SourceManual, dedupKey: key})
	}

	// 3. Discovered (best-effort).
	if s.discover != nil {
		discovered, derr := s.discover(ctx)
		if derr == nil {
			for _, d := range discovered {
				key := d.dedupKey
				if key == "" {
					key = normalize(d.URL)
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				d.Source = SourceDiscovered
				d.dedupKey = key
				out = append(out, d)
			}
		}
	}

	return out, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/registry/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/registry/registry.go internal/registry/registry_test.go
git commit -m "feat(registry): merge local/manual/discovered nodes"
```

---

## Task 5: Registry HTTP handlers (`/api/nodes`)

**Files:**
- Create: `internal/registry/handler.go`
- Test: `internal/registry/handler_test.go`

- [ ] **Step 1: Write failing handler tests**

Create `internal/registry/handler_test.go`:

```go
package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type memDB struct{ nodes []ManualNode }

func (m *memDB) ListManualNodes(context.Context) ([]ManualNode, error) { return m.nodes, nil }
func (m *memDB) AddManualNode(_ context.Context, id, name, url string) error {
	m.nodes = append(m.nodes, ManualNode{ID: id, Name: name, URL: url})
	return nil
}
func (m *memDB) RenameManualNode(_ context.Context, id, name string) error {
	for i := range m.nodes {
		if m.nodes[i].ID == id {
			m.nodes[i].Name = name
		}
	}
	return nil
}
func (m *memDB) DeleteManualNode(_ context.Context, id string) error {
	out := m.nodes[:0]
	for _, n := range m.nodes {
		if n.ID != id {
			out = append(out, n)
		}
	}
	m.nodes = out
	return nil
}

func newTestHandlers() (*Handlers, *memDB) {
	db := &memDB{}
	svc := New(db, Node{ID: "self", Name: "this", Source: SourceLocal, Self: true}, nil)
	return NewHandlers(svc, db, func() string { return "id-1" }), db
}

func TestList_ReturnsSelf(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodes", nil))
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct{ Nodes []Node }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Nodes) != 1 || !body.Nodes[0].Self {
		t.Errorf("nodes = %+v", body.Nodes)
	}
}

func TestAddThenList(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"gpu","url":"http://gpu:80"}`)))
	if rec.Code != 201 {
		t.Fatalf("add status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/nodes", nil))
	var body struct{ Nodes []Node }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Nodes) != 2 {
		t.Errorf("len = %d, want 2 (self+gpu)", len(body.Nodes))
	}
}

func TestAddRejectsBadURL(t *testing.T) {
	h, _ := newTestHandlers()
	mux := http.NewServeMux()
	h.Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/nodes",
		strings.NewReader(`{"name":"x","url":"not-a-url"}`)))
	if rec.Code != 400 {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/registry/ -run 'TestList_Returns|TestAdd' -v`
Expected: FAIL — `Handlers`/`NewHandlers` undefined.

- [ ] **Step 3: Implement the handlers**

Create `internal/registry/handler.go`:

```go
package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// WriteDB is the persistence dependency for mutations (satisfied by *db.DB).
type WriteDB interface {
	DB
	AddManualNode(ctx context.Context, id, name, url string) error
	RenameManualNode(ctx context.Context, id, name string) error
	DeleteManualNode(ctx context.Context, id string) error
}

// Handlers serves the /api/nodes registry endpoints.
type Handlers struct {
	svc    *Service
	db     WriteDB
	newID  func() string
}

// NewHandlers builds the registry handlers. newID generates manual-node IDs
// (inject a deterministic one in tests; use a UUID in production).
func NewHandlers(svc *Service, db WriteDB, newID func() string) *Handlers {
	return &Handlers{svc: svc, db: db, newID: newID}
}

// Register wires the routes onto the given mux. Method+path patterns coexist
// with the node mount at "/api/node/" (different path segment).
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nodes", h.list)
	mux.HandleFunc("POST /api/nodes", h.add)
	mux.HandleFunc("PATCH /api/nodes/{id}", h.rename)
	mux.HandleFunc("DELETE /api/nodes/{id}", h.delete)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.svc.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func (h *Handlers) add(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.URL = strings.TrimRight(strings.TrimSpace(body.URL), "/")
	if body.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must be an absolute http(s) URL"})
		return
	}
	id := h.newID()
	if err := h.db.AddManualNode(r.Context(), id, body.Name, body.URL); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (h *Handlers) rename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if err := h.db.RenameManualNode(r.Context(), id, strings.TrimSpace(body.Name)); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteManualNode(r.Context(), r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/registry/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/registry/handler.go internal/registry/handler_test.go
git commit -m "feat(registry): /api/nodes CRUD handlers"
```

---

## Task 6: `/api/node/summary` endpoint

**Files:**
- Create: `internal/node/api/summary.go`
- Create: `internal/node/api/summary_test.go`
- Modify: `internal/node/api/router.go`

- [ ] **Step 1: Write a failing summary test**

Create `internal/node/api/summary_test.go`. Model it on the existing `status_test.go` setup (reuse its DB + watcher-manager test helpers — open the test file to copy the construction of `deps.Database` and `deps.WatcherManager`). The assertion:

```go
func TestSummary_CountsAttentionAndBusy(t *testing.T) {
	// Arrange: 3 sessions — one active, one idle+unread, one idle clean.
	// (Use the same helpers status_test.go uses to seed sessions + snapshot.)
	// ... seed: sActive (snapshot active), sUnread (unread_since set, idle),
	//     sClean (idle, no unread) ...

	req := httptest.NewRequest("GET", "/summary", nil)
	rec := httptest.NewRecorder()
	handleSummary(watcherMgr, database).ServeHTTP(rec, req)

	var got struct {
		Attention int `json:"attention"`
		Busy      int `json:"busy"`
		Total     int `json:"total"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if got.Total != 3 || got.Busy != 1 || got.Attention != 1 {
		t.Errorf("summary = %+v, want total=3 busy=1 attention=1", got)
	}
}
```

(The executor should mirror `status_test.go`'s exact seeding helpers; the behavioral contract is the three counts above.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/node/api/ -run TestSummary -v`
Expected: FAIL — `handleSummary` undefined.

- [ ] **Step 3: Implement the handler**

Create `internal/node/api/summary.go`:

```go
package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/status"
)

// handleSummary returns a handler for GET /summary (external /api/node/summary).
// It reports lightweight per-node counts for the multi-node node list:
//   - busy:      sessions currently active (agent running)
//   - attention: idle/dead sessions that are unread (waiting on the user)
//   - total:     all sessions
// "attention" mirrors the sidebar rule: a session needs attention when it is
// unread (auto unread_since OR manual user_marked_unread_at) and not active.
func handleSummary(mgr *status.WatcherManager, database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := mgr.Snapshot()
		sessions, err := database.ListSessions(r.Context())
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		var busy, attention int
		for _, s := range sessions {
			active := false
			if entry, ok := snap.Statuses[s.ID]; ok && entry.State == status.StateActive {
				active = true
			}
			unread := s.UnreadSince != nil || s.UserMarkedUnreadAt != nil
			if active {
				busy++
			} else if unread {
				attention++
			}
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"attention": attention,
			"busy":      busy,
			"total":     len(sessions),
		})
	}
}
```

- [ ] **Step 4: Register the route**

In `internal/node/api/router.go`, add alongside the other session routes (it needs `deps.WatcherManager` + `deps.Database`, both already in `Deps`):

```go
	mux.HandleFunc("GET /summary", handleSummary(deps.WatcherManager, deps.Database))
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/node/api/ -run TestSummary -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/node/api/summary.go internal/node/api/summary_test.go internal/node/api/router.go
git commit -m "feat(node): /api/node/summary attention/busy counts"
```

---

## Task 7: Wire the registry + discovery into the instance

This threads the pieces together: `node.Setup` exposes the DB; `makeListeners` exposes the tsnet server; `runInstance` builds the self node, the discovery function, and mounts `/api/nodes`.

**Files:**
- Modify: `internal/node/setup.go`
- Modify: `cmd/argus/main.go`
- Add dependency: a UUID generator (check `go.mod` for `github.com/google/uuid`; if absent, use the crypto/rand helper below).

- [ ] **Step 1: Expose the DB from `node.Setup`**

In `internal/node/setup.go`, change `Setup` to return the `*db.DB` as well. New signature + return sites:

```go
func Setup(cfg *config.Config, baseURL string) (http.Handler, *db.DB, func(), error) {
```

Update every `return nil, nil, ...` to `return nil, nil, nil, ...`, and the final success return to:

```go
	return handler, database, cleanup, nil
```

- [ ] **Step 2: Update the `node.Setup` call in `runInstance`**

In `cmd/argus/main.go` `runInstance`, change the call + add the DB:

```go
	nodeHandler, database, cleanup, err := node.Setup(cfg, baseURL)
	if err != nil {
		return err
	}
	defer cleanup()
```

- [ ] **Step 3: Return the tsnet server from `makeListeners`**

In `cmd/argus/main.go`, change `makeListeners` to also return the `*ts.Server` (nil when Tailscale is disabled). Add it to the named returns: `(..., tsCloser func() error, tsServer *ts.Server, err error)`. In the Tailscale-disabled branches return `nil` for it; in the enabled branch return the created `tsServer`. Update the `runInstance` call:

```go
	listeners, discoveryAddr, tsFQDN, tsCloser, tsServer, err := makeListeners(ctx, cfg.Tailscale, cfg.BindAddress, cfg.Port)
```

- [ ] **Step 4: Build the self node + discovery + mount the registry**

In `cmd/argus/main.go` `runInstance`, after `node.Setup` and before `serve(...)`, add:

```go
	// Self entry: same-origin from the browser (URL empty). Its dedup key is
	// the tailnet URL (if any) so a discovered copy of self is collapsed.
	selfName := cfg.Tailscale.HostnamePrefix
	if selfName == "" {
		if h, err := os.Hostname(); err == nil {
			selfName = h
		} else {
			selfName = "this node"
		}
	}
	self := registry.Node{
		ID:     "local",
		Name:   selfName,
		URL:    "",
		Source: registry.SourceLocal,
		Self:   true,
	}
	self.SetDedupKey(baseURL) // baseURL is "http://<fqdn>" or "" when no Tailscale

	var discover registry.DiscoverFunc
	if tsServer != nil {
		tag := cfg.Tailscale.DiscoveryTag
		discover = func(ctx context.Context) ([]registry.Node, error) {
			peers, err := tsServer.Peers(ctx)
			if err != nil {
				return nil, err
			}
			var nodes []registry.Node
			for _, p := range peers {
				if !hasTag(p.Tags, tag) {
					continue
				}
				host := strings.TrimSuffix(p.DNSName, ".")
				if host == "" {
					continue
				}
				url := "http://" + host // :80 implicit
				nodes = append(nodes, registry.NodeFromDiscovery(host, url))
			}
			return nodes, nil
		}
	}

	regSvc := registry.New(database, self, discover)
	regHandlers := registry.NewHandlers(regSvc, database, newNodeID)
	regHandlers.Register(mux)
```

Add these helpers to `main.go`:

```go
func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// newNodeID returns a random hex id for a manual node.
func newNodeID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
```

Add imports to `main.go`: `"crypto/rand"`, `"encoding/hex"`, `"github.com/bxnlabs/argus/internal/registry"` (and `"strings"` if not already imported — it is, from Plan 1). Ensure the `mux` is created before this block (it is, in Plan 1's `runInstance`), and that the registry routes register on the same `mux` that already has `/api/node/` and `/`.

- [ ] **Step 5: Add the two small registry constructors used above**

In `internal/registry/registry.go`, export the dedup helpers referenced by `main.go` (the `dedupKey` field is unexported, so provide setters/constructors):

```go
// SetDedupKey sets the local node's dedup key from its canonical (tailnet) URL.
func (n *Node) SetDedupKey(canonicalURL string) {
	if canonicalURL != "" {
		n.dedupKey = normalize(canonicalURL)
	}
}

// NodeFromDiscovery builds a discovered node with its dedup key set.
func NodeFromDiscovery(name, rawURL string) Node {
	return Node{
		ID:       "discovered:" + normalize(rawURL),
		Name:     name,
		URL:      rawURL,
		Source:   SourceDiscovered,
		dedupKey: normalize(rawURL),
	}
}
```

- [ ] **Step 6: Build + run the full Go test suite**

Run: `go build ./... && go test ./...`
Expected: clean build, all PASS. (If `main.go` complains about an unused `tsCloser`/`tsServer` in a branch, ensure both are returned and used.)

- [ ] **Step 7: Commit**

```bash
git add internal/node/setup.go cmd/argus/main.go internal/registry/registry.go
git commit -m "feat: wire node registry + Tailscale discovery into the instance"
```

---

## Task 8: Migration command + end-to-end smoke

**Files:** none (verification only); confirms existing-install upgrade path.

- [ ] **Step 1: Confirm `argus migrate` applies the new table on an existing DB**

Create a DB at the previous schema, then migrate:
```bash
TMP=$(mktemp -d)
# Simulate an existing DB by removing the seeded nodes migration:
ARGUS_HOME=$TMP go run ./cmd/argus migrate 2>&1 | tail -3
```
Expected: migrate runs without error; on a fresh DB it is a no-op (already seeded). (The contract is that `node.Setup`'s `CheckMigrations` passes after `migrate`.)

- [ ] **Step 2: Smoke the registry + summary endpoints**

```bash
ARGUS_HOME=$(mktemp -d) ARGUS_PORT=3199 go run ./cmd/argus &
sleep 2
echo "--- list (expect self only) ---"
curl -fsS http://127.0.0.1:3199/api/nodes
echo; echo "--- add ---"
curl -fsS -X POST http://127.0.0.1:3199/api/nodes -d '{"name":"gpu","url":"http://gpu-box:80"}' -w " [%{http_code}]\n"
echo "--- list (expect self + gpu) ---"
curl -fsS http://127.0.0.1:3199/api/nodes
echo; echo "--- summary ---"
curl -fsS http://127.0.0.1:3199/api/node/summary
echo; kill %1
```
Expected: first list has one node with `"self":true`; add returns `[201]`; second list has two nodes (`local` + `manual`); summary returns `{"attention":…,"busy":…,"total":…}`.

- [ ] **Step 3: Commit any fixups**

```bash
git add -A
git commit -m "test: registry + summary smoke green" --allow-empty
```

---

## Self-Review

**Spec coverage (Plan 2 scope):**
- `nodes` table in the single SQLite DB via schema + migration → Task 1. ✅
- Manual-node CRUD → Task 2. ✅
- Tailscale tag discovery (`Peers()` + `discovery_tag`) → Tasks 3, 7. ✅
- Merge local+manual+discovered, dedup by normalized URL, precedence → Task 4. ✅
- `/api/nodes` CRUD endpoints → Task 5. ✅
- `/api/node/summary` (attention/busy/total) → Task 6. ✅
- DB-ownership hoist + wiring → Task 7. ✅
- Existing-install migration path → Task 8. ✅

**Placeholder scan:** the only non-literal step is Task 6 Step 1 (the summary test reuses `status_test.go`'s seeding helpers rather than reproducing them) — its behavioral contract (total=3, busy=1, attention=1) is explicit. Everything else is complete code.

**Type/name consistency:** `registry.Node`/`Source`/`New`/`NewHandlers`/`Register`/`SetDedupKey`/`NodeFromDiscovery`/`DiscoverFunc` defined in Tasks 4–5 and used consistently in Task 7. `db` methods `ListManualNodes`/`AddManualNode`/`RenameManualNode`/`DeleteManualNode` (Task 2) satisfy `registry.DB`/`WriteDB` (Task 4/5). `node.Setup` new 4-value return (Task 7 Step 1) matched at its call site (Step 2). `handleSummary(mgr, database)` (Task 6) matches its route registration.

**Risks called out for the executor:**
- **tsnet field shapes** (Task 3 Step 2): `LocalClient()` arity and `PeerStatus.Tags`/`.Online`/`.DNSName` types vary by tailscale version — verify with `go doc` before building; discovery degrades gracefully if wrong, but it must compile.
- **Mixed serving ports:** discovery derives `http://<host>` (`:80`). Nodes serving on a non-`:80` tailnet port (or running combined-as-pane) won't be reachable via discovery — that is by design (manual add covers them); no code change, but don't "fix" the implied port.
- **Registry vs node route precedence:** `GET /api/nodes` and `/api/node/` (subtree) do not conflict in Go 1.22 `ServeMux` (different path after `/api/`); if the executor sees a mux registration panic, it means a pattern was mistyped.
