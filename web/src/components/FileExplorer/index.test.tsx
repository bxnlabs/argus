import { describe, it, expect, vi, afterEach, beforeAll, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { FileExplorer } from "./index";
import { StubNodeProvider } from "@/test/node-context";
import { useFilesQuery, useFileContentQuery } from "@/data/files";
import type { FileContent } from "@/data/files";
import { apiFetch, ApiError } from "@/api/client";
import { useViewport } from "@/hooks/useViewport";

// FileExplorer's job is the same wiring EditorCenter has, plus which of the
// two top-level views (tree vs. editor) is showing. Mock the queries so these
// tests exercise only that wiring, not fetching (covered in contentQuery.test.tsx).
vi.mock("@/data/files", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/data/files")>()),
  useFilesQuery: vi.fn(),
  useFileContentQuery: vi.fn(),
}));

// This branch-sourced test file predates PR 13, when FileExplorer still
// serves desktop too (a resizable two-pane layout) alongside the mobile
// drill-in it was written against. Every test here but one is layout-agnostic
// and runs against the desktop default set in beforeEach below. The one
// exception — "keeps directory expansion state after opening a file and going
// back" — exercises the mobile-only "Back to files" control, so it opts into
// the mobile branch explicitly.
vi.mock("@/hooks/useViewport", () => ({
  useViewport: vi.fn(),
}));

// FileTree (real, unmocked) fetches a directory's children directly through
// apiFetch on expand — needed for the tree-survives-drill-in test below.
vi.mock("@/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/client")>()),
  apiFetch: vi.fn(),
}));

// Mirrors EditorCenter/index.test.tsx's stub: enough of FileEditor's own
// branching to assert content reached it, without pulling in Monaco.
vi.mock("./FileEditor", () => ({
  FileEditor: ({ content }: { content: string }) => (
    <div data-testid="file-editor">
      <span data-testid="editor-content">{content}</span>
    </div>
  ),
}));

function Wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

function makeContent(overrides: Partial<FileContent> = {}): FileContent {
  return {
    content: "hello",
    language: "typescript",
    isBinary: false,
    isLarge: false,
    etag: "5-1000",
    ...overrides,
  };
}

const FILES_STUB = {
  data: { files: [{ name: "a.ts", path: "/repo/a.ts", type: "file", size: 10 }] },
  isPending: false,
  isError: false,
  error: undefined,
  isRefetching: false,
  refetch: vi.fn(),
};

function renderExplorer() {
  return render(<FileExplorer workingDirectory="/repo" />, { wrapper: Wrapper });
}

