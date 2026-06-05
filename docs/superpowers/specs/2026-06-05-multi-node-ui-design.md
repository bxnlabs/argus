# Multi-node UI — connect to multiple Argus instances (BXN-106)

## Overview

Argus was split into a server/node architecture so that multiple nodes can run
on different machines/containers while a single web client acts as one pane of
glass across all of them. Today that potential is unrealized: the web client
talks to exactly **one** node (`getNodeBaseUrl()` returns same-origin), the
`server` command only serves the SPA, and there is no registry of nodes.

This work makes the UI multi-node: an instance tracks a set of peer nodes
(added manually or discovered via Tailscale tags), surfaces them in a
**workspace rail** with at-a-glance per-node attention/activity, and lets the
user switch between them. The web client remains the only thing that talks to
nodes — the instance serving the UI never proxies node traffic, and nodes never
talk back to it.

While here, we take a deliberate **architectural simplification** that the
multi-node goal exposes as worthwhile: collapse the `server`/`node`/`combined`
modes into a single unified instance type, on a single port, with a single
unified `/api` namespace and a single SQLite database. This removes the
mode-dependent port/hostname plumbing that would otherwise make discovery
ambiguous, and leaves one coherent thing — "an Argus instance" — that is both a
standalone deployment *and* a peer in a multi-node fleet.

## Goals

- The web client can connect to and switch between multiple Argus nodes.
- A node's **need for attention is visible at a glance** — you can point at the
  specific node that has sessions waiting for you without opening a menu, even
  in the sidebar's default collapsed state.
- Nodes enter the registry two ways: **manual add** (name + URL) in the UI, and
  **Tailscale tag discovery**.
- Per-node unread/attention **bubbles up** into the node list, kept live by
  background polling; unreachable nodes show an **offline** state rather than
  erroring.
- Reuse Argus's existing attention vocabulary (blue = needs you, green pulse =
  busy, grey = idle, red = dead/offline) lifted from the session level to the
  node level.
- Simplify the deployment + API surface: one instance type, one port, one
  `/api` namespace, one database.

## Non-Goals (YAGNI)

- **No server↔node operational traffic / proxying.** The instance serving the
  UI tracks peers; the browser connects to each node directly.
- **No app-level auth tokens.** The trust boundary stays the network (loopback /
  tailnet), matching today's model. Cross-origin node access is enabled via
  permissive CORS.
- **No per-node tab persistence.** Switching nodes resets the workspace to the
  target node's session list; open tabs are node-scoped and torn down on switch.
- **No dedicated full "Nodes" management view.** Add/rename/remove lives in a
  small dialog off the rail; a richer view can layer on later.
- **No per-node serving-port discovery.** Every instance serves on the
  well-known tailnet port; mixed/custom ports use manual add.
- **No backward compatibility for the old split commands or `node.port`
  config** (see Breaking Changes).

## Decisions (resolved during brainstorming)

- **Registration:** two sources — **manual UI add** and **Tailscale tag
  discovery**. Identity is the **normalized base URL**; dedup across sources with
  precedence `local > manual > discovered`.
- **Topology / instance model:** **one unified instance type.** Every `argus`
  serves SPA + node API + registry. The `local` node is **always present** (a
  pure pane-of-glass is just an instance with no sessions). The `server` and
  `node` subcommands are **removed**.
- **Ports:** single port. **Tailnet binding is fixed at `:80`** (not
  configurable — tsnet listeners are userspace on the instance's own tailnet IP,
  so no privilege/collision concern). Loopback uses one configurable `port`
  (default 3000). `node.port` and the mode→port/hostname plumbing are removed.
- **API namespace:** unified under `/api`. **`/api/node/*`** = this instance's
  node API; **`/api/*`** = registry/discovery (e.g. `/api/nodes`). Replaces the
  old `/node/api/*`.
- **Persistence:** **one SQLite DB** per instance (the existing node DB at
  `shared.DBPath()`). The registry is a new `nodes` table added via the existing
  schema + migration framework. No separate `server.db`.
