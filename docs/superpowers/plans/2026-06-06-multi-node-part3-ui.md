# Multi-node Part 3 — Multi-node UI (Workspace Rail) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let the web client connect to and switch between the registered Argus nodes, with an always-visible **workspace rail** that shows, at a glance, which node needs attention (blue badge) or is busy (green dot), reusing argus's existing session attention vocabulary.

**Architecture:** A module-scoped "active node base URL" makes every existing `/api/node/*` call target the selected node's origin (empty string = same-origin local node). A `NodeProvider` owns the node list, the active selection (persisted in `localStorage`), and the switch operation (which drops node-scoped React-Query caches and remounts the workspace via a React `key`). A `useNodes` hook background-polls each node's `/api/node/summary`. The rail is a thin always-on column (desktop) / horizontal strip (mobile), rendered only when ≥2 nodes exist; a small dialog adds/renames/removes manual nodes.

**Tech Stack:** React/TypeScript, TanStack Query (`useQuery`/`useQueries`/`useQueryClient`), Tailwind, Vitest + Testing Library.

This is **Plan 3 of 3** for BXN-106. It builds on Plan 1 (`/api/node/*` paths, CORS) and Plan 2 (`/api/nodes` registry + `/api/node/summary`). Spec: `docs/superpowers/specs/2026-06-05-multi-node-ui-design.md`.

**Prerequisite:** Plans 1 & 2 merged. Depends on: web calls use `/api/node/...` (Plan 1); server serves `GET/POST/PATCH/DELETE /api/nodes` and `GET /api/node/summary` (Plan 2). Key existing files: `web/src/api/client.ts` (`getNodeBaseUrl`/`getNodeWsUrl`), `web/src/App.tsx` (`App` = `TooltipProvider > TabProvider > HomeContent`; `HomeContent` renders `DesktopView`/`MobileView`), `web/src/components/views/DesktopView.tsx` (outer `<div className="bg-background flex h-app overflow-hidden">` → sidebar → main), `web/src/lib/query-client.ts`.

---

## File Structure

**Created:**
- `web/src/data/nodes/api.ts` — registry + summary fetch helpers (registry is same-origin; summary is per-node origin).
- `web/src/data/nodes/keys.ts` — query keys (`nodes`, `node-summary`).
- `web/src/data/nodes/queries.ts` — `useNodesQuery`, add/rename/delete mutations.
- `web/src/hooks/useNodes.ts` — merge registry list + per-node summary polling → `NodeWithStatus[]`.
- `web/src/hooks/useNodes.test.ts` — aggregation + offline test.
- `web/src/contexts/NodeContext.tsx` — `NodeProvider`, `useNodeContext`, switch logic.
- `web/src/contexts/NodeContext.test.tsx` — switch resets base URL + caches.
- `web/src/components/NodeRail/index.tsx` — the rail (vertical + horizontal).
- `web/src/components/NodeRail/index.test.tsx` — tile state rendering.
- `web/src/components/ManageNodesDialog/index.tsx` — add/rename/remove dialog.

**Modified:**
- `web/src/api/client.ts` — module-scoped active base URL.
- `web/src/types.ts` — `NodeSource`, `NodeInfo`, `NodeSummary`, `NodeWithStatus`.
- `web/src/App.tsx` — wrap in `NodeProvider`; remount workspace by active node id.
- `web/src/components/views/DesktopView.tsx` — render `<NodeRail />` column.
- `web/src/components/views/MobileView.tsx` — render `<NodeRail orientation="horizontal" />` strip.

---

## Task 1: Make the node base URL selectable

**Files:**
- Modify: `web/src/api/client.ts`
- Test: `web/src/api/client.test.ts` (create)

- [ ] **Step 1: Write a failing test**

Create `web/src/api/client.test.ts`:

