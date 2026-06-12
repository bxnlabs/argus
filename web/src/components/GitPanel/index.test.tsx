import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { ReactNode } from "react";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { GitPanel } from "./index";
import { StubNodeProvider } from "@/test/node-context";
import type { GitTabRequest } from "./GitPanelTabs";
import {
  useGitCurrentBranchQuery,
  useGitStatusFilesQuery,
  useWorkingDiffQuery,
  useGitFetchMutation,
} from "@/data/git";
import { useViewport } from "@/hooks/useViewport";

// Exercise the real App -> GitPanel sync path (the chord `requestedTab` event ->
// activeTab) without the data layer. The git queries and viewport are mocked,
// and the heavy tab bodies are stubbed — CommitHistory/CompareView render their
// `header` (which carries the real GitPanelTabs) so the tabs stay clickable in
// every tab state.
vi.mock("@/data/git", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/data/git")>()),
  useGitCurrentBranchQuery: vi.fn(),
  useGitStatusFilesQuery: vi.fn(),
  useWorkingDiffQuery: vi.fn(),
  useGitFetchMutation: vi.fn(),
}));

vi.mock("@/hooks/useViewport", () => ({ useViewport: vi.fn() }));

vi.mock("./CommitHistory", () => ({
  CommitHistory: ({ header }: { header?: ReactNode }) => (
    <div data-testid="history-body">{header}</div>
  ),
}));
vi.mock("./CompareView", () => ({
  CompareView: ({ header }: { header?: ReactNode }) => (
    <div data-testid="compare-body">{header}</div>
  ),
}));
vi.mock("./GitStatusHeader", () => ({
  GitStatusHeader: () => <div data-testid="git-header" />,
}));
vi.mock("./FileChanges", () => ({
  FileChanges: () => <div data-testid="file-changes" />,
}));
vi.mock("@/components/DiffViewer/ExpandableUnifiedDiff", () => ({
  ExpandableUnifiedDiff: () => <div data-testid="expandable-diff" />,
}));

/** Assert which GitPanelTabs tab is the active one (the source of truth for
 *  GitPanel's activeTab) via its `aria-selected` state. */
function expectActiveTab(name: "Changes" | "History" | "Compare") {
  expect(screen.getByRole("tab", { name }).getAttribute("aria-selected")).toBe(
    "true",
  );
}

function renderPanel(initial: GitTabRequest) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const ui = (req: GitTabRequest) => (
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <GitPanel workingDirectory="/repo" requestedTab={req} />
      </StubNodeProvider>
    </QueryClientProvider>
  );
  const utils = render(ui(initial));
  return {
    ...utils,
    rerenderWith: (req: GitTabRequest) => utils.rerender(ui(req)),
  };
}

beforeEach(() => {
  vi.mocked(useViewport).mockReturnValue({
    isMobile: false,
    isDesktop: true,
    isHydrated: true,
  });
  vi.mocked(useGitCurrentBranchQuery).mockReturnValue({
    data: "main",
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useGitCurrentBranchQuery>);
  vi.mocked(useGitStatusFilesQuery).mockReturnValue({
    data: { staged: [], unstaged: [], untracked: [] },
    isPending: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useGitStatusFilesQuery>);
  vi.mocked(useWorkingDiffQuery).mockReturnValue({
    data: { diff: "", files: [], totalLines: {} },
    isLoading: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  } as unknown as ReturnType<typeof useWorkingDiffQuery>);
  vi.mocked(useGitFetchMutation).mockReturnValue({
    mutate: vi.fn(),
    isPending: false,
  } as unknown as ReturnType<typeof useGitFetchMutation>);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("GitPanel — requestedTab sync", () => {
  it("re-applies the same requested tab after a manual switch (new request)", () => {
    // This is the regression the seq token fixes: while the panel stays mounted,
    // repeating the same chord (a fresh request object with the same tab) must
    // re-navigate even after the user manually switched away.
    const { rerenderWith } = renderPanel({ tab: "history", seq: 0 });
    expectActiveTab("History");
    expect(screen.getByTestId("history-body")).toBeTruthy();

    // User manually switches to Changes.
    fireEvent.click(screen.getByRole("tab", { name: "Changes" }));
    expectActiveTab("Changes");
    expect(screen.queryByTestId("history-body")).toBeNull();

    // Same chord fires again -> new request object, same tab, bumped seq.
    rerenderWith({ tab: "history", seq: 1 });
    expectActiveTab("History");
    expect(screen.getByTestId("history-body")).toBeTruthy();
  });

  it("does not override a manual switch on a re-render without a new request", () => {
    // Guards against over-firing: an unrelated parent re-render passes the SAME
    // request object identity, so the sync effect must not run and clobber the
    // user's manual tab selection.
    const sameRequest: GitTabRequest = { tab: "history", seq: 0 };
    const { rerenderWith } = renderPanel(sameRequest);
    expectActiveTab("History");

    fireEvent.click(screen.getByRole("tab", { name: "Changes" }));
    expectActiveTab("Changes");

    // Re-render with the identical request reference (no new chord).
    rerenderWith(sameRequest);
    expectActiveTab("Changes");
  });
});
