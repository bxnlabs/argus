# Node Status Snippet + Toggleable Node Rail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a compact `node-name · Status` snippet under the `argus` wordmark that reports the active node's connection status and toggles a now-hidden-by-default node rail.

**Architecture:** A new `pending` flag on `NodeWithStatus` (derived from the summary query's first-settle state) lets a small presentational `NodeStatus` component render Online / Connecting… / Offline. The node rail stops rendering unconditionally; the views gate it on a new `railOpen` boolean (alongside the existing `sidebarOpen`) that the snippet toggles.

**Tech Stack:** React + TypeScript, TanStack Query, Tailwind, Vitest + @testing-library/react. Run tests from `web/` with `pnpm test`.

---

## File Structure

- `web/src/types.ts` — add `pending: boolean` to `NodeWithStatus`.
- `web/src/hooks/useNodes.ts` — derive and return `pending` per node.
- `web/src/hooks/useNodes.test.tsx` — assert `pending` for settled vs. in-flight polls.
- `web/src/components/NodeStatus/index.tsx` — **new** snippet component.
- `web/src/components/NodeStatus/index.test.tsx` — **new** unit tests.
- `web/src/components/NodeRail/index.test.tsx` — add `pending` to the test fixture.
- `web/src/components/views/types.ts` — add `railOpen` / `setRailOpen` to `ViewProps`.
- `web/src/App.tsx` — add `railOpen` state; thread it into `viewProps`.
- `web/src/components/views/DesktopView.tsx` — render `NodeStatus`; gate `NodeRail`.
- `web/src/components/views/MobileView.tsx` — render `NodeStatus`; gate `NodeRail`.

All paths below are relative to the repo root; the `web/` app is where `pnpm test` runs.

---

## Task 1: Add `pending` to the node status model

**Files:**
- Modify: `web/src/types.ts:202-205`
- Modify: `web/src/hooks/useNodes.ts:27-37`
- Test: `web/src/hooks/useNodes.test.tsx`

`pending` means the node's summary poll has not settled yet (no success, no error) — i.e. the very first request is still in flight. Because the query uses `retry: false`, `isPending` is true *only* before the first settle and never again on background refetches, so it cleanly distinguishes "Connecting…" (in flight) from "Offline" (settled error).

- [ ] **Step 1: Add the failing assertions to the existing useNodes tests**

In `web/src/hooks/useNodes.test.tsx`, in the first test (`"aggregates summaries and marks unreachable nodes offline"`), after the existing `expect(gpu.summary).toBeNull();` (line 43), add:

```tsx
    // A settled (errored) poll is Offline, not Connecting.
    expect(local.pending).toBe(false);
    expect(gpu.pending).toBe(false);
```

In the second test (`"keeps a node offline while its poll is in flight (no online flash)"`), after the existing `expect(gpu.summary).toBeNull();` (line 78), add:

```tsx
    // Its first poll has never settled, so it reads as Connecting, not Offline.
    expect(gpu.pending).toBe(true);
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd web && pnpm test -- src/hooks/useNodes.test.tsx`
Expected: FAIL — `pending` is `undefined` (TypeScript may also error that `pending` is not on `NodeWithStatus`).

- [ ] **Step 3: Add `pending` to the type**

In `web/src/types.ts`, change the `NodeWithStatus` interface (lines 202-205) to:

```ts
export interface NodeWithStatus extends NodeInfo {
  summary: NodeSummary | null;
  online: boolean;
  pending: boolean;
}
```

- [ ] **Step 4: Derive `pending` in the hook**

In `web/src/hooks/useNodes.ts`, change the node-mapping body (lines 35-36) from:

```ts
    const online = !!q && q.isSuccess;
    return { ...n, summary: online && q?.data ? q.data : null, online };
```

to:

```ts
    const online = !!q && q.isSuccess;
    // Pending = the first poll is still in flight (never settled). retry:false
    // means isPending flips false the moment a poll settles and stays false on
    // background refetches, so it marks "Connecting…" without flashing.
    const pending = !!q && q.isPending;
    return { ...n, summary: online && q?.data ? q.data : null, online, pending };
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd web && pnpm test -- src/hooks/useNodes.test.tsx`
Expected: PASS (both tests).

- [ ] **Step 6: Fix the NodeRail test fixture (type now requires `pending`)**

In `web/src/components/NodeRail/index.test.tsx`, line 27, change the `base` fixture to include `pending`:

```ts
const base: NodeWithStatus = { id: "x", name: "x", url: "", source: "manual", self: false, summary: null, online: true, pending: false };
```

- [ ] **Step 7: Run the NodeRail tests to verify they still pass**

Run: `cd web && pnpm test -- src/components/NodeRail/index.test.tsx`
Expected: PASS (3 tests).

- [ ] **Step 8: Commit**

```bash
git add web/src/types.ts web/src/hooks/useNodes.ts web/src/hooks/useNodes.test.tsx web/src/components/NodeRail/index.test.tsx
git commit -m "feat(multi-node): derive pending (connecting) state for nodes"
```

---

## Task 2: Build the `NodeStatus` snippet component

**Files:**
- Create: `web/src/components/NodeStatus/index.tsx`
- Test: `web/src/components/NodeStatus/index.test.tsx`

The component reads the active node from `useNodeContext()` and renders `‹name› · ‹Status›`. The name truncates; the status word is colored (Online green, Connecting amber, Offline muted gray). Clicking it calls the `onToggleRail` prop. When there is no active node it renders nothing. A tooltip shows the full `‹name› · ‹Status›` (covers a truncated name).

- [ ] **Step 1: Write the failing test**

Create `web/src/components/NodeStatus/index.test.tsx`:

```tsx
import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";

afterEach(() => { cleanup(); });
import { NodeContext } from "@/contexts/NodeContext";
import { TooltipProvider } from "@/components/ui/tooltip";
import { NodeStatus } from "./index";
import type { NodeWithStatus } from "@/types";

const base: NodeWithStatus = {
  id: "local", name: "my-laptop", url: "", source: "local",
  self: true, summary: null, online: true, pending: false,
};

function renderStatus(active: NodeWithStatus | null, onToggleRail = vi.fn()) {
  render(
    <TooltipProvider>
      <NodeContext.Provider
        value={{
          nodes: active ? [active] : [],
          isLoaded: true,
          activeNodeId: active?.id ?? null,
          activeNode: active,
          setActiveNode: vi.fn(),
        }}
      >
        <NodeStatus onToggleRail={onToggleRail} />
      </NodeContext.Provider>
    </TooltipProvider>,
  );
  return onToggleRail;
}

describe("NodeStatus", () => {
  it("renders the active node name and an Online status", () => {
    renderStatus(base);
    expect(screen.getByText("my-laptop")).toBeTruthy();
    expect(screen.getByText("Online")).toBeTruthy();
  });

  it("renders Offline for a settled, unreachable node", () => {
    renderStatus({ ...base, online: false, pending: false });
    expect(screen.getByText("Offline")).toBeTruthy();
  });

  it("renders Connecting… while the first poll is in flight", () => {
    renderStatus({ ...base, online: false, pending: true });
    expect(screen.getByText("Connecting…")).toBeTruthy();
  });

  it("calls onToggleRail when clicked", () => {
    const onToggleRail = renderStatus(base);
    fireEvent.click(screen.getByTestId("node-status"));
    expect(onToggleRail).toHaveBeenCalledTimes(1);
  });

  it("renders nothing when there is no active node", () => {
    const { container } = render(
      <TooltipProvider>
        <NodeContext.Provider
          value={{ nodes: [], isLoaded: true, activeNodeId: null, activeNode: null, setActiveNode: vi.fn() }}
        >
          <NodeStatus onToggleRail={vi.fn()} />
        </NodeContext.Provider>
      </TooltipProvider>,
    );
    expect(container.querySelector("[data-testid='node-status']")).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd web && pnpm test -- src/components/NodeStatus/index.test.tsx`
Expected: FAIL — cannot resolve `./index` (component not created yet).

- [ ] **Step 3: Write the component**

Create `web/src/components/NodeStatus/index.tsx`:

```tsx
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { NodeWithStatus } from "@/types";

interface Status {
  label: string;
  className: string;
}

// Connection status only. Online wins; an unsettled first poll reads as
// Connecting; a settled failure reads as Offline. Offline is muted rather than
// red — it recedes, matching the rail's "offline never alarms" treatment.
function statusOf(node: NodeWithStatus): Status {
  if (node.online) return { label: "Online", className: "text-green-500" };
  if (node.pending) return { label: "Connecting…", className: "text-amber-500" };
  return { label: "Offline", className: "text-muted-foreground" };
}

/**
 * Compact `name · Status` line under the `argus` wordmark. Renders only when a
 * node is active (the caller already hides it when the sidebar is collapsed).
 * Clicking it toggles the node rail via `onToggleRail`.
 */
export function NodeStatus({ onToggleRail }: { onToggleRail: () => void }) {
  const { activeNode } = useNodeContext();
  if (!activeNode) return null;

  const status = statusOf(activeNode);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-testid="node-status"
          onClick={onToggleRail}
          className="hover:bg-accent/50 flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-sm transition-colors"
        >
          <span className="text-foreground min-w-0 truncate">{activeNode.name}</span>
          <span className="text-muted-foreground flex-shrink-0">·</span>
          <span className={cn("flex-shrink-0 font-medium", status.className)}>
            {status.label}
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent side="bottom">
        {activeNode.name} · {status.label}
      </TooltipContent>
    </Tooltip>
  );
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd web && pnpm test -- src/components/NodeStatus/index.test.tsx`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/NodeStatus/index.tsx web/src/components/NodeStatus/index.test.tsx
git commit -m "feat(multi-node): add NodeStatus snippet component"
```

---

## Task 3: Add `railOpen` state and thread it to the views

**Files:**
- Modify: `web/src/components/views/types.ts:13-15`
- Modify: `web/src/App.tsx:28` and `web/src/App.tsx:411-433`

No test step: this task only adds state plumbing; behavior is exercised by Tasks 4–5. The `pnpm build` typecheck in Step 4 is the gate.

- [ ] **Step 1: Extend `ViewProps`**

In `web/src/components/views/types.ts`, the sidebar block currently reads (lines 13-15):

```ts
  // Sidebar
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
```

Change it to:

```ts
  // Sidebar
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;

  // Node rail (hidden by default; toggled by the NodeStatus snippet)
  railOpen: boolean;
  setRailOpen: (open: boolean) => void;
```

- [ ] **Step 2: Add the state in `App.tsx`**

In `web/src/App.tsx`, after line 28 (`const [sidebarOpen, setSidebarOpen] = useState(false);`) add:

```ts
  const [railOpen, setRailOpen] = useState(false);
```

- [ ] **Step 3: Pass it through `viewProps`**

In `web/src/App.tsx`, in the `viewProps` object (around line 412), after the `setSidebarOpen,` line add:

```ts
    railOpen,
    setRailOpen,
```

- [ ] **Step 4: Verify the type compiles**

Run: `cd web && pnpm build`
Expected: build succeeds (no TypeScript errors). The new props are present on `viewProps` and required by `ViewProps`.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/views/types.ts web/src/App.tsx
git commit -m "feat(multi-node): add railOpen state and thread to views"
```

---

## Task 4: Wire the snippet and gated rail into `DesktopView`

**Files:**
- Modify: `web/src/components/views/DesktopView.tsx`

The desktop view currently renders `<NodeRail />` unconditionally (line 46) and shows the `argus` wordmark in a single header row. After this task the rail renders only when `sidebarOpen && railOpen`, and the snippet sits in a second header row under the wordmark (expanded sidebar only).

- [ ] **Step 1: Import `NodeStatus`**

In `web/src/components/views/DesktopView.tsx`, after the existing `import { NodeRail } from "@/components/NodeRail";` line, add:

```tsx
import { NodeStatus } from "@/components/NodeStatus";
```

- [ ] **Step 2: Destructure the new props**

In the `DesktopView({ ... })` parameter list, add `railOpen,` and `setRailOpen,` next to `sidebarOpen,` / `setSidebarOpen,`.

- [ ] **Step 3: Gate the rail**

Change the unconditional rail (line 46) from:

```tsx
      <NodeRail />
```

to:

```tsx
      {sidebarOpen && railOpen && <NodeRail />}
```

- [ ] **Step 4: Restructure the header to add the snippet row**

Replace the header block — currently:

```tsx
        {/* Header row: branding + toggle */}
        <div
          className={cn(
            "flex items-center px-3 py-3",
            sidebarOpen ? "justify-between" : "justify-center"
          )}
        >
          {sidebarOpen && (
            <h2 className="pl-1 text-2xl font-bold tracking-wide">argus</h2>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setSidebarOpen(!sidebarOpen)}
              >
                {sidebarOpen ? (
                  <PanelLeftClose className="h-4 w-4" />
                ) : (
                  <PanelLeft className="h-4 w-4" />
                )}
              </Button>
            </TooltipTrigger>
            {!sidebarOpen && (
              <TooltipContent side="right">Expand sidebar</TooltipContent>
            )}
          </Tooltip>
        </div>
```

with:

```tsx
        {/* Header: branding + toggle, then the node status snippet (expanded only) */}
        <div className="px-3 py-3">
          <div
            className={cn(
              "flex items-center",
              sidebarOpen ? "justify-between" : "justify-center"
            )}
          >
            {sidebarOpen && (
              <h2 className="pl-1 text-2xl font-bold tracking-wide">argus</h2>
            )}
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setSidebarOpen(!sidebarOpen)}
                >
                  {sidebarOpen ? (
                    <PanelLeftClose className="h-4 w-4" />
                  ) : (
                    <PanelLeft className="h-4 w-4" />
                  )}
                </Button>
              </TooltipTrigger>
              {!sidebarOpen && (
                <TooltipContent side="right">Expand sidebar</TooltipContent>
              )}
            </Tooltip>
          </div>
          {sidebarOpen && (
            <div className="mt-1">
              <NodeStatus onToggleRail={() => setRailOpen(!railOpen)} />
            </div>
          )}
        </div>
```

- [ ] **Step 5: Verify the build and full test suite**

Run: `cd web && pnpm build && pnpm test`
Expected: build succeeds; all tests pass.

- [ ] **Step 6: Manually verify in the running app**

Run the dev server (`cd web && pnpm dev`), then in the browser confirm:
- With the sidebar expanded, `‹node› · Online` shows under `argus`; the rail is hidden.
- Clicking the snippet slides the rail in (pushing content); clicking again hides it.
- Collapsing the sidebar hides both the snippet and the rail; re-expanding restores the rail's prior open/closed state.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/views/DesktopView.tsx
git commit -m "feat(multi-node): toggle rail from NodeStatus snippet (desktop)"
```

---

## Task 5: Wire the snippet and gated rail into `MobileView`

**Files:**
- Modify: `web/src/components/views/MobileView.tsx`

The mobile drawer has no collapsed (56px) state — when open it is always expanded. The rail currently renders unconditionally inside the drawer (`<NodeRail side="left" />`); after this task it renders only when `railOpen`, toggled by the snippet in the drawer header.

- [ ] **Step 1: Import `NodeStatus`**

In `web/src/components/views/MobileView.tsx`, after `import { NodeRail } from "@/components/NodeRail";`, add:

```tsx
import { NodeStatus } from "@/components/NodeStatus";
```

- [ ] **Step 2: Destructure the new props**

In the `MobileView({ ... })` parameter list, add `railOpen,` and `setRailOpen,` next to `sidebarOpen,` / `setSidebarOpen,`.

- [ ] **Step 3: Gate the rail**

Change `<NodeRail side="left" />` to:

```tsx
              {railOpen && <NodeRail side="left" />}
```

- [ ] **Step 4: Add the snippet under the wordmark**

Replace the mobile header block — currently:

```tsx
                {/* Header */}
                <div className="flex items-center justify-between px-3 py-3">
                  <h2 className="pl-1 text-2xl font-bold tracking-wide">
                    argus
                  </h2>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setSidebarOpen(false)}
                  >
                    <PanelLeftClose className="h-4 w-4" />
                  </Button>
                </div>