- **Unread/liveness:** **background-poll every node** via a lightweight
  `/api/node/summary` endpoint; unreachable → offline.
- **Switcher UX:** **Direction B — workspace rail** (Slack/VS-Code activity-bar
  style), always-visible, one tile per node, reusing the session attention
  colors. Lightweight add/manage dialog; **no** separate Nodes view.

## Breaking Changes

- `argus server` and `argus node` subcommands are **removed**. `argus` (the
  unified instance) is the only run mode.
- Config: `[node] port` is removed; `[server] port` collapses to a single
  top-level `port` (default 3000). Existing configs using `[server] port` /
  `[node] port` must migrate to `port`. (Pre-1.0; clean break, no shim.)
- API paths change `/node/api/*` → `/api/node/*` (and `/node/ws/*` →
  `/api/node/ws/*`). Internal/coordinated; no external consumers besides the SPA
  and CLI, both updated in this change.

## Architecture

### Instance model (the simplification)

`makeListeners` loses its `mode` parameter and the `server`/`node` branching;
`deriveHostname` loses its mode suffix. Every instance:

- Binds **one loopback listener** on the configured `port` (default 3000),
  serving the full mux: SPA at `/`, node API at `/api/node/*`, registry at
  `/api/*`.
- When Tailscale is enabled, binds **one tsnet listener on `:80`** serving the
  same mux. (`tailscale.port`, the WireGuard UDP port, is unchanged and
  unrelated.)

A discovered or manually-added peer is therefore always reachable at
`http://<host>` (tailnet, `:80` implicit) or `http://<host>:<port>` (explicit),
with its node API under `/api/node/*` — uniform across every instance, so the
client's per-node base URL is "just the origin."

### Server: node registry

A small root-mux registry, backed by the single SQLite DB:

- **DB:** new `nodes(id, name, url, created_at)` table, added to `schema.go`
  (fresh DBs) and as an `add_nodes_table` entry in `allMigrations` (existing
  DBs). Stores **manual** nodes only; `local` and `discovered` entries are
  computed per request.
- **Ownership/wiring:** DB ownership hoists up one level — a bootstrap opens the
  single `*db.DB` and hands it to both the node API handler and the registry
  handler. Registry CRUD lives in `internal/node/db/nodes.go`; the root-mux
  `/api/nodes` handler is a thin server-scoped package. (Renaming `node/db` to
  an "instance DB" package is deferred as out-of-scope churn.)
- **Endpoints:**
  - `GET /api/nodes` → merged, deduped list `[{id, name, url, source, self}]`
    where `source ∈ {local, manual, discovered}`.
  - `POST /api/nodes {name, url}` → add manual (validates URL shape; does **not**
    contact the node).
  - `PATCH /api/nodes/{id} {name}` → rename manual.
  - `DELETE /api/nodes/{id}` → remove manual.

### Tailscale discovery

- New method on the tsnet wrapper: `Peers(ctx)` over `LocalClient().Status()`,
  returning peers whose `Tags` include the configured discovery tag. Excludes
  self.
- New config: `tailscale.discovery_tag` (default `tag:argus-node`).
- Each discovered node: `name` = DNS-name label, `url` = `http://<DNSName>`
  (`:80` implicit). Deterministic — no port logic.
- Best-effort: if Tailscale is disabled or `Status()` errors, the list is just
  `local` + `manual`.

### Node: summary endpoint + CORS

- **`GET /api/node/summary`** → `{name, attention, busy, total}`. `attention` =
  sessions with `unread_since` or `user_marked_unread_at` and not active; `busy`
  = active/running count. Reuses existing DB/status logic; cheap, no session
  payloads.
- **CORS:** the node API gains middleware reflecting the request `Origin`, and
  the WS upgrader accepts cross-origin. No new auth — trust stays the network
  boundary. (Surfaced explicitly as it is security-adjacent.)