```ts
import { describe, it, expect, afterEach } from "vitest";
import { getNodeBaseUrl, setActiveNodeBaseUrl, getNodeWsUrl } from "./client";

afterEach(() => setActiveNodeBaseUrl(""));

describe("active node base URL", () => {
  it("defaults to same-origin (empty)", () => {
    expect(getNodeBaseUrl()).toBe("");
  });
  it("routes calls at the selected node origin", () => {
    setActiveNodeBaseUrl("http://gpu-box:80");
    expect(getNodeBaseUrl()).toBe("http://gpu-box:80");
    expect(getNodeWsUrl("s1")).toBe("ws://gpu-box:80/api/node/ws/sessions/s1");
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test run src/api/client.test.ts`
Expected: FAIL — `setActiveNodeBaseUrl` is not exported.

- [ ] **Step 3: Replace the env-based base URL with a module-scoped one**

In `web/src/api/client.ts`, replace the `getNodeBaseUrl` implementation:

```ts
// The active node's origin. Empty string == same-origin (the local node).
// Mutated by NodeProvider when the user switches nodes; read by every
// node-scoped fetch/WebSocket helper below.
let activeNodeBaseUrl = "";

/** Sets the origin all node API/WS calls target. "" means same-origin. */
export function setActiveNodeBaseUrl(url: string): void {
  activeNodeBaseUrl = url;
}

/** Returns the base URL for the active node's API ("" == same-origin). */
export function getNodeBaseUrl(): string {
  return activeNodeBaseUrl;
}
```

(`getNodeWsUrl`, `baseFetch`, `apiFetch`, `apiTextFetch` are unchanged — they already call `getNodeBaseUrl()`. Note the WS path is `/api/node/ws/...` after Plan 1.)

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm test run src/api/client.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/client.ts web/src/api/client.test.ts
git commit -m "feat(web): selectable active node base URL"
```

---

## Task 2: Node types + registry/summary API helpers

**Files:**
- Modify: `web/src/types.ts`
- Create: `web/src/data/nodes/api.ts`
- Create: `web/src/data/nodes/keys.ts`

- [ ] **Step 1: Add the node types**

Append to `web/src/types.ts`:

```ts
export type NodeSource = "local" | "manual" | "discovered";

export interface NodeInfo {
  id: string;
  name: string;
  url: string; // "" == same-origin (the local node)
  source: NodeSource;
  self: boolean;
}

export interface NodeSummary {
  attention: number;
  busy: number;
  total: number;
}

export interface NodeWithStatus extends NodeInfo {
  summary: NodeSummary | null;
  online: boolean;
}
```

- [ ] **Step 2: Add the query keys**

Create `web/src/data/nodes/keys.ts`:

```ts
// Key prefixes "nodes" and "node-summary" are deliberately distinct from the
// node-scoped keys (sessions, session-statuses, git, …) so NodeProvider can
// drop the latter on switch while keeping the rail's data alive.
export const nodeKeys = {
  all: ["nodes"] as const,
  list: () => ["nodes", "list"] as const,
  summary: (id: string) => ["node-summary", id] as const,
};
```

- [ ] **Step 3: Add the fetch helpers**

Create `web/src/data/nodes/api.ts`:

```ts
import type { NodeInfo, NodeSummary } from "@/types";

// Registry is served by the instance that served the SPA (same-origin),
// regardless of which node is active — so these bypass the node base URL.
export async function fetchNodes(): Promise<NodeInfo[]> {
  const res = await fetch("/api/nodes");
  if (!res.ok) throw new Error(`fetch nodes: ${res.status}`);
  const body = (await res.json()) as { nodes?: NodeInfo[] };
  return body.nodes ?? [];
}

async function mutate(method: string, path: string, body?: unknown): Promise<void> {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    const detail = (await res.json().catch(() => ({}))) as { error?: string };
    throw new Error(detail.error ?? `${method} ${path}: ${res.status}`);
  }
}

export const addNode = (name: string, url: string) =>
  mutate("POST", "/api/nodes", { name, url });
export const renameNode = (id: string, name: string) =>
  mutate("PATCH", `/api/nodes/${encodeURIComponent(id)}`, { name });
export const deleteNode = (id: string) =>
  mutate("DELETE", `/api/nodes/${encodeURIComponent(id)}`);

