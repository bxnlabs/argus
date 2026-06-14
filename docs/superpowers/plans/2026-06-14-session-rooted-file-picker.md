# Session-rooted File Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the file picker opens from a session, list that session's working directory immediately (and keep typed search grounded there), instead of showing an empty "Looking for something?" prompt.

**Architecture:** A single frontend file changes — `web/src/components/FileBrowser.tsx`. The existing `searchPath` prop (already the fuzzy-search root, already plumbed from every session entry point) is broadened to also be the **browse root**: when the input is empty and `searchPath` is set, the browser lists `searchPath` ("base listing") reusing the existing path-mode listing/breadcrumb/parent/error machinery. No new props, no backend changes. Callers that pass no `searchPath` (e.g. `SourcePicker` directory mode) are unaffected.

**Tech Stack:** React + TypeScript, TanStack Query, Vitest + @testing-library/react.

---

## Background for the implementer (read once)

`FileBrowser` (`web/src/components/FileBrowser.tsx`) has three relevant behaviors today, keyed off the search input text:

- **Path mode** — input starts with `~` or `/`. `isPathMode` is true. It lists a directory via `useFilesQuery(directoryToList)` and shows breadcrumbs + a `..` parent entry.
- **Search mode** — non-path text. It fuzzy-searches via `useFileSearchQuery(query, { searchPath })`. `searchPath` is already passed by every session caller, so search is already grounded at the working directory.
- **Empty input** — shows the literal "Looking for something?" with no listing.

This plan adds a fourth: when the input is empty **and** `searchPath` is set, list `searchPath` (the "base listing"). The cleanest way is to generalize the two path-mode signals into browse signals:

- `isBrowsing` = `isPathMode || isBaseListing` — "we are listing a directory" (vs. searching or idle).
- `browseDir` = the directory to list (`directoryToList` in path mode, `searchPath` in the base listing).

