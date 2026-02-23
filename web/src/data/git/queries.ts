import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type {
  GitStatus,
  CommitSummary,
  CommitDetail,
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
        `/agent/api/git/check?path=${encodeURIComponent(path!)}`,
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

export function useGitStatusQuery(
  path: string,
  options?: { enabled?: boolean; refetchInterval?: number },
) {
  return useQuery({
    queryKey: gitKeys.status(path),
    queryFn: async () => {
      const data = await apiFetch<GitStatusResponse>(
        `/agent/api/git/status?path=${encodeURIComponent(path)}`,
      );
      return data.status;
    },
    staleTime: 5_000,
    refetchInterval: options?.refetchInterval ?? 5_000,
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
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
        `/agent/api/git/diff?${params}`,
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
        `/agent/api/git/history?path=${encodeURIComponent(path)}&limit=${limit}`,
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
        `/agent/api/git/history/${hash}?path=${encodeURIComponent(path)}`,
      );
      return data.commit;
    },
    staleTime: Infinity, // Commit details never change
    enabled: path.trim().length > 0 && !!hash,
  });
}

// --- Commit File Diff ---

export function useCommitFileDiffQuery(
  path: string,
  hash: string | null,
  file: string | null,
) {
  return useQuery({
    queryKey: gitKeys.commitFileDiff(path, hash ?? "", file ?? ""),
    queryFn: async () => {
      const data = await apiFetch<DiffResponse>(
        `/agent/api/git/history/${hash}/diff?path=${encodeURIComponent(path)}&file=${encodeURIComponent(file!)}`,
      );
      return data.diff ?? "";
    },
    staleTime: Infinity,
    enabled: path.trim().length > 0 && !!hash && !!file,
  });
}
