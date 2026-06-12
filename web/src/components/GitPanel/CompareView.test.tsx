import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, screen, cleanup, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { CompareView } from "./CompareView";
import { StubNodeProvider } from "@/test/node-context";
import {
  useCompareBranchesQuery,
  useCompareQuery,
  useGitCurrentBranchQuery,
} from "@/data/git";
import { useReviewQuery, useSaveReviewMutation } from "@/data/review";
import { useViewport } from "@/hooks/useViewport";
import type { CompareResult, ReviewComment } from "@/types";

// Drive CompareView through the loading / error / loaded compare states the
// clear-menu gating depends on by mocking the data layer and viewport. Sibling
// presentational children are stubbed so the test isolates CompareView's own
// branching logic — each child is covered by its own tests.
vi.mock("@/data/git", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/data/git")>()),
  useCompareBranchesQuery: vi.fn(),
  useCompareQuery: vi.fn(),
  useGitCurrentBranchQuery: vi.fn(),
}));

vi.mock("@/data/review", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/data/review")>()),
  useReviewQuery: vi.fn(),
  useSaveReviewMutation: vi.fn(),
}));

vi.mock("@/hooks/useViewport", () => ({ useViewport: vi.fn() }));
vi.mock("@/hooks/useScrollToFileCorrection", () => ({
  useScrollToFileCorrection: vi.fn(),
}));

// The fix lives in how CompareView derives the clear counts (and whether the
// menu renders at all in a given branch), not in the menu's own rendering — so
// the stub exposes the counts it was handed as data attributes.
vi.mock("./ClearCommentsMenu", () => ({
  ClearCommentsMenu: ({ counts }: { counts: Record<string, number> }) => (
    <div
      data-testid="clear-menu"
      data-all={counts.all}
      data-pending={counts.pending}
      data-submitted={counts.submitted}
      data-unanchored={counts.unanchored}
    />
  ),
}));

vi.mock("./ReviewSubmitButton", () => ({
  ReviewSubmitButton: () => <div data-testid="review-submit" />,
}));
vi.mock("./CommentNav", () => ({ CommentNav: () => <div data-testid="comment-nav" /> }));
vi.mock("./MobileCommentSheet", () => ({
  MobileCommentSheet: () => <div data-testid="mobile-sheet" />,
}));
vi.mock("./ReviewBodyCard", () => ({ ReviewBodyCard: () => <div data-testid="review-body" /> }));
vi.mock("./UnanchoredCommentSection", () => ({
  UnanchoredCommentSection: () => <div data-testid="unanchored-section" />,
}));

function makeComment(id: string, submitted: boolean): ReviewComment {
  return {
    id,
    file: "src/app.ts",
    line: { from: { side: "R", line: 10 }, to: { side: "R", line: 10 } },
    snippet: "",
    body: "x",
    submitted,
    createdAt: "2026-05-23T00:00:00Z",
  };
}

// 1 pending + 2 submitted = 3 total.
const COMMENTS: ReviewComment[] = [
  makeComment("c1", false),
  makeComment("c2", true),
  makeComment("c3", true),
];

// An empty-diff compare result: the request succeeded but there are no changed
// files, so every comment partitions as unanchored (no candidate diff). Used as
// the "compare data is present" baseline.
function loadedEmptyCompare(): CompareResult {
  return {
    diff: "",
    files: [],
    totalLines: {},
    totalAdditions: 0,
    totalDeletions: 0,
    baseRef: "base-sha",
    headRef: "head-sha",
    baseUpstream: "origin/main",
    baseBehindBy: 0,
  };
}

function mockCompare(state: {
  data?: CompareResult;
  isLoading?: boolean;
  isError?: boolean;
}) {
  vi.mocked(useCompareQuery).mockReturnValue({
    data: state.data,
    isLoading: state.isLoading ?? false,
    isError: state.isError ?? false,
    error: state.isError ? new Error("compare failed") : null,
  } as unknown as ReturnType<typeof useCompareQuery>);
}

