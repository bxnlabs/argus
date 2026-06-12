# Node status snippet + toggleable node rail

**Date:** 2026-06-08
**Status:** Approved (pending spec review)

## Problem

The node rail is a permanent 56px vertical strip on the far left of the
desktop layout. It is always on screen even when the user only has one node and
never switches, and it gives no plain-language readout of *which* node is
currently active or whether that node is reachable. We want a compact status
snippet under the `argus` wordmark that names the active node and its connection
status, and that doubles as the toggle for the rail.

## Goals

- Show the active node's name and connection status as a short line under the
  `argus` wordmark.
- Make the node rail **hidden by default**, revealed by clicking the snippet.
- Keep the rail's existing internals (node tiles for 2+ nodes, `+` manage
  button) unchanged.
- Keep the snippet visible whenever the sidebar is expanded, regardless of
  whether the rail is open.

## Non-goals

- No change to how nodes are discovered, polled, or switched (`NodeContext` /
  `useNodes` are untouched).
- The snippet conveys **connection** status only (Online / Connecting / Offline),
  not session activity. Per-node `busy`/`attention` counts stay on the rail
  tiles.
- No persistence of rail open/closed state across reloads — it is session state,
  matching the existing `sidebarOpen`.

## Behavior

### Visibility model

Today `NodeRail` is rendered unconditionally. After this change it is gated on
**`sidebarOpen && railOpen`**:

- `railOpen` is a new boolean in `App.tsx`, sitting next to the existing
  `sidebarOpen` (`useState(false)`), default `false`.
- Collapsing the sidebar hides the rail without discarding `railOpen`, so
  re-expanding the sidebar restores the rail exactly as it was (React state
  retention).
- The rail still animates in at 56px and **pushes** the layout, exactly as the
  always-on rail does today.

### The snippet (`NodeStatus`)

A new small component rendered under the `argus` wordmark in both the desktop
and mobile view headers.

- **Renders only when the sidebar is expanded.** It lives alongside the
  wordmark, which is itself expanded-only. When the sidebar is collapsed to
  56px there is no snippet and no status dot — only the existing toggle button,
  which remains the sole sidebar collapse/expand control.
- **Layout:** a full-width clickable line reading `‹node-name› · ‹Status›`,
  e.g. `my-laptop · Online`.
  - Node name truncates with `max-w` + ellipsis; the **full name is always
    available via tooltip** (covers truncation).
  - Status word is colored by state:
    - **Online** — green
    - **Connecting…** — amber
    - **Offline** — muted gray (intentionally not red; mirrors the rail's
      "offline recedes rather than alarms" treatment).
  - `hover:bg-accent/50` + `cursor-pointer` for discoverability. No chevron or
    other affordance — the rail sliding in is the feedback.
- **Click:** toggles `railOpen`.
- **Empty state:** if `activeNode` is null (a brief window before the
  same-origin fallback node resolves), render nothing.

### Status derivation

From `useNodeContext().activeNode` (already enriched by `useNodes`):

- `online === true` → **Online**.
- `online === false` **and** the node's summary poll has settled (errored) →
  **Offline**.
- `online === false` **and** the first poll has not settled yet → **Connecting…**.

`useNodes` currently exposes `online` (last settled poll succeeded). To
distinguish Connecting from Offline, `NodeStatus` needs to know whether the
active node's poll has settled at least once. Implementation note: surface this
without leaking React Query internals — either (a) treat the very first
pre-settle render as Connecting via a flag derived in `useNodes`, or (b) keep it
simple and collapse Connecting into Offline if threading that state proves
noisy. Decide during implementation; Connecting is a nicety, Online/Offline are
the contract.

### Mobile

The mobile sidebar is a drawer with no collapsed (56px) state. The snippet
renders under the `argus` wordmark in the drawer header and toggles the
in-drawer rail. No dot-degradation path is needed.

## Affected code

- `web/src/App.tsx` — add `railOpen` state next to `sidebarOpen`; pass
  `railOpen` / `setRailOpen` through to the views.
- `web/src/components/views/types.ts` — add `railOpen` / `setRailOpen` to
  `ViewProps`.
- `web/src/components/views/DesktopView.tsx` — render `<NodeStatus>` under the
  wordmark; gate `<NodeRail>` on `sidebarOpen && railOpen`.
- `web/src/components/views/MobileView.tsx` — render `<NodeStatus>` under the
  wordmark; gate the in-drawer `<NodeRail>` on `railOpen`.
- `web/src/components/NodeStatus/` — new component (+ test).
- `web/src/components/NodeRail/index.tsx` — no internal change beyond losing its
  always-visible assumption; callers now control visibility.

## Testing

- **`NodeStatus` unit tests:** renders name + correctly colored status word for
  Online / Offline / (Connecting if implemented); truncates a long name and
  exposes the full name via tooltip; clicking toggles `railOpen`; renders
  nothing when `activeNode` is null.
- **`NodeRail` tests:** update for gated visibility (hidden unless
  `sidebarOpen && railOpen`).
- **Multi-node e2e:** open the rail via the snippet before switching nodes.

## Open questions

None blocking. The only deferred decision is whether to render a distinct
**Connecting…** state or collapse it into Offline; resolved during
implementation per the Status derivation note above.