```

with:

```tsx
                {/* Header: branding + close, then the node status snippet */}
                <div className="px-3 py-3">
                  <div className="flex items-center justify-between">
                    <h2 className="pl-1 text-2xl font-bold tracking-wide">
                      argus
                    </h2>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => setSidebarOpen(false)}
                    >
                      <PanelLeftClose className="h-4 w-4" />
                    </Button>
                  </div>
                  <div className="mt-1">
                    <NodeStatus onToggleRail={() => setRailOpen(!railOpen)} />
                  </div>
                </div>
```

- [ ] **Step 5: Verify the build and full test suite**

Run: `cd web && pnpm build && pnpm test`
Expected: build succeeds; all tests pass.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/views/MobileView.tsx
git commit -m "feat(multi-node): toggle rail from NodeStatus snippet (mobile)"
```

---

## Self-Review Notes

- **Spec coverage:** Hidden-by-default rail (Tasks 4–5, gated on `railOpen`); snippet content `name · Status` with colored status (Task 2); Online/Connecting/Offline derivation (Tasks 1–2); snippet expanded-sidebar-only + collapses with sidebar (Task 4); rail-state retained across sidebar collapse via React state (Task 3, `railOpen` lives in `App`); mobile treatment (Task 5); single-node case unchanged (rail still shows the `+` when opened — `NodeRail` internals untouched). Empty `activeNode` renders nothing (Task 2).
- **Type consistency:** `pending: boolean` added in Task 1 is used by `statusOf` in Task 2 and the fixtures in Tasks 1–2; `railOpen`/`setRailOpen` defined in Task 3 are consumed identically in Tasks 4–5; `NodeStatus` takes a single `onToggleRail: () => void` prop everywhere it is rendered.
- **Offline color:** muted gray (`text-muted-foreground`), matching the rail's deliberate "offline recedes" treatment, per the approved spec.