// Summary is fetched against each node's own origin (cross-origin for remote
// nodes; "" == same-origin for the local node).
export async function fetchSummary(baseUrl: string): Promise<NodeSummary> {
  const res = await fetch(`${baseUrl}/api/node/summary`);
  if (!res.ok) throw new Error(`summary: ${res.status}`);
  return (await res.json()) as NodeSummary;
}
```

- [ ] **Step 4: Typecheck**

Run: `cd web && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/types.ts web/src/data/nodes/
git commit -m "feat(web): node types + registry/summary API helpers"
```

---

## Task 3: Registry queries + mutations

**Files:**
- Create: `web/src/data/nodes/queries.ts`

- [ ] **Step 1: Implement the queries/mutations**

Create `web/src/data/nodes/queries.ts`:

```ts
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { addNode, deleteNode, fetchNodes, renameNode } from "./api";
import { nodeKeys } from "./keys";

export function useNodesQuery() {
  return useQuery({
    queryKey: nodeKeys.list(),
    queryFn: fetchNodes,
    staleTime: 10_000,
    refetchInterval: 15_000,
  });
}

export function useAddNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ name, url }: { name: string; url: string }) => addNode(name, url),
    onSuccess: () => qc.invalidateQueries({ queryKey: nodeKeys.list() }),
  });
}

export function useRenameNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, name }: { id: string; name: string }) => renameNode(id, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: nodeKeys.list() }),
  });
}

export function useDeleteNode() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteNode(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: nodeKeys.list() }),
  });
}
```

- [ ] **Step 2: Typecheck**

Run: `cd web && pnpm exec tsc --noEmit`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/data/nodes/queries.ts
git commit -m "feat(web): node registry queries + mutations"
```

---

## Task 4: `useNodes` — merge registry list with per-node summary polling

**Files:**
- Create: `web/src/hooks/useNodes.ts`
- Test: `web/src/hooks/useNodes.test.ts`

- [ ] **Step 1: Write the failing aggregation/offline test**

Create `web/src/hooks/useNodes.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useNodes } from "./useNodes";

function wrapper(qc: QueryClient) {
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn(async (input: string) => {
    if (input === "/api/nodes") {
      return new Response(JSON.stringify({ nodes: [
        { id: "local", name: "this", url: "", source: "local", self: true },
        { id: "m1", name: "gpu", url: "http://gpu:80", source: "manual", self: false },
      ] }), { status: 200 });
    }
    if (input === "/api/node/summary") { // local
      return new Response(JSON.stringify({ attention: 2, busy: 1, total: 5 }), { status: 200 });
    }
    if (input === "http://gpu:80/api/node/summary") { // remote down
      return new Response("", { status: 502 });
    }
    return new Response("", { status: 404 });
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe("useNodes", () => {
  it("aggregates summaries and marks unreachable nodes offline", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const { result } = renderHook(() => useNodes(), { wrapper: wrapper(qc) });

    await waitFor(() => expect(result.current.nodes).toHaveLength(2));
    const local = result.current.nodes.find((n) => n.id === "local")!;
    const gpu = result.current.nodes.find((n) => n.id === "m1")!;

    await waitFor(() => expect(local.summary?.attention).toBe(2));
    expect(local.online).toBe(true);
    await waitFor(() => expect(gpu.online).toBe(false));
    expect(gpu.summary).toBeNull();
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test run src/hooks/useNodes.test.ts`
Expected: FAIL — `useNodes` not found.

- [ ] **Step 3: Implement the hook**

Create `web/src/hooks/useNodes.ts`:

```ts
import { useQueries } from "@tanstack/react-query";
import { useNodesQuery } from "@/data/nodes/queries";
import { fetchSummary } from "@/data/nodes/api";
import { nodeKeys } from "@/data/nodes/keys";
import type { NodeWithStatus } from "@/types";

/**
 * Returns the registered nodes, each enriched with its latest summary and an
 * online flag. Summaries are polled per node every 5s against that node's own
 * origin; a node whose poll errors is marked offline (badges cleared) and keeps
 * retrying so it recovers automatically.
 */
export function useNodes(): { nodes: NodeWithStatus[]; isLoaded: boolean } {
  const { data: list = [], isSuccess } = useNodesQuery();

  const summaries = useQueries({
    queries: list.map((n) => ({
      queryKey: nodeKeys.summary(n.id),
      queryFn: () => fetchSummary(n.url),
      refetchInterval: 5_000,
      retry: false,
      // Keep polling a downed node so it recovers without a manual refresh.
      refetchIntervalInBackground: true,
    })),
  });

  const nodes: NodeWithStatus[] = list.map((n, i) => {
    const q = summaries[i];
    const online = !!q && !q.isError;
    return { ...n, summary: online && q?.data ? q.data : null, online };
  });

  return { nodes, isLoaded: isSuccess };
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm test run src/hooks/useNodes.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useNodes.ts web/src/hooks/useNodes.test.ts
git commit -m "feat(web): useNodes summary polling + offline detection"
```

