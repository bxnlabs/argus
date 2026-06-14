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

    expect(screen.getByText("src")).toBeTruthy();
    expect(screen.getByText("README.md")).toBeTruthy();
    // Not the idle prompt — the directory is being shown.
    expect(screen.queryByText("Looking for something?")).toBeNull();
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

    expect(screen.getByText("..")).toBeTruthy();
    expect(screen.queryByText("Looking for something?")).toBeNull();
  });

  it("falls back to the idle prompt when no searchPath is provided", () => {
    renderBrowser({});

    expect(screen.getByText("Looking for something?")).toBeTruthy();
  });
});