beforeAll(() => {
  // FileTabs scrolls the active tab into view; jsdom has no layout engine.
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

beforeEach(() => {
  // Reset to the desktop default before every test. clearAllMocks (below)
  // only clears call history, not a mockReturnValue — so without this, a
  // test that overrides useViewport would leak that override into whatever
  // runs after it, rather than just the one test that needs it.
  vi.mocked(useViewport).mockReturnValue({
    isMobile: false,
    isDesktop: true,
    isHydrated: true,
  });
});

afterEach(() => {
  vi.clearAllMocks();
  cleanup();
});

describe("FileExplorer", () => {
  // The tree panel is shared with FileTreeSidebar. Before it was shared, this
  // copy of the header had no accessible name on its refresh button while the
  // sidebar's did — the exact drift a second copy invites.
  it("gives the tree's refresh control an accessible name", () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: false,
      data: undefined,
      error: undefined,
    } as never);

    renderExplorer();
    expect(screen.getByRole("button", { name: "Refresh files" })).toBeTruthy();
  });

  it("shows a spinner while the opened file is pending, then its content once loaded", async () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: true,
      isError: false,
      data: undefined,
      error: undefined,
    } as never);

    const { rerender } = renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));

    expect(screen.getByTestId("file-loading")).toBeTruthy();
    expect(screen.queryByTestId("file-editor")).toBeNull();

    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: false,
      data: makeContent(),
      error: undefined,
    } as never);
    rerender(<FileExplorer workingDirectory="/repo" />);

    expect(await screen.findByTestId("file-editor")).toBeTruthy();
    expect(screen.getByTestId("editor-content").textContent).toBe("hello");
  });

  it("shows the real error message when a file fails to load and there is no content to fall back on", () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: true,
      data: undefined,
      error: new Error("permission denied"),
    } as never);

    renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));

    expect(screen.getByText("permission denied")).toBeTruthy();
    expect(screen.queryByTestId("file-editor")).toBeNull();
  });

  it("falls back to a generic message when the error carries none", () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: true,
      data: undefined,
      error: undefined,
    } as never);

    renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));

    expect(screen.getByText("Failed to open file")).toBeTruthy();
  });

  // Stale content wins over a blip (matches EditorCenter): a transient
  // refetch failure on an already-open file must not blank the pane or claim
  // failure. Reverting either the FileExplorer gating or its placement back
  // under the old hidden-tree overlay would make the error text reappear
  // here even though good content is on screen.
  it("keeps showing stale content when a refetch fails transiently", async () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: false,
      data: makeContent(),
      error: undefined,
    } as never);

    const { rerender } = renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));
    expect(await screen.findByTestId("file-editor")).toBeTruthy();

    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: true,
      data: makeContent(),
      error: new Error("Failed to fetch"),
    } as never);
    rerender(<FileExplorer workingDirectory="/repo" />);

    expect(screen.getByTestId("file-editor")).toBeTruthy();
    expect(screen.getByTestId("editor-content").textContent).toBe("hello");
    expect(screen.queryByText("Failed to fetch")).toBeNull();
    expect(screen.queryByText("Failed to open file")).toBeNull();
  });

  // Stale content winning (above) must not read as "still fresh" — a pane
  // stuck failing every poll and every reload looks identical to a current
  // one unless something on screen says otherwise.
  it("says refreshing has stopped once a poll fails, instead of going stale silently", async () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: false,
      data: makeContent(),
      error: undefined,
    } as never);

    const { rerender } = renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));
    expect(await screen.findByTestId("file-editor")).toBeTruthy();

    expect(screen.queryByRole("status")).toBeNull();

    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: true,
      data: makeContent(),
      error: new Error("Failed to fetch"),
    } as never);
    rerender(<FileExplorer workingDirectory="/repo" />);

    const banner = screen.getByRole("status");
    expect(banner.textContent).toContain("Showing the last version read — refresh failed");
    expect(banner.textContent).toContain("Failed to fetch");
  });

  // ...but not over a verdict. Same rule as EditorCenter: a 4xx means the
  // node looked and the file is gone or unreadable, so the cached bytes stop
  // being something worth showing.
  it("replaces stale content when the node says the file is gone", async () => {
    vi.mocked(useFilesQuery).mockReturnValue(FILES_STUB as never);
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: false,
      data: makeContent(),
      error: undefined,
    } as never);

    const { rerender } = renderExplorer();
    fireEvent.click(screen.getByText("a.ts"));
    expect(await screen.findByTestId("file-editor")).toBeTruthy();

    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: true,
      data: makeContent(),
      error: new ApiError("file not found", 404),
    } as never);
    rerender(<FileExplorer workingDirectory="/repo" />);

    expect(screen.getByText("file not found")).toBeTruthy();
    expect(screen.queryByTestId("file-editor")).toBeNull();
  });

  // The tree must stay mounted (hidden, not unmounted) behind the editor
  // view: FileTree keeps expand/collapse state in local useState, so
  // unmounting it on drill-in would collapse everything and refire a fetch
  // per folder on every return to the tree.
  it("keeps directory expansion state after opening a file and going back", async () => {
    // Only this test exercises the mobile "Back to files" control — see the
    // useViewport mock note above.
    vi.mocked(useViewport).mockReturnValue({
      isMobile: true,
      isDesktop: false,
      isHydrated: true,
    });
    vi.mocked(useFilesQuery).mockReturnValue({
      ...FILES_STUB,
      data: {
        files: [
          { name: "src", path: "/repo/src", type: "directory" },
          { name: "a.ts", path: "/repo/a.ts", type: "file", size: 10 },
        ],
      },
    } as never);
    vi.mocked(apiFetch).mockResolvedValue({
      files: [{ name: "index.ts", path: "/repo/src/index.ts", type: "file", size: 5 }],
    });
    vi.mocked(useFileContentQuery).mockReturnValue({
      isPending: false,
      isError: false,
      data: makeContent(),
      error: undefined,
    } as never);

    renderExplorer();

    fireEvent.click(screen.getByText("src"));
    expect(await screen.findByText("index.ts")).toBeTruthy();

    fireEvent.click(screen.getByText("a.ts"));
    expect(await screen.findByTestId("file-editor")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Back to files" }));

    expect(screen.getByText("index.ts")).toBeTruthy();
    // Only the one apiFetch call from the original expand — going back did
    // not need to refetch the directory it had already loaded.
    expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(1);
  });
});