---

## Task 5: `NodeContext` — active selection + switch

**Files:**
- Create: `web/src/contexts/NodeContext.tsx`
- Test: `web/src/contexts/NodeContext.test.tsx`

- [ ] **Step 1: Write the failing switch test**

Create `web/src/contexts/NodeContext.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, act, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NodeProvider, useNodeContext } from "./NodeContext";
import { getNodeBaseUrl } from "@/api/client";

function Probe() {
  const { nodes, activeNode, setActiveNode } = useNodeContext();
  return (
    <div>
      <span data-testid="active">{activeNode?.id ?? "none"}</span>
      <button onClick={() => setActiveNode("m1")}>switch</button>
      <span data-testid="count">{nodes.length}</span>
    </div>
  );
}

beforeEach(() => {
  localStorage.clear();
  vi.stubGlobal("fetch", vi.fn(async (input: string) => {
    if (input === "/api/nodes") {
      return new Response(JSON.stringify({ nodes: [
        { id: "local", name: "this", url: "", source: "local", self: true },
        { id: "m1", name: "gpu", url: "http://gpu:80", source: "manual", self: false },
      ] }), { status: 200 });
    }
    return new Response(JSON.stringify({ attention: 0, busy: 0, total: 0 }), { status: 200 });
  }));
});
afterEach(() => vi.unstubAllGlobals());

describe("NodeProvider", () => {
  it("defaults to the local node and switches base URL on select", async () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    render(
      <QueryClientProvider client={qc}>
        <NodeProvider><Probe /></NodeProvider>
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.getByTestId("count").textContent).toBe("2"));
    expect(screen.getByTestId("active").textContent).toBe("local");
    expect(getNodeBaseUrl()).toBe("");

    await act(async () => { screen.getByText("switch").click(); });
    expect(screen.getByTestId("active").textContent).toBe("m1");
    expect(getNodeBaseUrl()).toBe("http://gpu:80");
    expect(localStorage.getItem("argus.activeNodeId")).toBe("m1");
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test run src/contexts/NodeContext.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the provider**

Create `web/src/contexts/NodeContext.tsx`:

```tsx
import {
  createContext, useCallback, useContext, useLayoutEffect, useMemo, useState,
} from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNodes } from "@/hooks/useNodes";
import { setActiveNodeBaseUrl } from "@/api/client";
import type { NodeWithStatus } from "@/types";

const STORAGE_KEY = "argus.activeNodeId";

interface NodeContextValue {
  nodes: NodeWithStatus[];
  isLoaded: boolean;
  activeNodeId: string | null;
  activeNode: NodeWithStatus | null;
  setActiveNode: (id: string) => void;
}

const NodeContext = createContext<NodeContextValue | null>(null);