Every place that currently checks `isPathMode` / `directoryToList` for *listing UI* (results, breadcrumbs, parent entry, error state, single-line row layout, empty-state message) switches to `isBrowsing` / `browseDir`. The places that drive *search vs. path parsing* (the debounce effect, `useFileSearchQuery`'s `enabled`, `handleEnter`'s path branch, `drillIntoItem`'s path branch) stay on `isPathMode` — they must not fire for the base listing.

**Decision (from the spec): free navigation.** The base listing shows `..` and breadcrumbs up to root — the session dir is where the picker opens, not a boundary. This falls out of pointing the existing parent/breadcrumb logic at `browseDir`.

---

## Task 1: List the session working directory on empty input

**Files:**
- Modify: `web/src/components/FileBrowser.tsx`
- Test: `web/src/components/FileBrowser.test.tsx` (create)

### - [ ] Step 1: Write the failing tests

Create `web/src/components/FileBrowser.test.tsx`:

```tsx
import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { FileBrowser } from "./FileBrowser";
import { StubNodeProvider } from "@/test/node-context";
import { useFilesQuery, useFileSearchQuery } from "@/data/files";
import { useViewport } from "@/hooks/useViewport";
import type { FilesResponse, FileSearchResult } from "@/types";

// Mock the data layer so we can drive the browser through its listing/search
// states without a backend. Real query hooks would need a live node.
vi.mock("@/data/files", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/data/files")>()),
  useFilesQuery: vi.fn(),
  useFileSearchQuery: vi.fn(),
}));
vi.mock("@/hooks/useViewport", () => ({ useViewport: vi.fn() }));

// FileBrowser calls useFilesQuery twice: once for the browsed directory and
// once for "~" (to learn the home path for tilde display). Resolve by path.
function mockListings(byPath: Record<string, FilesResponse | undefined>) {
  vi.mocked(useFilesQuery).mockImplementation(
    (path: string) =>
      ({
        data: byPath[path],
        isLoading: false,
        error: null,
      }) as unknown as ReturnType<typeof useFilesQuery>,
  );
}

function mockListingError(failPath: string, error: Error) {
  vi.mocked(useFilesQuery).mockImplementation(
    (path: string) =>
      ({
        data: path === failPath ? undefined : { files: [], path },
        isLoading: false,
        error: path === failPath ? error : null,
      }) as unknown as ReturnType<typeof useFilesQuery>,
  );
}

function mockSearch(results: FileSearchResult[]) {
  vi.mocked(useFileSearchQuery).mockReturnValue({
    data: { results, query: "", count: results.length },
    isLoading: false,
  } as unknown as ReturnType<typeof useFileSearchQuery>);
}

function renderBrowser(
  props: { mode?: "directory" | "all"; searchPath?: string } = {},
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <FileBrowser
          open
          onSelect={() => {}}
          onClose={() => {}}
          mode={props.mode ?? "all"}
          searchPath={props.searchPath}
        />
      </StubNodeProvider>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.mocked(useViewport).mockReturnValue({
    isMobile: false,
    isDesktop: true,
    isHydrated: true,
  });
  mockSearch([]);
  mockListings({ "~": { files: [], path: "/home/jeev" } });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("FileBrowser base listing", () => {
  it("lists the session working directory when input is empty and searchPath is set", () => {
    mockListings({
      "/home/jeev/proj": {
        files: [
          { name: "src", path: "/home/jeev/proj/src", type: "directory" },
          { name: "README.md", path: "/home/jeev/proj/README.md", type: "file" },
        ],
        path: "/home/jeev/proj",
      },
      "~": { files: [], path: "/home/jeev" },
    });

    renderBrowser({ searchPath: "/home/jeev/proj" });

    expect(screen.getByText("src")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    // Not the idle prompt — the directory is being shown.
    expect(screen.queryByText("Looking for something?")).not.toBeInTheDocument();
  });

  it("activates the base listing (parent entry, no idle prompt) for an empty working directory", () => {
    // A non-root directory always renders a ".." parent entry (free
    // navigation), so an empty working dir shows ".." rather than the literal
    // "Empty directory" text — which only appears at filesystem root.
    mockListings({
      "/home/jeev/empty": { files: [], path: "/home/jeev/empty" },
      "~": { files: [], path: "/home/jeev" },
    });

    renderBrowser({ searchPath: "/home/jeev/empty" });

    expect(screen.getByText("..")).toBeInTheDocument();
    expect(screen.queryByText("Looking for something?")).not.toBeInTheDocument();
  });

  it("falls back to the idle prompt when no searchPath is provided", () => {
    renderBrowser({});

    expect(screen.getByText("Looking for something?")).toBeInTheDocument();
  });
});
```

### - [ ] Step 2: Run the tests and verify they fail

Run: `cd web && npx vitest run src/components/FileBrowser.test.tsx`

Expected: the first two tests FAIL (the base listing isn't implemented yet — the directory rows / `..` parent are not rendered and the idle prompt shows instead). The third test ("falls back to the idle prompt") PASSES (current behavior).

### - [ ] Step 3: Derive the browse root

In `web/src/components/FileBrowser.tsx`, find the `directoryToList` / `filterSegment` `useMemo` (it ends around line 63, right before the `// --- Debounce for search mode ---` comment). Immediately **after** that `useMemo`, add:

```tsx
  // --- Browse root ---
  // searchPath (the session working directory) is both the search root AND the
  // directory listed when the input is empty ("base listing"). Browsing is
  // active in explicit path mode OR in the base listing.
  const baseDir = searchPath ? searchPath.replace(/\/+$/, "") || "/" : "";
  const isBaseListing = !isPathMode && !query.trim() && !!baseDir;
  const isBrowsing = isPathMode || isBaseListing;
  const browseDir = isPathMode ? directoryToList : baseDir;
```

### - [ ] Step 4: Point the directory listing query at `browseDir`

In the same file, change the `filesQuery` declaration:

```tsx
  // --- API queries ---
  const filesQuery = useFilesQuery(directoryToList, {
    enabled: open && isPathMode && !!directoryToList,
  });
```

to:

```tsx
  // --- API queries ---
  const filesQuery = useFilesQuery(browseDir, {
    enabled: open && isBrowsing && !!browseDir,
  });
```

### - [ ] Step 5: Compute results, parent, breadcrumbs, and rows from the browse signals

Make these four edits in `web/src/components/FileBrowser.tsx`:

**(a) Results memo** — change the guard and its dependency from `isPathMode` to `isBrowsing`:

```tsx
  const { results, isLoading } = useMemo(() => {
    if (isBrowsing) {
      const allFiles = filesQuery.data?.files ?? [];
```

and in that memo's dependency array, replace `isPathMode,` with `isBrowsing,`.

**(b) Parent entry** — change the guard and the directory source:

```tsx
  const parentEntry = useMemo(() => {
    if (!isBrowsing) return null;
    // Don't show at root
    const dir = browseDir;
```

and change its dependency array from `[isPathMode, directoryToList]` to `[isBrowsing, browseDir]`.

**(c) Breadcrumbs** — change the guard, both directory references, and the dependency array:

```tsx
  const breadcrumbs = useMemo(() => {
    if (!isBrowsing || !browseDir) return [];

    const display = homePath
      ? contractTilde(browseDir, homePath)
      : browseDir;
```

and change its dependency array from `[isPathMode, directoryToList, homePath]` to `[isBrowsing, browseDir, homePath]`.

**(d) Single-line row layout** — base-listing rows should look like browsing rows (single-line name), not search rows (two-line name + path). Find the row body conditional (around line 540):

```tsx
                {!isPathMode && !isParent ? (
```

and change it to:

```tsx
                {!isBrowsing && !isParent ? (
```

### - [ ] Step 6: Render breadcrumbs, the error state, and the empty-state message for browsing

Three more edits in `web/src/components/FileBrowser.tsx`:

**(a) Breadcrumbs render guard** (around line 422):

```tsx
      {isPathMode && breadcrumbs.length > 0 && (
```

→

```tsx
      {isBrowsing && breadcrumbs.length > 0 && (
```

**(b) Directory-listing error branch** (around line 462):

```tsx
        ) : filesQuery.error && isPathMode ? (
```

→

```tsx
        ) : filesQuery.error && isBrowsing ? (
```

**(c) Empty-state message** (around line 494) — show "Looking for something?" only when truly idle (no base listing), and the empty-directory message when a browsed directory is empty. Replace:

```tsx
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            {!query.trim()
              ? "Looking for something?"
              : isPathMode && !filterSegment
                ? mode === "directory"
                  ? "No directories found"
                  : "Empty directory"
                : `No matches for “${isPathMode ? filterSegment : query}”`}
          </div>
```

with:

```tsx
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            {!query.trim() && !isBaseListing
              ? "Looking for something?"
              : isBrowsing && !filterSegment
                ? mode === "directory"
                  ? "No directories found"
                  : "Empty directory"
                : `No matches for “${isPathMode ? filterSegment : query}”`}
          </div>
```

### - [ ] Step 7: Run the tests and verify they pass

Run: `cd web && npx vitest run src/components/FileBrowser.test.tsx`

Expected: all three tests PASS.

### - [ ] Step 8: Typecheck and lint

Run: `cd web && npx tsc --noEmit && npx eslint src/components/FileBrowser.tsx src/components/FileBrowser.test.tsx`

Expected: no errors.

### - [ ] Step 9: Commit

```bash
cd "$(git rev-parse --show-toplevel)"
git add web/src/components/FileBrowser.tsx web/src/components/FileBrowser.test.tsx
git commit -m "feat(filepicker): list session working dir on open (BXN-108)"
```

---

## Task 2: Lock in grounded search, navigation, error, and fallback behavior

These tests characterize the rest of the spec's behavior. They pass against Task 1's implementation — they are regression guards, no further production code is expected. If any fails, fix `FileBrowser.tsx` to satisfy it before committing.

**Files:**
- Modify: `web/src/components/FileBrowser.test.tsx`

### - [ ] Step 1: Add the behavior tests

Append to `web/src/components/FileBrowser.test.tsx` (after the existing `describe` block, before end of file):

```tsx
describe("FileBrowser grounding and navigation", () => {
  it("grounds typed search at the session working directory", () => {
    mockListings({
      "/home/jeev/proj": { files: [], path: "/home/jeev/proj" },
      "~": { files: [], path: "/home/jeev" },
    });

    renderBrowser({ searchPath: "/home/jeev/proj" });

    // The search hook is invoked every render with the working dir as its root,
    // so typed search walks the session tree rather than $HOME.
    expect(useFileSearchQuery).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ searchPath: "/home/jeev/proj" }),
    );
  });

  it("drills into a subfolder and returns to the base listing when cleared", () => {
    // Use a path outside $HOME so tilde contraction is a no-op and the drilled
    // query is a predictable absolute path.
    mockListings({
      "/srv/work": {
        files: [{ name: "src", path: "/srv/work/src", type: "directory" }],
        path: "/srv/work",
      },
      "/srv/work/src": {
        files: [{ name: "app.ts", path: "/srv/work/src/app.ts", type: "file" }],
        path: "/srv/work/src",
      },
      "~": { files: [], path: "/home/jeev" },
    });

    renderBrowser({ searchPath: "/srv/work" });

    // Base listing shows the working dir's entries.
    expect(screen.getByText("src")).toBeInTheDocument();

    // Drilling in lists the subfolder.
    fireEvent.click(screen.getByText("src"));
    expect(screen.getByText("app.ts")).toBeInTheDocument();

    // Clearing the input returns to the base listing.
    fireEvent.change(screen.getByPlaceholderText("Search..."), {
      target: { value: "" },
    });
    expect(screen.getByText("src")).toBeInTheDocument();
    expect(screen.queryByText("app.ts")).not.toBeInTheDocument();
  });

  it("surfaces a listing error from the base listing", () => {
    mockListingError("/srv/work", new Error("boom"));

    renderBrowser({ searchPath: "/srv/work" });

    expect(screen.getByText("Could not load directory")).toBeInTheDocument();
  });

  it("keeps the idle prompt in directory mode without a searchPath", () => {
    renderBrowser({ mode: "directory" });

    expect(screen.getByText("Looking for something?")).toBeInTheDocument();
  });
});
```

### - [ ] Step 2: Run the tests and verify they pass

Run: `cd web && npx vitest run src/components/FileBrowser.test.tsx`

Expected: all tests (Task 1 + Task 2) PASS.

If "drills into a subfolder..." fails because the clear-input query selects a different element, target the input via `screen.getByRole("textbox")` instead of `getByPlaceholderText("Search...")`.

### - [ ] Step 3: Commit

```bash
cd "$(git rev-parse --show-toplevel)"
git add web/src/components/FileBrowser.test.tsx
git commit -m "test(filepicker): cover grounded search, drill-in, and error states (BXN-108)"
```

---

## Final verification

### - [ ] Run the full web test suite and typecheck

Run: `cd web && npx vitest run && npx tsc --noEmit`

Expected: all tests pass, no type errors.

### - [ ] Manual smoke (optional, on the vite dev server)

Per repo note, UI changes verify on the vite dev server (`:5273`), not the embedded prod build. Open a session, click the attach/paperclip → file picker. Confirm it opens listing the session's working directory; typing a filename searches within it; `~` jumps to home; clearing returns to the working-dir listing.
