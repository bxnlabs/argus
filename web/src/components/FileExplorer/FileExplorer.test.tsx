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
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
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
// sits inside the tab's [role="button"] wrapper; tree rows are plain
// <button> elements, which that attribute selector does not match.
function queryTab(name: string): HTMLElement | null {
  return (
    screen
      .queryAllByText(name)
      .map((el) => el.closest('[role="button"]'))
      .find((el): el is HTMLElement => el !== null) ?? null
  );
}

function getTab(name: string): HTMLElement {
  const tab = queryTab(name);
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

  // The tab is opened before the read is attempted, so a read that fails must
  // say so rather than leaving the tab selected and blank. The tab stays put —
  // it is the thing you close or retry from — but it has to carry the failure.
  it("a read that fails reports the error and offers a retry", async () => {
    // Deferred so the tab's existence can be asserted *before* the read fails.
    // Rejecting immediately would let this test pass even if the click had
    // never opened a tab at all.
    const aMeta = deferred<Response>();
    let metaCount = 0;
    let failNext = true;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          metaCount += 1;
          if (failNext) return aMeta.promise;
          return jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" });
        }
        if (input === contentUrl("/repo/a.ts")) return textResponse("content a");
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));

    // The tab is really up and the read is really in flight...
    expect(metaCount).toBe(1);
    expect(queryTab("a.ts")).not.toBeNull();

    // ...and the failure surfaces in the pane rather than silently blanking it.
    aMeta.reject(new Error("file not found"));
    expect(await screen.findByText("file not found")).toBeTruthy();

    // The tab survives, flagged, so it can be closed or retried.
    const tab = queryTab("a.ts");
    expect(tab).not.toBeNull();
    expect(screen.getByLabelText("a.ts failed to load")).toBeTruthy();
    // Not the idle prompt: a file *is* selected, it just could not be read.
    expect(screen.queryByText("Select a file to edit")).toBeNull();
    expect(screen.queryByTestId("monaco")).toBeNull();

    // Retrying re-reads the file and clears the error.
    failNext = false;
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));

    expect(await screen.findByText("content a")).toBeTruthy();
    expect(metaCount).toBe(2);
    expect(screen.queryByText("file not found")).toBeNull();
    expect(screen.queryByLabelText("a.ts failed to load")).toBeNull();
  });

  // Closing an errored tab must take its error with it, or reopening the file
  // lands straight back in the error pane without ever reading anything.
  it("closing an errored tab clears the error, so reopening reads again", async () => {
    let failNext = true;
    let metaCount = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          metaCount += 1;
          if (failNext) throw new Error("file not found");
          return jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" });
        }
        if (input === contentUrl("/repo/a.ts")) return textResponse("content a");
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));
    await screen.findByText("file not found");

    fireEvent.click(getTab("a.ts").querySelector("button")!);
    await waitFor(() => expect(queryTab("a.ts")).toBeNull());
    expect(screen.queryByText("file not found")).toBeNull();

    failNext = false;
    fireEvent.click(screen.getByText("a.ts"));

    expect(await screen.findByText("content a")).toBeTruthy();
    expect(metaCount).toBe(2);
  });

  // Closing a tab while its read is still in flight must abandon that read.
  // If the late response is still allowed to land in the content cache, it
  // populates a path that has no tab, and the next open of that file serves
  // those bytes forever instead of reading the file again.
  it("closing a tab mid-read abandons it, so reopening reads again", async () => {
    const aMeta = deferred<Response>();
    let aMetaCount = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          aMetaCount += 1;
          // Only the first open hangs; the reopen resolves immediately.
          return aMetaCount === 1
            ? aMeta.promise
            : jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" });
        }
        if (input === contentUrl("/repo/a.ts")) return textResponse("content a");
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();

    fireEvent.click(screen.getByText("a.ts"));
    expect(aMetaCount).toBe(1);

    // Close it while the meta read is still hanging.
    fireEvent.click(getTab("a.ts").querySelector("button")!);
    await waitFor(() => expect(queryTab("a.ts")).toBeNull());

    // The abandoned read now completes. It must not repopulate the cache.
    // (Asserting on the editor here would prove nothing — with no active tab
    // there is no editor either way. The reopen below is what pins it.)
    aMeta.resolve(
      jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" }),
    );

    // Reopening must issue a genuinely new read rather than serving whatever
    // the abandoned one left behind.
    fireEvent.click(screen.getByText("a.ts"));
    await screen.findByText("content a");
    expect(aMetaCount).toBe(2);
  });

  // Backing out of a file that is still loading must not block the phone's
  // file tree behind anything. The read keeps running, so returning to the tab
  // finds the file already there rather than starting over.
  it("on mobile, backing out mid-read leaves the tree usable and the read running", async () => {
    vi.mocked(useViewport).mockReturnValue({
      isMobile: true,
      isDesktop: false,
      isHydrated: true,
    });

    const aMeta = deferred<Response>();
    let metaCount = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          metaCount += 1;
          return aMeta.promise;
        }
        if (input === contentUrl("/repo/a.ts")) return textResponse("content a");
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));

    // Back out while the read is still in flight.
    fireEvent.click(screen.getByLabelText("Back to files"));

    // The tree is reachable, with nothing covering it and nothing to dismiss.
    // (Backing out unmounts the editor view, tab strip and all, so the tree is
    // the whole screen and "a.ts" is unambiguous again.)
    expect(screen.getByText("b.ts")).toBeTruthy();
    expect(screen.queryByText("Cancel")).toBeNull();

    // The read was never cancelled, so it lands while the user is on the tree.
    aMeta.resolve(
      jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" }),
    );
    await waitFor(() => expect(metaCount).toBe(1));

    // Reopening shows it straight away, without reading the file a second time.
    fireEvent.click(screen.getByText("a.ts"));
    expect(await screen.findByText("content a")).toBeTruthy();
    expect(metaCount).toBe(1);
  });

  // The hard interleaving: both reads of the same path in flight at once. The
  // abandoned one must neither commit its bytes over the live one's nor clear
  // the live one's loading state when its own cleanup runs.
  it("a read abandoned by close cannot clobber a newer read of the same path", async () => {
    const firstMeta = deferred<Response>();
    const secondMeta = deferred<Response>();
    let metaCount = 0;
    let contentCount = 0;

    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string) => {
        if (input === metaUrl("/repo/a.ts")) {
          metaCount += 1;
          return metaCount === 1 ? firstMeta.promise : secondMeta.promise;
        }
        if (input === contentUrl("/repo/a.ts")) {
          contentCount += 1;
          return textResponse(`content a v${contentCount}`);
        }
        throw new Error(`unexpected fetch: ${input}`);
      }),
    );

    renderExplorer();

    // Read 1 starts and is left hanging.
    fireEvent.click(screen.getByText("a.ts"));
    expect(metaCount).toBe(1);

    // Close and immediately reopen, while read 1 is still pending. The reopen
    // must not be turned away as a duplicate of the read we just abandoned.
    fireEvent.click(getTab("a.ts").querySelector("button")!);
    await waitFor(() => expect(queryTab("a.ts")).toBeNull());
    fireEvent.click(screen.getByText("a.ts"));
    expect(metaCount).toBe(2);

    // Let read 1 run to completion — including its content fetch and its
    // finally — while read 2 is still pending.
    firstMeta.resolve(
      jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" }),
    );
    await waitFor(() => expect(contentCount).toBe(1));

    // Read 2 is still the live one: its tab is up and still loading, with
    // neither read 1's bytes nor the idle prompt showing.
    expect(queryTab("a.ts")).not.toBeNull();
    expect(screen.queryByText("content a v1")).toBeNull();
    expect(screen.queryByText("Select a file to edit")).toBeNull();
    expect(screen.queryByTestId("monaco")).toBeNull();

    // Read 2 finishes and its own bytes are what render.
    secondMeta.resolve(
      jsonResponse({ size: 11, isBinary: false, contentType: "text/plain", path: "/repo/a.ts" }),
    );
    expect(await screen.findByText("content a v2")).toBeTruthy();
    expect(screen.queryByText("content a v1")).toBeNull();
  });
});
