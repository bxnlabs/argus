# Multi-node UI polish — Design

**Date:** 2026-06-12
**Status:** Approved for planning

## Summary

Three small, presentational improvements to the multi-node UI:

1. A glassy, primary-blue-tinted gradient on the node rail so it visually separates from the adjacent sidebar.
2. A keyboard shortcut `Cmd/Ctrl+; b` to toggle the sidebar.
3. Keyboard shortcuts `Cmd/Ctrl+; 1–9` to switch to a node by number.

The changes live in `web/src/globals.css` and
`web/src/components/NodeRail/index.tsx` for the gradient, and `web/src/App.tsx`
(the chord map) for both shortcuts. During implementation the node-switch digit
logic was extracted into a small pure helper (`web/src/lib/nodeShortcuts.ts`)
with unit tests (`web/src/lib/nodeShortcuts.test.ts`) so it can be tested in
isolation; see the implementation plan for the final file breakdown.

## Background

The node rail (`web/src/components/NodeRail/index.tsx`) is a `w-14` vertical
strip that renders to the left of the sidebar when the sidebar is expanded,
the rail is open, and there are 2+ nodes (`{sidebarOpen && railOpen && <NodeRail />}`
in `web/src/components/views/DesktopView.tsx:52`). Both the rail and the sidebar
use `bg-sidebar-background`, so when shown together they read as one
undivided dark block: `[NodeRail w-14][Sidebar w-72]`.

Keyboard shortcuts use a leader-key chord system (`web/src/hooks/useKeyboardChords.ts`):
the user presses `Cmd+;` (macOS) / `Ctrl+;` (else), then an unmodified key. The
chord map is defined as a `ChordMap` in a `useMemo` in `HomeContent` at
`web/src/App.tsx:219–282`. The hook is gated on `!isMobile`
(`web/src/App.tsx:285`). The `?` help overlay is generated from the same chord
map, so any binding added there appears in the overlay automatically.

`HomeContent` already calls `useNodeContext()` (`web/src/App.tsx:40`) and holds
`sidebarOpen`/`setSidebarOpen` (`web/src/App.tsx:42`), so the chord map can reach
the node list, the node switcher, and the sidebar toggle without new wiring.

## Feature 1 — Glassy gradient on the node rail

### Goal

Visually separate the node rail from the adjacent sidebar so the two
`bg-sidebar-background` surfaces stop reading as a single block. Style:
accent-tinted glass using the app's primary blue.

### Approach

Add a dedicated `.node-rail-glass` CSS class to the global stylesheet (the file
that already houses `.node-working` and the pulse keyframes) rather than inline
Tailwind arbitrary-value utilities. A multi-layer glass effect is far more
readable as a named class, and rail/node-specific visual treatments already live
there, so this matches repo precedent.

The class layers three things over the rail container:

1. **Base gradient** — a vertical (top → bottom) gradient that blends a faint
   primary-blue tint over the sidebar base color, lighter at the top and
   settling darker toward the bottom. This is a subtle tint, not a color fill.
2. **Glass sheen** — a soft light highlight along the rail's *inner* edge (the
   side facing the sidebar divider), giving the "pane of glass" read.
3. **Divider reinforcement** — keep the existing `border-r`, but let the
   gradient and sheen carry most of the separation so it feels like depth rather
   than just a line.

The class is applied to the rail container at
`web/src/components/NodeRail/index.tsx:131–134`, alongside the existing layout
utilities (`flex h-full w-14 ...`). The class supplies the gradient/tint; the
existing utility classes keep the layout. `bg-sidebar-background` may remain as
the fallback base the gradient sits on.

The rail's `side="right"` case mirrors the inner-edge sheen to the opposite edge
so the glass treatment stays oriented toward the divider. This variant is
supported on `NodeRail` but not exercised by any current caller — the only
`<NodeRail />` usage (`DesktopView.tsx`) takes the default `"left"`, and the
mobile path renders a separate `MobileNodePanel`, not `NodeRail`.

### Non-goals

- No change to node tiles or their colors.
- No `backdrop-blur` / literal transparency — nothing meaningful sits behind the
  rail (it's at the window's left edge), so true glassmorphism would reveal only
  the OS desktop / window chrome.
- No new theme tokens — the app has only a dark theme.

## Feature 2 — `Cmd/Ctrl+; b` toggles the sidebar

