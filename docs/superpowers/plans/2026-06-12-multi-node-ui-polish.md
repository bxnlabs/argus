# Multi-node UI Polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a glassy primary-blue gradient that visually separates the node rail from the sidebar, plus chord shortcuts to toggle the sidebar (`Cmd/Ctrl+; b`) and switch nodes by number (`Cmd/Ctrl+; 1–9`).

**Architecture:** Two of the three changes are entries in the existing leader-key chord map in `web/src/App.tsx`. The node-switch digit logic is extracted into a small pure helper (`web/src/lib/nodeShortcuts.ts`) so it can be unit-tested in isolation; the `b` toggle is a one-line binding. The gradient is a dedicated `.node-rail-glass` CSS class in `web/src/globals.css`, applied to the rail container — matching the repo's precedent of housing rail/node visual treatments (`.node-working`) in that file.

**Tech Stack:** React 19 + TypeScript, Vite, Tailwind CSS v4 (dark theme via HSL CSS variables in `globals.css`), Vitest + Testing Library.

---

## Background / orientation

Read these before starting:

- `web/src/hooks/useKeyboardChords.ts` — the chord engine. Types: `ChordBinding` (`{ label: string; run?: () => void; children?: Record<string, ChordBinding> }`) and `ChordMap = Record<string, ChordBinding>`. The leader is `Cmd+;` (macOS) / `Ctrl+;` (else); the next unmodified key picks a binding. The engine is already fully tested (`useKeyboardChords.test.ts`) — we are only adding bindings, not touching the engine.
- `web/src/App.tsx:219–282` — the `bindings: ChordMap` `useMemo` inside `HomeContent`. Existing keys: `n`, `=`, `-`, `ArrowLeft`, `ArrowRight`, `t`, conditional `d`/`i`/`g`/`e`, and `?`. The `?` help overlay is generated from this same map, so any binding added here appears automatically.
- `web/src/App.tsx:40` — `HomeContent` already calls `const { activeNode } = useNodeContext();`. We extend this destructure to also pull `nodes` and `setActiveNode`.
- `web/src/App.tsx:42` — `const [sidebarOpen, setSidebarOpen] = useState(false);` lives in the same scope as the chord map.
- `web/src/components/views/DesktopView.tsx:52` — `{sidebarOpen && railOpen && <NodeRail />}`; line 82 — the existing toggle button does `setSidebarOpen(!sidebarOpen)` (collapse to `w-14` icon strip ⇄ expand to `w-72`; never fully hidden).
- `web/src/components/NodeRail/index.tsx:128–135` — the rail container `<div>`. Currently `className={cn("bg-sidebar-background flex h-full w-14 flex-shrink-0 flex-col items-stretch gap-3 py-3", side === "right" ? "border-l" : "border-r")}`. `side` defaults to `"left"`; the mobile drawer passes `"right"`.
- `web/src/globals.css:69` — `--primary: 217.2 91.2% 59.8%;` (HSL channels). `--sidebar-background: 0 0% 7%;` (line 87). `.node-working` and its keyframes sit around lines 253–286 — add the new class right after them.
- `web/src/types.ts:202` — `NodeWithStatus extends NodeInfo`, which has `id: string` and `name: string`.

**Commands:**
- Typecheck: `cd web && npx tsc -b`
- Unit tests: `cd web && npm test` (or a single file: `cd web && npx vitest run src/lib/nodeShortcuts.test.ts`)

---

## File Structure

- **Create** `web/src/lib/nodeShortcuts.ts` — pure helper `buildNodeSwitchBindings(nodes, setActiveNode)` returning a `ChordMap` of digit bindings. One responsibility: derive the `1–9` node-switch chords. Lives in `lib/` alongside other pure, unit-tested helpers (`sidebarSections.ts`, etc.).
- **Create** `web/src/lib/nodeShortcuts.test.ts` — unit tests for the helper.
- **Modify** `web/src/App.tsx` — extend the `useNodeContext()` destructure; add the `b` binding and spread the digit bindings into the chord map; extend the `useMemo` deps.
- **Modify** `web/src/globals.css` — add `.node-rail-glass` (and its `[data-side="right"]` variant).
- **Modify** `web/src/components/NodeRail/index.tsx` — apply `node-rail-glass` and `data-side={side}` to the rail container.

