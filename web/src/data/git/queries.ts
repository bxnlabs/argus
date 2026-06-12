import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
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
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.check(scope, path ?? ""),
    queryFn: async () => {
      const data = await apiFetch<GitCheckResponse>(
        baseUrl,
        `/api/node/git/check?path=${encodeURIComponent(path!)}`,
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
  const { scope, baseUrl } = useActiveNode();
  return useQuery<GitStatus, Error, TData>({
    queryKey: gitKeys.status(scope, path),
    queryFn: async () => {
      const data = await apiFetch<GitStatusResponse>(
        baseUrl,
        `/api/node/git/status?path=${encodeURIComponent(path)}`,
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
  const { scope, baseUrl } = useActiveNode();
  const staged = options?.staged ?? false;
  const untracked = options?.untracked ?? false;

  return useQuery({
    queryKey: gitKeys.fileDiff(scope, path, file, staged, untracked),
    queryFn: async () => {
      const params = new URLSearchParams({ path, file });
      if (staged) params.set("staged", "true");
      if (untracked) params.set("untracked", "true");
      const data = await apiFetch<DiffResponse>(
        baseUrl,
        `/api/node/git/diff?${params}`,
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
  const { scope, baseUrl } = useActiveNode();
  const limit = options?.limit ?? 30;

  return useQuery({
    queryKey: gitKeys.history(scope, path),
    queryFn: async () => {
      const data = await apiFetch<HistoryResponse>(
        baseUrl,
        `/api/node/git/history?path=${encodeURIComponent(path)}&limit=${limit}`,
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
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.commitDetail(scope, path, hash ?? ""),
    queryFn: async () => {
      const data = await apiFetch<CommitDetailResponse>(
        baseUrl,
        `/api/node/git/history/${hash}?path=${encodeURIComponent(path)}`,
      );
      return data.commit;
    },
    staleTime: Infinity, // Commit details never change
    enabled: path.trim().length > 0 && !!hash,
  });
}

// --- Compare Branches ---

export function useCompareBranchesQuery(path: string) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.compareBranches(scope, path),
    queryFn: async () => {
      const data = await apiFetch<BranchList>(
        baseUrl,
        `/api/node/git/compare/branches?path=${encodeURIComponent(path)}`,
      );
      return data;
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0,
  });
}

// --- Compare ---

export function useCompareQuery(path: string, base: string | null) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.compare(scope, path, base ?? ""),
    queryFn: async () => {
      const params = new URLSearchParams({
        path,
        base: base!,
      });
      return apiFetch<CompareResult>(baseUrl, `/api/node/git/compare?${params}`);
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!base,
  });
}

// --- Commit Full Diff ---

export function useCommitFullDiffQuery(path: string, hash: string | null) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.commitFullDiff(scope, path, hash ?? ""),
    queryFn: async () => {
      return apiFetch<CommitFullDiffResult>(
        baseUrl,
        `/api/node/git/history/${hash}/full-diff?path=${encodeURIComponent(path)}`,
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
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.branches(scope, source),
    queryFn: async () => {
      const data = await apiFetch<BranchesResponse>(
        baseUrl,
        `/api/node/git/branches?source=${encodeURIComponent(source)}`,
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
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: gitKeys.workingDiff(scope, path),
    queryFn: async () => {
      const data = await apiFetch<WorkingDiffResult>(
        baseUrl,
        `/api/node/git/working-diff?path=${encodeURIComponent(path)}`,
      );
      return data;
    },
    staleTime: 5_000,
    refetchInterval: 5_000,
    enabled: path.trim().length > 0 && (options?.enabled ?? true),
  });
}