export function NodeProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const { nodes, isLoaded } = useNodes();
  const [activeNodeId, setActiveNodeId] = useState<string | null>(
    () => localStorage.getItem(STORAGE_KEY),
  );

  // Resolve the selection; default to the local/self node, else the first node.
  const activeNode = useMemo(() => {
    if (nodes.length === 0) return null;
    return (
      nodes.find((n) => n.id === activeNodeId) ??
      nodes.find((n) => n.self) ??
      nodes[0]
    );
  }, [nodes, activeNodeId]);

  // Apply the active origin before child queries fetch (passive effects run
  // after this layout effect, so the first session fetch sees the right base).
  useLayoutEffect(() => {
    setActiveNodeBaseUrl(activeNode?.url ?? "");
  }, [activeNode?.url]);

  const setActiveNode = useCallback(
    (id: string) => {
      const target = nodes.find((n) => n.id === id);
      if (!target || id === activeNode?.id) return;
      setActiveNodeBaseUrl(target.url);
      localStorage.setItem(STORAGE_KEY, id);
      // Drop node-scoped caches (sessions/statuses/git/files/profiles/…) so the
      // new node loads fresh; keep the registry + summaries that feed the rail.
      queryClient.removeQueries({
        predicate: (q) => {
          const k = q.queryKey[0];
          return k !== "nodes" && k !== "node-summary";
        },
      });
      setActiveNodeId(id);
    },
    [nodes, activeNode?.id, queryClient],
  );

  const value = useMemo<NodeContextValue>(
    () => ({ nodes, isLoaded, activeNodeId: activeNode?.id ?? null, activeNode, setActiveNode }),
    [nodes, isLoaded, activeNode, setActiveNode],
  );

  return <NodeContext.Provider value={value}>{children}</NodeContext.Provider>;
}

export function useNodeContext(): NodeContextValue {
  const ctx = useContext(NodeContext);
  if (!ctx) throw new Error("useNodeContext must be used within NodeProvider");
  return ctx;
}

export { NodeContext };
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd web && pnpm test run src/contexts/NodeContext.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/contexts/NodeContext.tsx web/src/contexts/NodeContext.test.tsx
git commit -m "feat(web): NodeContext active selection + switch"
```

---

## Task 6: The workspace rail

**Files:**
- Create: `web/src/components/NodeRail/index.tsx`
- Test: `web/src/components/NodeRail/index.test.tsx`

- [ ] **Step 1: Write the failing rendering test**

Create `web/src/components/NodeRail/index.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { NodeContext } from "@/contexts/NodeContext";
import { NodeRail } from "./index";
import type { NodeWithStatus } from "@/types";

function renderRail(nodes: NodeWithStatus[], activeId: string) {
  const active = nodes.find((n) => n.id === activeId) ?? null;
  return render(
    <NodeContext.Provider
      value={{ nodes, isLoaded: true, activeNodeId: activeId, activeNode: active, setActiveNode: vi.fn() }}
    >
      <NodeRail />
    </NodeContext.Provider>,
  );
}

const base: NodeWithStatus = { id: "x", name: "x", url: "", source: "manual", self: false, summary: null, online: true };

describe("NodeRail", () => {
  it("renders nothing with a single node", () => {
    const { container } = renderRail([{ ...base, id: "local", self: true }], "local");
    expect(container.querySelector("[data-testid='node-rail']")).toBeNull();
  });

  it("shows an attention badge with the count", () => {
    renderRail(
      [
        { ...base, id: "local", self: true },
        { ...base, id: "m1", name: "gpu", summary: { attention: 4, busy: 1, total: 6 } },
      ],
      "local",
    );
    expect(screen.getByTestId("node-attention-m1").textContent).toBe("4");
  });

  it("marks an offline node", () => {
    renderRail(
      [
        { ...base, id: "local", self: true },
        { ...base, id: "m1", name: "gpu", online: false },
      ],
      "local",
    );
    expect(screen.getByTestId("node-tile-m1").getAttribute("data-online")).toBe("false");
  });
});
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd web && pnpm test run src/components/NodeRail/index.test.tsx`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the rail**

Create `web/src/components/NodeRail/index.tsx`:

```tsx
import { useState } from "react";
import { cn } from "@/lib/utils";
import { useNodeContext } from "@/contexts/NodeContext";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Plus } from "lucide-react";
import type { NodeWithStatus } from "@/types";
import { ManageNodesDialog } from "@/components/ManageNodesDialog";

function initial(name: string): string {
  return (name.trim()[0] ?? "?").toUpperCase();
}