---

## Task 1: Node-switch chord helper (`buildNodeSwitchBindings`)

**Files:**
- Create: `web/src/lib/nodeShortcuts.ts`
- Test: `web/src/lib/nodeShortcuts.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/lib/nodeShortcuts.test.ts`:

```ts
import { describe, it, expect, vi } from "vitest";
import type { NodeWithStatus } from "@/types";
import { buildNodeSwitchBindings, MAX_NODE_CHORDS } from "./nodeShortcuts";

/** Minimal NodeWithStatus factory — only id/name matter to the helper. */
function node(id: string, name: string): NodeWithStatus {
  return {
    id,
    name,
    url: "",
    source: "manual",
    self: false,
    summary: null,
    online: true,
    pending: false,
  };
}

describe("buildNodeSwitchBindings", () => {
  it("returns an empty map when there is nothing to switch between", () => {
    const setActiveNode = vi.fn();
    expect(buildNodeSwitchBindings([], setActiveNode)).toEqual({});
    expect(buildNodeSwitchBindings([node("a", "Alpha")], setActiveNode)).toEqual(
      {},
    );
  });

  it("maps nodes to digit keys in order, labelled by name", () => {
    const setActiveNode = vi.fn();
    const bindings = buildNodeSwitchBindings(
      [node("a", "Alpha"), node("b", "Bravo"), node("c", "Charlie")],
      setActiveNode,
    );

    expect(Object.keys(bindings)).toEqual(["1", "2", "3"]);
    expect(bindings["1"].label).toBe("Alpha");
    expect(bindings["2"].label).toBe("Bravo");

    bindings["3"].run!();
    expect(setActiveNode).toHaveBeenCalledTimes(1);
    expect(setActiveNode).toHaveBeenCalledWith("c");
  });

  it(`caps at the first ${MAX_NODE_CHORDS} nodes (rest are mouse-only)`, () => {
    const setActiveNode = vi.fn();
    const many = Array.from({ length: 11 }, (_, i) =>
      node(`id-${i}`, `Node ${i}`),
    );

    const bindings = buildNodeSwitchBindings(many, setActiveNode);

    expect(Object.keys(bindings)).toEqual([
      "1", "2", "3", "4", "5", "6", "7", "8", "9",
    ]);
    expect(bindings["10"]).toBeUndefined();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && npx vitest run src/lib/nodeShortcuts.test.ts`