### Client: node routing

- `getNodeBaseUrl()` / `getNodeWsUrl()` become **selected-node aware**: they
  read the active node's origin and prepend it to the existing `/api/node/...`
  paths. The `local`/self node → `""` (same-origin, unchanged behavior).
- Path migration: every `/node/api/*` → `/api/node/*`, `/node/ws/*` →
  `/api/node/ws/*` (47 call sites across 15 files; mechanical). Vite dev proxy
  rules updated (`/node/api`, `/node/ws` → `/api/node`).
- **Selection** persisted in `localStorage`; defaults to the `local` node, else
  the first reachable.
- **Switching semantics:** react-query caches are **node-scoped** (keyed by node
  id); switching tears down open tabs + terminal WS and opens the target node
  fresh. Per-node tab persistence is out of scope.
- New **`useNodes()`** hook: fetches `/api/nodes`, background-polls each node's
  `/api/node/summary` (~5s), tracks reachable/offline per node, aggregates into
  rail badges.

## UI: workspace rail (Direction B)

- New `NodeRail` column (`w-12`) left of the sidebar in `DesktopView`, **rendered
  only when ≥2 nodes exist** — solo instances see today's UI untouched.
- Per tile (reusing existing session attention colors):
  - **Selected** → blue `bg-primary` left pill (mirrors the active-session pill)
    + brighter tile.
  - **Attention** → blue count badge, top-right corner (sessions waiting on you).
  - **Busy** → green `animate-pulse-green` dot (≥1 active session).
  - **Idle** → grey; **Offline** → desaturated tile + dim-red ring, badges
    cleared.
- **Management:** a `+`/gear tile at the bottom opens a small **Add/Manage
  Nodes** dialog (add by name+URL; rename/remove manual nodes; discovered nodes
  shown read-only). Per-tile right-click context menu for rename/remove.
- **Mobile:** `MobileView` folds the rail into a horizontal node strip at the top
  of the session drawer — no permanent column.

## Data flow

`useNodes` → `GET /api/nodes` (registry; local + manual + discovered, deduped) →
for each node, poll `GET {origin}/api/node/summary` → aggregate into rail
badges. The active node additionally drives the full existing
session/status/terminal flow against its origin.

## Error / offline handling

- A node whose summary poll fails → `offline` (desaturated tile, red ring,
  badges cleared). Polling continues with backoff so it recovers automatically.
- If the **active** node goes offline, the workspace shows an inline "node
  unreachable — retrying" state instead of erroring out; the user can switch to
  another tile.

## Testing

- **Go:**
  - Registry CRUD + SQLite persistence; the `add_nodes_table` migration applies
    to an existing DB and `CheckMigrations` stays green.
  - Dedup/precedence across `local`/`manual`/`discovered`.
  - Discovery filtering against mocked tailnet peers (tag match, self-exclusion,
    URL derivation).
  - `/api/node/summary` counts (attention vs busy vs total).
  - CORS headers present on node API responses and WS upgrade.
  - Single-port/unified listener wiring; removed-subcommand surface.
- **Web (vitest):**
  - `useNodes` aggregation + offline transitions + backoff.
  - Rail tile rendering for each state (selected/attention/busy/idle/offline);
    rail hidden when <2 nodes.
  - Switching resets node-scoped query caches and re-targets API/WS at the new
    origin.
- Existing tmux/session tests stay isolated per the repo's tmux test-safety
  rule (no shared default socket).

## Open risks

- **tsnet peer enumeration:** `LocalClient().Status()` exposing peer `Tags` +
  `DNSName` is the one capability not already wrapped; small spike to confirm the
  field shape before building discovery on it. Discovery is best-effort, so a
  gap degrades gracefully to manual-only.
- **Cross-origin WS:** terminal WS to a remote node must pass the node's
  Origin/upgrade checks; verify the upgrader accepts the pane's origin under the
  permissive-CORS model.