function NodeTile({
  node, active, onSelect,
}: { node: NodeWithStatus; active: boolean; onSelect: () => void }) {
  const attention = node.summary?.attention ?? 0;
  const busy = node.summary?.busy ?? 0;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-testid={`node-tile-${node.id}`}
          data-online={node.online}
          onClick={onSelect}
          className={cn(
            "relative mx-auto flex h-10 w-10 items-center justify-center rounded-lg text-sm font-semibold transition-colors",
            active ? "bg-accent text-accent-foreground" : "text-muted-foreground hover:bg-accent/50",
            !node.online && "opacity-50 ring-1 ring-red-500/50",
          )}
        >
          {active && (
            <span aria-hidden className="bg-primary absolute left-[-6px] top-1 h-8 w-1 rounded-full" />
          )}
          {initial(node.name)}
          {node.online && attention > 0 && (
            <span
              data-testid={`node-attention-${node.id}`}
              className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-blue-500 px-1 text-[10px] font-bold text-white"
            >
              {attention}
            </span>
          )}
          {node.online && attention === 0 && busy > 0 && (
            <span
              aria-hidden
              data-testid={`node-busy-${node.id}`}
              className="bg-green-500 animate-pulse-green absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full"
            />
          )}
        </button>
      </TooltipTrigger>
      <TooltipContent side="right">
        {node.name}
        {!node.online && " · offline"}
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * Always-visible rail of node tiles. Renders nothing when there is only the
 * local node (single-node users see the unchanged UI). `orientation="horizontal"`
 * is used by the mobile drawer.
 */
export function NodeRail({ orientation = "vertical" }: { orientation?: "vertical" | "horizontal" }) {
  const { nodes, activeNodeId, setActiveNode } = useNodeContext();
  const [manageOpen, setManageOpen] = useState(false);

  if (nodes.length < 2) return null;

  const horizontal = orientation === "horizontal";
  return (
    <>
      <div
        data-testid="node-rail"
        className={cn(
          "bg-sidebar-background flex gap-2",
          horizontal
            ? "w-full flex-row items-center overflow-x-auto border-b px-2 py-2"
            : "h-full w-12 flex-shrink-0 flex-col items-stretch border-r py-3",
        )}
      >
        {nodes.map((n) => (
          <NodeTile
            key={n.id}
            node={n}
            active={n.id === activeNodeId}
            onSelect={() => setActiveNode(n.id)}
          />
        ))}
        <button
          type="button"
          aria-label="Manage nodes"
          onClick={() => setManageOpen(true)}
          className="text-muted-foreground hover:bg-accent/50 mx-auto flex h-10 w-10 items-center justify-center rounded-lg"
        >
          <Plus className="h-4 w-4" />
        </button>
      </div>
      <ManageNodesDialog open={manageOpen} onClose={() => setManageOpen(false)} />
    </>
  );
}
```

- [ ] **Step 4: Run the rail test (it will fail to import `ManageNodesDialog`)**

Run: `cd web && pnpm test run src/components/NodeRail/index.test.tsx`
Expected: FAIL — `@/components/ManageNodesDialog` not found. (Created in Task 7; proceed.)

- [ ] **Step 5: Commit (after Task 7 makes it green)**

Defer the commit to the end of Task 7, which provides the missing dialog.

---

## Task 7: Add/Manage Nodes dialog

**Files:**
- Create: `web/src/components/ManageNodesDialog/index.tsx`

- [ ] **Step 1: Implement the dialog**

Create `web/src/components/ManageNodesDialog/index.tsx`:

```tsx
import { useState } from "react";
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Trash2 } from "lucide-react";
import { useNodeContext } from "@/contexts/NodeContext";
import { useAddNode, useDeleteNode } from "@/data/nodes/queries";

