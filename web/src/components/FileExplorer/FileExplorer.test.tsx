import { describe, it, expect, beforeAll, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { FileExplorer } from "./index";
import { StubNodeProvider } from "@/test/node-context";
import { useFilesQuery } from "@/data/files";
import { useViewport } from "@/hooks/useViewport";
import type { FilesResponse } from "@/types";

beforeAll(() => {
  // FileTabs scrolls the active tab into view; jsdom has no layout engine.
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

// This file covers the content-loading wiring that lives in index.tsx itself
// (handleFileClick -> loadFileContent -> contents state, and
// handleCloseFile's cleanup of that state) rather than in useFileEditor or
// FileEditor, which have their own test files. It is deliberately not named
// index.test.tsx: that filename belongs to a later PR that replaces this
// component's body wholesale, and taking it whole there must not collide
// with — or silently inherit — tests written against this PR's shape.

// Monaco is heavy and irrelevant here: we only need to see which content the
// pane was handed.
const editorProps = vi.fn();
vi.mock("@monaco-editor/react", () => ({
  default: (props: Record<string, unknown>) => {
    editorProps(props);
    return <div data-testid="monaco">{String(props.value)}</div>;
  },
}));

// Mock the directory listing so the tree renders fixed entries without a
// backend; the content reads below go through the real fetch path (mocked
// via global fetch) since that is exactly the wiring under test.
vi.mock("@/data/files", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/data/files")>()),
  useFilesQuery: vi.fn(),
}));
vi.mock("@/hooks/useViewport", () => ({ useViewport: vi.fn() }));

function mockListing(data: FilesResponse) {
  vi.mocked(useFilesQuery).mockReturnValue({
    data,
    isPending: false,
    isError: false,
    error: null,
    isRefetching: false,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof useFilesQuery>);
}

// A promise this test can resolve on its own schedule, to control the order
// two concurrent file reads settle in.
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function metaUrl(path: string) {
  return `/api/node/files/meta?path=${encodeURIComponent(path)}`;
}
function contentUrl(path: string) {
  return `/api/node/files/content?path=${encodeURIComponent(path)}`;
}

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
function textResponse(body: string) {
  return new Response(body, { status: 200 });
}

// Once a file is open, its name appears twice — once in the tree, once in
// its tab — so plain getByText is ambiguous. The tab is the one whose text
// sits inside the tab's [role="button"] wrapper.
function getTab(name: string): HTMLElement {
  const tab = screen
    .getAllByText(name)
    .map((el) => el.closest('[role="button"]'))
    .find((el): el is HTMLElement => el !== null);
  if (!tab) throw new Error(`no open tab found for ${name}`);
  return tab;
}

function renderExplorer() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <FileExplorer workingDirectory="/repo" />
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
  mockListing({
    files: [
      { name: "a.ts", path: "/repo/a.ts", type: "file" },
      { name: "b.ts", path: "/repo/b.ts", type: "file" },
    ],
    path: "/repo",
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  cleanup();
});

describe("FileExplorer content-loading wiring", () => {
  it("opening a file loads and renders its content", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          return jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" });
        }
        if (input === contentUrl("/repo/a.ts")) {
          return textResponse("content a");
        }
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));

    expect(await screen.findByText("content a")).toBeTruthy();
  });

  // Pins the per-path loading fix: a fetch that resolves for a file that is
  // not the active tab must not clear the active tab's own loading state, or
  // the pane flashes "Select a file to edit" while its read is still pending.
  it("opening a second file while the first is in flight resolves to the right content for the active tab", async () => {
    const aMeta = deferred<Response>();
    const bMeta = deferred<Response>();

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) return aMeta.promise;
        if (input === contentUrl("/repo/a.ts")) return textResponse("content a");
        if (input === metaUrl("/repo/b.ts")) return bMeta.promise;
        if (input === contentUrl("/repo/b.ts")) return textResponse("content b");
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();

    // Open a (its meta fetch is left hanging), then switch to b before a
    // resolves. b becomes the active tab with its own read in flight.
    fireEvent.click(screen.getByText("a.ts"));
    fireEvent.click(screen.getByText("b.ts"));

    expect(screen.queryByText("Select a file to edit")).toBeNull();

    // a's read finishes while b is still active and still loading. The old
    // shared boolean would clear here and the pane would show the idle
    // "Select a file to edit" prompt for a beat; the per-path fix must not.
    aMeta.resolve(
      jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" }),
    );
    await waitFor(() => expect(screen.queryByTestId("monaco")).toBeNull());
    expect(screen.queryByText("Select a file to edit")).toBeNull();

    // Now b's read finishes too, and its content — not a's — renders.
    bMeta.resolve(
      jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/b.ts" }),
    );
    expect(await screen.findByText("content b")).toBeTruthy();
    expect(screen.queryByText("content a")).toBeNull();
  });

  it("closing a tab clears its cached content, so reopening it reads again", async () => {
    let bFetchCount = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          return jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" });
        }
        if (input === contentUrl("/repo/a.ts")) return textResponse("content a");
        if (input === metaUrl("/repo/b.ts")) {
          bFetchCount += 1;
          return jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/b.ts" });
        }
        if (input === contentUrl("/repo/b.ts")) return textResponse("content b");
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();

    fireEvent.click(screen.getByText("a.ts"));
    await screen.findByText("content a");
    fireEvent.click(screen.getByText("b.ts"));
    await screen.findByText("content b");
    expect(bFetchCount).toBe(1);

    // Closing b's tab (the close button next to its name) should drop its
    // cached content, falling back to a as the active tab.
    fireEvent.click(getTab("b.ts").querySelector("button")!);
    await screen.findByText("content a");
    expect(screen.queryByText("content b")).toBeNull();

    // b's tab is gone now, so "b.ts" is unambiguous again (tree only).
    // Reopening it must read it again rather than serving stale state that
    // handleCloseFile failed to clear.
    fireEvent.click(screen.getByText("b.ts"));
    await screen.findByText("content b");
    expect(bFetchCount).toBe(2);
  });
});
