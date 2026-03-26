// web/src/data/git/hooks.ts
import type { GitStatus } from "@/types";
import { useGitStatusQuery } from "./queries";

// --- Module-scoped selectors and notify arrays (stable identity across renders) ---

const selectSummary = (s: GitStatus) => ({ branch: s.branch, ahead: s.ahead, behind: s.behind });
const selectBranch = (s: GitStatus) => s.branch;
const selectFiles = (s: GitStatus) => ({ staged: s.staged, unstaged: s.unstaged, untracked: s.untracked });

const NOTIFY_WITH_REFETCHING: Array<"data" | "error" | "isPending" | "isError" | "isRefetching"> =
  ["data", "error", "isPending", "isError", "isRefetching"];
const NOTIFY_DATA_ONLY: Array<"data" | "error" | "isPending" | "isError"> =
  ["data", "error", "isPending", "isError"];

/**
 * Branch summary for the header bar.
 * Subscribes to isRefetching so the refresh spinner works.
 */
export function useGitStatusSummaryQuery(path: string) {
  return useGitStatusQuery(path, {
    select: selectSummary,
    notifyOnChangeProps: NOTIFY_WITH_REFETCHING,
  });
}

/**
 * Current branch name only.
 * Does NOT subscribe to isRefetching — compare tab uses this.
 */
export function useGitCurrentBranchQuery(path: string) {
  return useGitStatusQuery(path, {
    select: selectBranch,
    notifyOnChangeProps: NOTIFY_DATA_ONLY,
  });
}

/**
 * File lists for the changes tab.
 * Does NOT subscribe to isRefetching.
 */
export function useGitStatusFilesQuery(path: string) {
  return useGitStatusQuery(path, {
    select: selectFiles,
    notifyOnChangeProps: NOTIFY_DATA_ONLY,
  });
}