Expected: FAIL — `Failed to resolve import "./nodeShortcuts"` (module doesn't exist yet).

- [ ] **Step 3: Write the helper**

Create `web/src/lib/nodeShortcuts.ts`:

```ts
import type { ChordMap } from "@/hooks/useKeyboardChords";
import type { NodeWithStatus } from "@/types";

/** Highest node reachable by a number chord; beyond this requires the mouse. */
export const MAX_NODE_CHORDS = 9;

/**
 * Build the `1`–`9` chord bindings that switch the active node, in rail order
 * (`1` = first tile). Returns an empty map when there is nothing to switch
 * between (a single node or none), so the chord/help overlay never advertises a
 * no-op. Nodes past the 9th get no binding — they are reachable only by
 * clicking the rail tile. Each binding mirrors a tile click: `setActiveNode(id)`.
 */
export function buildNodeSwitchBindings(
  nodes: NodeWithStatus[],
  setActiveNode: (id: string) => void,
): ChordMap {
  if (nodes.length < 2) return {};

  const bindings: ChordMap = {};
  nodes.slice(0, MAX_NODE_CHORDS).forEach((node, i) => {
    bindings[String(i + 1)] = {
      label: node.name,
      run: () => setActiveNode(node.id),
    };
  });
  return bindings;
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && npx vitest run src/lib/nodeShortcuts.test.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Typecheck**

Run: `cd web && npx tsc -b`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/nodeShortcuts.ts web/src/lib/nodeShortcuts.test.ts
git commit -m "feat(shortcuts): node-switch chord binding helper"
```

---

## Task 2: Wire `b` toggle + node-switch digits into the chord map

**Files:**
- Modify: `web/src/App.tsx` (import; `useNodeContext()` destructure at line 40; `bindings` `useMemo` at 219–282)

This task has no unit test — the bindings live inside `HomeContent`, whose render pulls in the full app shell, so a render-based assertion would be disproportionate. The digit logic is already covered by Task 1's helper tests; here we verify by typecheck and the manual smoke test in Step 5.

- [ ] **Step 1: Import the helper**

In `web/src/App.tsx`, add to the imports near the top (next to the other `@/lib` / `@/hooks` imports):

```ts
import { buildNodeSwitchBindings } from "@/lib/nodeShortcuts";
```

- [ ] **Step 2: Pull `nodes` and `setActiveNode` from the node context**

In `web/src/App.tsx`, change the destructure at line 40 from:

```ts
  const { activeNode } = useNodeContext();
```

to:

```ts
  const { activeNode, nodes, setActiveNode } = useNodeContext();
```

- [ ] **Step 3: Add the `b` binding and spread the digit bindings**

In the `bindings` `useMemo` return object (`web/src/App.tsx:225–281`), add the `b` binding immediately after the `t` (Terminal) line, and spread the node-switch digits immediately before the `"?"` line.

After the `t:` line (currently line 231), add:

```ts
      b: { label: "Toggle sidebar", run: () => setSidebarOpen((v) => !v) },
```

Immediately before the `"?": { label: "Show all shortcuts", ... }` line (currently line 280), add:

```ts
      ...buildNodeSwitchBindings(nodes, setActiveNode),
```

- [ ] **Step 4: Extend the `useMemo` dependency array**

The `bindings` `useMemo` dependency array (currently ending `web/src/App.tsx:282`) must include the new reactive inputs. `setSidebarOpen` and `setActiveNode` are stable, but the bindings rebuild when `nodes` changes (labels/ids), so add `nodes` and `setActiveNode`. Change:

```ts
  }, [tabs, activeTabId, isGitRepo, activeWorkingDirectory, addTab, closeTab, switchTab, requestGitTab, activeTab, detachSession]);
```

to:

```ts
  }, [tabs, activeTabId, isGitRepo, activeWorkingDirectory, addTab, closeTab, switchTab, requestGitTab, activeTab, detachSession, nodes, setActiveNode]);
```

- [ ] **Step 5: Typecheck**

Run: `cd web && npx tsc -b`
Expected: no errors. (If `tsc` reports `setActiveNode` is declared but unused, re-check Step 3 added the spread.)

- [ ] **Step 6: Manual smoke test**

Run: `cd web && npm run dev`, open the app on desktop (window wider than the mobile breakpoint), and verify:
- Press the leader (`Cmd+;` on macOS / `Ctrl+;` else), then `b` → the sidebar collapses; repeat → it expands.
- Press `Cmd/Ctrl+;` then `?` → the help overlay lists "Toggle sidebar" under `b`, and (with 2+ nodes) the digit keys mapped to node names.
- With 2+ nodes: `Cmd/Ctrl+;` then `1`/`2`/… switches the active node, matching rail order.
- With a single node: the overlay shows no digit entries.

- [ ] **Step 7: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat(shortcuts): Cmd/Ctrl+; b toggles sidebar, 1-9 switch nodes"
```

---

## Task 3: Glassy gradient on the node rail

**Files:**
- Modify: `web/src/globals.css` (add `.node-rail-glass` after the `.node-working` block, ~line 286)
- Modify: `web/src/components/NodeRail/index.tsx:128–135` (rail container)

This is a presentational change with no meaningful unit test (CSS gradients aren't unit-testable); verification is typecheck + visual inspection.

- [ ] **Step 1: Add the `.node-rail-glass` class**

In `web/src/globals.css`, immediately after the `.node-working::before { … }` block (which ends ~line 286, before the `/* Toast prominence */` comment), add:

```css
/* Node rail: accent-tinted glass so the rail reads as a surface distinct from
   the adjacent sidebar (both otherwise share --sidebar-background). Layers, top
   to bottom in the shorthand: an inner-edge sheen facing the sidebar divider, a
   primary-blue vertical tint (lighter top -> darker bottom), then the sidebar
   base. Primary blue channels are 217.2 91.2% 59.8% (see --primary). This class
   is unlayered, so it overrides the Tailwind `bg-sidebar-background` utility,
   same as .node-working overrides its surroundings. */
.node-rail-glass {
  background:
    linear-gradient(to left, hsl(0 0% 100% / 0.06), transparent 16px),
    linear-gradient(
      to bottom,
      hsl(217.2 91.2% 59.8% / 0.12),
      hsl(217.2 91.2% 59.8% / 0.03) 45%,
      hsl(0 0% 0% / 0.12)
    ),
    hsl(var(--sidebar-background));
}

/* Mobile drawer variant: the rail sits on the right, so mirror the sheen to the
   left edge, which now faces the sidebar divider. */
.node-rail-glass[data-side="right"] {
  background:
    linear-gradient(to right, hsl(0 0% 100% / 0.06), transparent 16px),
    linear-gradient(
      to bottom,
      hsl(217.2 91.2% 59.8% / 0.12),
      hsl(217.2 91.2% 59.8% / 0.03) 45%,
      hsl(0 0% 0% / 0.12)
    ),
    hsl(var(--sidebar-background));
}
```

- [ ] **Step 2: Apply the class and `data-side` to the rail container**

In `web/src/components/NodeRail/index.tsx`, the container `<div>` (lines 128–135). Add `node-rail-glass` as the first class in the `cn(...)` string and add a `data-side={side}` attribute. Change:

```tsx
      <div
        id="node-rail"
        data-testid="node-rail"
        className={cn(
          "bg-sidebar-background flex h-full w-14 flex-shrink-0 flex-col items-stretch gap-3 py-3",
          side === "right" ? "border-l" : "border-r",
        )}
      >
```

to:

```tsx
      <div
        id="node-rail"
        data-testid="node-rail"
        data-side={side}
        className={cn(
          "node-rail-glass bg-sidebar-background flex h-full w-14 flex-shrink-0 flex-col items-stretch gap-3 py-3",
          side === "right" ? "border-l" : "border-r",
        )}
      >
```

(`bg-sidebar-background` is kept as a harmless fallback; the unlayered `.node-rail-glass` wins.)

- [ ] **Step 3: Typecheck**

Run: `cd web && npx tsc -b`
Expected: no errors.

- [ ] **Step 4: Visual verification**

Run: `cd web && npm run dev` and verify on desktop with **2+ nodes** and the sidebar expanded:
- The node rail is now visibly distinct from the sidebar — a faint blue-tinted glassy panel with a subtle vertical gradient and a soft highlight along its inner (sidebar-facing) edge, rather than blending into one dark block.
- The single-node state (rail collapsed to just the "Add node" button) still shows the gradient surface and reads cleanly.
- The tint is subtle (a sheen, not a solid blue fill); node tiles remain legible.

- [ ] **Step 5: Commit**

```bash
git add web/src/globals.css web/src/components/NodeRail/index.tsx
git commit -m "feat(node-rail): glassy primary-tinted gradient to separate from sidebar"
```

---

## Self-review notes

- **Spec coverage:** Feature 1 (gradient) → Task 3; Feature 2 (`b` toggle) → Task 2 Step 3; Feature 3 (`1–9` switch) → Task 1 + Task 2. The `side="right"` mirroring requirement → Task 3 Step 1 (`[data-side="right"]` variant) + Step 2 (`data-side`). The "auto-appears in `?` overlay" property is inherent (same chord map) and checked in Task 2 Step 6.
- **Type consistency:** `buildNodeSwitchBindings(nodes, setActiveNode)` and `MAX_NODE_CHORDS` are named identically in the helper, its test, and the App wiring. `ChordMap`/`NodeWithStatus` are imported from their real sources (`@/hooks/useKeyboardChords`, `@/types`).
- **No placeholders:** every code step shows complete code; every run step has an expected result.