function renderView() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <CompareView workingDirectory="/repo" />
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
  vi.mocked(useCompareBranchesQuery).mockReturnValue({
    data: { branches: ["main", "feature"], defaultBase: "main" },
    isLoading: false,
    isError: false,
    error: null,
  } as unknown as ReturnType<typeof useCompareBranchesQuery>);
  vi.mocked(useGitCurrentBranchQuery).mockReturnValue({
    data: "feature",
  } as unknown as ReturnType<typeof useGitCurrentBranchQuery>);
  vi.mocked(useReviewQuery).mockReturnValue({
    data: { head: "feature", base: "main", comments: COMMENTS },
  } as unknown as ReturnType<typeof useReviewQuery>);
  vi.mocked(useSaveReviewMutation).mockReturnValue({
    mutate: vi.fn(),
  } as unknown as ReturnType<typeof useSaveReviewMutation>);
  // Default: compare loaded successfully (empty diff).
  mockCompare({ data: loadedEmptyCompare() });
});

afterEach(() => {
  cleanup();
});

describe("CompareView — unanchored clear-count gating", () => {
  it("does not classify comments as unanchored while compare data is loading", async () => {
    mockCompare({ data: undefined, isLoading: true });
    renderView();

    const menu = await screen.findByTestId("clear-menu");
    // The submission-state categories still work without compare data...
    expect(menu.getAttribute("data-all")).toBe("3");
    expect(menu.getAttribute("data-pending")).toBe("1");
    expect(menu.getAttribute("data-submitted")).toBe("2");
    // ...but "Clear unanchored" must not silently mean "Clear all" just because
    // the diff hasn't loaded and every comment partitions as unanchored.
    expect(menu.getAttribute("data-unanchored")).toBe("0");
  });

  it("does not classify comments as unanchored when the compare query errors", async () => {
    mockCompare({ data: undefined, isError: true });
    renderView();

    const menu = await screen.findByTestId("clear-menu");
    expect(menu.getAttribute("data-all")).toBe("3");
    expect(menu.getAttribute("data-unanchored")).toBe("0");
  });

  it("does not trust stale compare data retained behind an errored refetch", async () => {
    // react-query keeps the last successful data while surfacing isError on a
    // same-key refetch failure — classification off that stale diff isn't
    // trustworthy, so the unanchored bucket stays empty.
    mockCompare({ data: loadedEmptyCompare(), isError: true });
    renderView();

    const menu = await screen.findByTestId("clear-menu");
    // Only the unanchored category is suppressed; the submission-state
    // categories stay live off the (untrusted-for-classification) comment list.
    expect(menu.getAttribute("data-unanchored")).toBe("0");
    expect(menu.getAttribute("data-all")).toBe("3");
    expect(menu.getAttribute("data-pending")).toBe("1");
    expect(menu.getAttribute("data-submitted")).toBe("2");
  });

  it("classifies unanchored comments once compare data has loaded", async () => {
    // Successful compare with no changed files: every comment is genuinely
    // unanchored, so the count reflects the real partition (here, all of them).
    mockCompare({ data: loadedEmptyCompare() });
    renderView();

    const menu = await screen.findByTestId("clear-menu");
    expect(menu.getAttribute("data-unanchored")).toBe("3");
  });
});

describe("CompareView — mobile clear-menu reachability", () => {
  beforeEach(() => {
    vi.mocked(useViewport).mockReturnValue({
      isMobile: true,
      isDesktop: false,
      isHydrated: true,
    });
  });

  it("surfaces the clear menu in the unanchored-only view with no changed files", async () => {
    // No file rows render in this state, so the diff-view toolbar (the menu's
    // only other mobile home) is unreachable — the menu must appear here.
    mockCompare({ data: loadedEmptyCompare() });
    renderView();

    expect(await screen.findByTestId("unanchored-section")).toBeTruthy();
    expect(screen.getByTestId("clear-menu")).toBeTruthy();
  });

  it("omits the clear menu when there are no comments to clear", async () => {
    vi.mocked(useReviewQuery).mockReturnValue({
      data: { head: "feature", base: "main", comments: [] },
    } as unknown as ReturnType<typeof useReviewQuery>);
    mockCompare({ data: loadedEmptyCompare() });
    renderView();

    await waitFor(() => {
      expect(screen.queryByTestId("clear-menu")).toBeNull();
    });
  });
});