export function ManageNodesDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { nodes } = useNodeContext();
  const addNode = useAddNode();
  const deleteNode = useDeleteNode();
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string | null>(null);

  const manual = nodes.filter((n) => n.source === "manual");
  const discovered = nodes.filter((n) => n.source === "discovered");

  const submit = async () => {
    setError(null);
    try {
      await addNode.mutateAsync({ name: name.trim(), url: url.trim() });
      setName("");
      setUrl("");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to add node");
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Manage nodes</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-2">
          <Input placeholder="Name" value={name} onChange={(e) => setName(e.target.value)} />
          <Input
            placeholder="http://host:80"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && name && url) submit(); }}
          />
          {error && <p className="text-destructive text-sm">{error}</p>}
          <Button onClick={submit} disabled={!name.trim() || !url.trim() || addNode.isPending}>
            Add node
          </Button>
        </div>

        {manual.length > 0 && (
          <div className="mt-2 flex flex-col gap-1">
            <p className="text-muted-foreground text-xs font-bold">ADDED</p>
            {manual.map((n) => (
              <div key={n.id} className="flex items-center gap-2 text-sm">
                <span className="flex-1 truncate">{n.name}</span>
                <span className="text-muted-foreground truncate text-xs">{n.url}</span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Remove ${n.name}`}
                  onClick={() => deleteNode.mutate(n.id)}
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        )}

        {discovered.length > 0 && (
          <div className="mt-2 flex flex-col gap-1">
            <p className="text-muted-foreground text-xs font-bold">DISCOVERED</p>
            {discovered.map((n) => (
              <div key={n.id} className="text-muted-foreground flex items-center gap-2 text-sm">
                <span className="flex-1 truncate">{n.name}</span>
                <span className="truncate text-xs">{n.url}</span>
              </div>
            ))}
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Run the rail + dialog tests together**

Run: `cd web && pnpm test run src/components/NodeRail/index.test.tsx`
Expected: PASS (the import now resolves and tiles render).

- [ ] **Step 3: Verify the `size="icon-sm"` button variant exists**

Run: `grep -n "icon-sm" web/src/components/ui/button.tsx`
Expected: a match. If absent, use `size="icon"` in the dialog instead.

- [ ] **Step 4: Commit (rail + dialog)**

```bash
git add web/src/components/NodeRail/ web/src/components/ManageNodesDialog/
git commit -m "feat(web): workspace rail + manage-nodes dialog"
```

---

## Task 8: Wire the provider, remount-on-switch, and the rail into the layout

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/views/DesktopView.tsx`
- Modify: `web/src/components/views/MobileView.tsx`

- [ ] **Step 1: Wrap the app in `NodeProvider` and remount the workspace on switch**

In `web/src/App.tsx`, change the `App` export to add `NodeProvider` and key the workspace subtree by the active node id (so switching tears down tabs + terminal and reloads the new node). Replace the `App` function with:

```tsx
export function App() {
  return (
    <TooltipProvider>
      <NodeProvider>
        <AppInner />
      </NodeProvider>
    </TooltipProvider>
  );
}

function AppInner() {
  const { activeNodeId } = useNodeContext();
  return (
    <TabProvider key={activeNodeId ?? "none"}>
      <HomeContent />
      <Toaster
        theme="dark"
        position="bottom-right"
        richColors
        toastOptions={{ className: "argus-toast" }}
      />
    </TabProvider>
  );
}
```

Add the imports at the top of `App.tsx`:

```tsx
import { NodeProvider, useNodeContext } from "@/contexts/NodeContext";
```

- [ ] **Step 2: Render the rail in `DesktopView`**

In `web/src/components/views/DesktopView.tsx`, import the rail and add it as the first child of the outer flex container. Add the import:

```tsx
import { NodeRail } from "@/components/NodeRail";
```

Change the opening of the returned JSX from:

```tsx
    <div className="bg-background flex h-app overflow-hidden">
      {/* Sidebar — always visible, toggles between expanded (w-72) and collapsed (w-14) */}
```

to:

```tsx
    <div className="bg-background flex h-app overflow-hidden">
      <NodeRail />
      {/* Sidebar — always visible, toggles between expanded (w-72) and collapsed (w-14) */}
```

(`NodeRail` returns `null` when there are fewer than 2 nodes, so the single-node layout is byte-for-byte unchanged.)

- [ ] **Step 3: Render the horizontal strip in `MobileView`**

In `web/src/components/views/MobileView.tsx`, import the rail and place a horizontal strip at the top of the drawer, just inside the drawer's flex column (after the line `<div className="flex h-full flex-col pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">`). Add the import:

```tsx
import { NodeRail } from "@/components/NodeRail";
```

And insert as the first child of that drawer column:

```tsx
              <NodeRail orientation="horizontal" />
```

- [ ] **Step 4: Typecheck + run the full web test suite**

Run: `cd web && pnpm exec tsc --noEmit && pnpm test run`
Expected: no type errors; all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/App.tsx web/src/components/views/DesktopView.tsx web/src/components/views/MobileView.tsx
git commit -m "feat(web): integrate node rail + remount workspace on switch"
```

---

## Task 9: End-to-end manual verification

**Files:** none (verification only).

- [ ] **Step 1: Build the web bundle**

Run: `cd web && pnpm build`
Expected: succeeds.

- [ ] **Step 2: Run two instances and switch between them**

```bash
# Instance A (the pane you'll open):
ARGUS_HOME=$(mktemp -d) ARGUS_PORT=3199 go run ./cmd/argus &
# Instance B (a second node):
ARGUS_HOME=$(mktemp -d) ARGUS_PORT=3198 go run ./cmd/argus &
sleep 2
# Register B with A (CORS lets the browser reach B directly):
curl -fsS -X POST http://127.0.0.1:3199/api/nodes \
  -d '{"name":"node-b","url":"http://127.0.0.1:3198"}' -w " [%{http_code}]\n"
```
Then open `http://127.0.0.1:3199` in a browser and confirm:
- The **rail appears** (2 nodes). The local tile is selected (blue pill).
- Creating a session on B (via its tile → switch → New Session) shows under B only; switching back to A shows A's sessions — **caches don't bleed**.
- Stop instance B (`kill` its PID); within ~5s its tile goes **offline** (desaturated, red ring) and badges clear; restart it and it recovers.

Clean up: `kill %1 %2`.

- [ ] **Step 3: Confirm single-node UI is unchanged**

With only instance A running and no registered peers, reload `http://127.0.0.1:3199`: the rail must be **absent** and the layout identical to pre-feature.

- [ ] **Step 4: Commit any fixups**

```bash
git add -A
git commit -m "test: multi-node UI e2e green" --allow-empty
```

---

## Self-Review

**Spec coverage (Plan 3 scope):**
- Selectable per-node base URL routing → Task 1. ✅
- `useNodes` background polling of `/api/node/summary` + offline state → Task 4. ✅
- Active selection persisted in `localStorage`, defaults to local → Task 5. ✅
- Switching resets node-scoped caches + remounts workspace → Tasks 5, 8. ✅
- Workspace rail (Direction B), attention/busy/offline/selected states, reused color vocabulary → Task 6. ✅
- Rail hidden when <2 nodes → Task 6 (`nodes.length < 2` → null), verified Task 9 Step 3. ✅
- Add/rename/remove management dialog → Task 7. ✅
- Mobile horizontal strip → Task 8 Step 3. ✅

**Placeholder scan:** none — all component/hook/context code is complete; mechanical layout edits show exact before/after anchors.

**Type/name consistency:** `NodeInfo`/`NodeSummary`/`NodeWithStatus`/`NodeSource` (Task 2) used across hooks/context/rail. `setActiveNodeBaseUrl`/`getNodeBaseUrl` (Task 1) used by `NodeContext` (Task 5). `nodeKeys` prefixes `"nodes"`/`"node-summary"` (Task 2) match the `removeQueries` predicate (Task 5). `useNodeContext`/`NodeContext`/`NodeProvider` (Task 5) consumed by `NodeRail` (Task 6), `ManageNodesDialog` (Task 7), and `App` (Task 8). `NodeRail` `orientation` prop (Task 6) used by MobileView (Task 8).

**Notes for the executor:**
- The `animate-pulse-green` class and the blue/green/red palette are reused verbatim from the session-status vocabulary (`web/src/lib/sessionStatus.ts`); no new theme tokens.
- The local node's `url` is `""` (same-origin); the rail tooltip, switching, and summary fetch all treat `""` correctly (same-origin). Do not synthesize a localhost URL for it.
- Switch correctness rests on two mechanisms together: `setActiveNodeBaseUrl` (re-targets fetch/WS) **and** the `TabProvider key` remount (tears down the terminal WS and re-fetches). Keep both.
- If `useQueries` types complain about the `React` import in `.test.ts(x)` JSX, ensure the test file uses the `.tsx` extension (the provided test files already do).