### Goal

A keyboard shortcut to collapse/expand the sidebar.

### Approach

Add one binding to the chord map at `web/src/App.tsx:225`:

```ts
b: { label: "Toggle sidebar", run: () => setSidebarOpen((v) => !v) },
```

- Uses the established leader-chord system (`Cmd/Ctrl+;` then `b`). The `b` key
  is currently unused.
- Mirrors the existing toggle button (`web/src/components/views/DesktopView.tsx:82`,
  `setSidebarOpen(!sidebarOpen)`): collapse to a `w-14` icon strip ⇄ expand to
  `w-72`. The sidebar is never fully hidden on desktop.
- Appears in the `?` help overlay automatically (generated from the same chord
  map).
- Desktop-only, like all chords (`useKeyboardChords` is gated on `!isMobile`),
  so there are no mobile-drawer edge cases.
- `setSidebarOpen` is a stable `useState` setter, so the functional update needs
  no new `useMemo` dependency.

### Expected side effect (not a bug)

Collapsing the sidebar also hides the node rail, since the rail is gated on
`sidebarOpen` (`DesktopView.tsx:52`). This is the existing toggle button's
behavior as well.

## Feature 3 — `Cmd/Ctrl+; 1–9` switches nodes

### Goal

Jump directly to a node by number. Nodes beyond the 9th stay mouse-only.

### Approach

Conditionally register digit bindings in the same chord map, only when there are
2+ nodes (a single node has nothing to switch to), capped at the first 9 in
**rail order** so the number matches the tile's visual position:

```ts
...(nodes.length >= 2
  ? Object.fromEntries(
      nodes.slice(0, 9).map((n, i) => [
        String(i + 1),
        { label: n.name, run: () => setActiveNode(n.id) },
      ]),
    )
  : {}),
```

- `1` = first rail tile, `2` = second, etc. — same order the rail renders
  (`nodes.map(...)` in `NodeRail/index.tsx`).
- Nodes past the 9th get no shortcut (mouse only); they are simply never
  registered, so the `?` overlay never advertises a key that does nothing.
- Each binding's label is the node's name, so the hint overlay reads
  `1 → <node>`, `2 → <node>`, etc. automatically.
- Mirrors a tile click exactly (`setActiveNode(n.id)`), including the same
  behavior for offline nodes that clicking has today.
- Adds `nodes` and `setActiveNode` to the `bindings` `useMemo` dependency array.
- No collisions: digits don't overlap any existing chord key (`n`, `=`, `-`,
  arrows, `t`, `d`, `i`, `g`, `e`, `b`, `?`).

## Testing

These changes are presentational plus chord-map entries. Verification is
primarily by running the app:

- **Gradient:** confirm the rail visibly separates from the sidebar, in both the
  single-node state (rail collapsed to the add-node button) and the 2+-node
  state (tiles visible). Confirm the glass tint reads as intended against the
  sidebar. (The `side="right"` mirror variant has no current caller, so there is
  nothing to verify there yet.)
- **`b` chord:** `Cmd/Ctrl+;` then `b` collapses/expands the sidebar; the binding
  shows in the `?` overlay.
- **`1–9` chords:** with 2+ nodes, the digits switch the active node and match
  rail order; with a single node, no digit bindings are registered; the `?`
  overlay lists the digit→node mappings.

The node-switch digit logic is extracted into `web/src/lib/nodeShortcuts.ts` and
unit-tested in `web/src/lib/nodeShortcuts.test.ts` (empty map below 2 nodes,
rail-order digit mapping labelled by name, cap at the first 9). The `b` toggle
and the gradient remain presentational/wiring changes verified by running the
app.

## Files touched

- `web/src/lib/nodeShortcuts.ts` — new pure helper deriving the `1–9`
  node-switch chord bindings.
- `web/src/lib/nodeShortcuts.test.ts` — unit tests for the helper.
- `web/src/globals.css` — add `.node-rail-glass` (and its `[data-side="right"]`
  variant).
- `web/src/components/NodeRail/index.tsx` — apply `.node-rail-glass` and
  `data-side={side}` to the rail container.
- `web/src/App.tsx` — add the `b` binding and spread the node-switch digit
  bindings into the chord map; extend the `useMemo` dependency array with `nodes`
  and `setActiveNode`.
