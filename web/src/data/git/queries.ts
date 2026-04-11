import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type {
  GitStatus,
  CommitSummary,
  CommitDetail,
  CompareResult,
  CommitFullDiffResult,
  BranchList,
  WorkingDiffResult,
} from "@/types";
import { gitKeys } from "./keys";

// --- Check (is git repo?) ---

interface GitCheckResponse {
  isGitRepo: boolean;
}

export function useGitCheckQuery(path: string | null) {
  return useQuery({
    queryKey: gitKeys.check(path ?? ""),
    queryFn: async () => {
      const data = await apiFetch<GitCheckResponse>(
        `/node/api/git/check?path=${encodeURIComponent(path!)}`,
      );
      return data.isGitRepo;
    },
    staleTime: 30_000,
    enabled: !!path && path.trim().length > 0,
  });
}

// --- Status ---

interface GitStatusResponse {
  status: GitStatus;
}

export function useGitStatusQuery<TData = GitStatus>(
  path: string,
  options?: {
    enabled?: boolean;
    refetchInterval?: number | false;
    select?: (data: GitStatus) => TData;
    notifyOnChangeProps?: Array<"data" | "error" | "isPending" | "isError" | "isRefetching">;
  },
) {
  return useQuery<GitStatus, Error, TData>({
    queryKey: gitKeys.status(path),
    queryFn: async () => {
      const data = await apiFetch<GitStatusResponse>(
        `/node/api/git/status?path=${encodeURIComponent(path)}`,
      );
      return data.status;
    },
    staleTime: 5_000,
    refetchInterval: options?.refetchInterval ?? 5_000,
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    select: options?.select,
    notifyOnChangeProps: options?.notifyOnChangeProps,
  });
}

// --- File Diff (current changes) ---

interface DiffResponse {
  diff: string;
}

export function useFileDiffQuery(
  path: string,
  file: string,
  options?: { staged?: boolean; untracked?: boolean; enabled?: boolean },
) {
  const staged = options?.staged ?? false;
  const untracked = options?.untracked ?? false;

  return useQuery({
    queryKey: gitKeys.fileDiff(path, file, staged, untracked),
    queryFn: async () => {
      const params = new URLSearchParams({ path, file });
      if (staged) params.set("staged", "true");
      if (untracked) params.set("untracked", "true");
      const data = await apiFetch<DiffResponse>(
        `/node/api/git/diff?${params}`,
      );
      return data.diff;
    },
    staleTime: 5_000,
    enabled:
      (options?.enabled ?? true) &&
      path.trim().length > 0 &&
      file.trim().length > 0,
  });
}

// --- History ---

interface HistoryResponse {
  commits: CommitSummary[];
}

export function useGitHistoryQuery(
  path: string,
  options?: { limit?: number; enabled?: boolean },
) {
  const limit = options?.limit ?? 30;

  return useQuery({
    queryKey: gitKeys.history(path),
    queryFn: async () => {
      const data = await apiFetch<HistoryResponse>(
        `/node/api/git/history?path=${encodeURIComponent(path)}&limit=${limit}`,
      );
      return data.commits ?? [];
    },
    staleTime: 30_000,
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
  });
}

// --- Commit Detail ---

interface CommitDetailResponse {
  commit: CommitDetail;
}

export function useCommitDetailQuery(
  path: string,
  hash: string | null,
) {
  return useQuery({
    queryKey: gitKeys.commitDetail(path, hash ?? ""),
    queryFn: async () => {
      const data = await apiFetch<CommitDetailResponse>(
        `/node/api/git/history/${hash}?path=${encodeURIComponent(path)}`,
      );
      return data.commit;
    },
    staleTime: Infinity, // Commit details never change
    enabled: path.trim().length > 0 && !!hash,
  });
}

// --- Compare Branches ---

export function useCompareBranchesQuery(path: string) {
  return useQuery({
    queryKey: gitKeys.compareBranches(path),
    queryFn: async () => {
      const data = await apiFetch<BranchList>(
        `/node/api/git/compare/branches?path=${encodeURIComponent(path)}`,
      );
      return data;
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0,
  });
}

// --- Compare ---

export function useCompareQuery(path: string, base: string | null) {
  return useQuery({
    queryKey: gitKeys.compare(path, base ?? ""),
    queryFn: async () => {
      const data = await apiFetch<CompareResult>(
        `/node/api/git/compare?path=${encodeURIComponent(path)}&base=${encodeURIComponent(base!)}`,
      );
      return data;
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!base,
  });
}

// --- Commit Full Diff ---

export function useCommitFullDiffQuery(path: string, hash: string | null) {
  return useQuery({
    queryKey: gitKeys.commitFullDiff(path, hash ?? ""),
    queryFn: async () => {
      return apiFetch<CommitFullDiffResult>(
        `/node/api/git/history/${hash}/full-diff?path=${encodeURIComponent(path)}`,
      );
    },
    staleTime: Infinity,
    enabled: path.trim().length > 0 && !!hash,
  });
}

// --- Branches (for session creation) ---

interface BranchesResponse {
  branches: string[];
}

export function useBranchesQuery(
  source: string,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: gitKeys.branches(source),
    queryFn: async () => {
      const data = await apiFetch<BranchesResponse>(
        `/node/api/git/branches?source=${encodeURIComponent(source)}`,
      );
      return data.branches ?? [];
    },
    staleTime: 30_000,
    enabled:
      (options?.enabled ?? true) && source.trim().length > 0,
  });
}

// --- Working Diff (full working-tree diff) ---

export function useWorkingDiffQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: gitKeys.workingDiff(path),
    queryFn: async () => {
      const data = await apiFetch<WorkingDiffResult>(
        `/node/api/git/working-diff?path=${encodeURIComponent(path)}`,
      );
      return data;
    },
    staleTime: 5_000,
    refetchInterval: 5_000,
    enabled: path.trim().length > 0 && (options?.enabled ?? true),
  });
}
