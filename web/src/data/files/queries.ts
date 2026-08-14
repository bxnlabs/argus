import { useCallback, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, apiFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import { getLanguageFromPath } from "@/lib/fileContent";
import type { FilesResponse, FileSearchResponse, FileViewResponse } from "@/types";
import { filesKeys } from "./keys";

export function useFilesQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  const { scope, baseUrl } = useActiveNode();
  return useQuery({
    queryKey: filesKeys.list(scope, path),
    queryFn: () =>
      apiFetch<FilesResponse>(baseUrl, `/api/node/files?path=${encodeURIComponent(path)}`),
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    staleTime: 10_000,
  });
}

export function useFileSearchQuery(
  query: string,
  options?: { enabled?: boolean; type?: string; limit?: number; searchPath?: string },
) {
  const { scope, baseUrl } = useActiveNode();
  const type = options?.type ?? "directory";
  const limit = options?.limit ?? 20;
  const searchPath = options?.searchPath;

  return useQuery({
    queryKey: filesKeys.search(scope, query, type, searchPath),
    queryFn: () => {
      const params = new URLSearchParams({
        q: query,
        type,
        limit: String(limit),
      });
      if (searchPath) {
        params.set("path", searchPath);
      }
      return apiFetch<FileSearchResponse>(baseUrl, `/api/node/files/search?${params}`);
    },
    enabled: (options?.enabled ?? true) && query.trim().length > 0,
    staleTime: 30_000,
  });
}

/**
 * Keep this read out of the browser's HTTP cache.
 *
 * The endpoint exists to observe a file changing underneath an open pane, and
 * a poll answered from the browser's own cache reports success while handing
 * back the bytes it already had — the exact staleness this query replaced the
 * buffer store to prevent. TanStack Query is the cache that matters here; the
 * browser's is pure downside. The cost is re-downloading an unchanged file,
 * bounded by the node's 5MB viewer ceiling.
 */
const NO_HTTP_CACHE: RequestInit = { cache: "no-store" };

/**
 * How often an open file re-reads itself.
 *
 * The pane exists to show what an agent did to a file, and remount and window
 * focus — which cover switching tabs and leaving the browser — do nothing for
 * a pane you are sitting and watching. TanStack does not poll a backgrounded
 * window, so this costs nothing while you are elsewhere, and only ever runs
 * for the file actually on screen: dockview unmounts hidden panels, which
 * takes their queries with them.
 *
 * A poll that finds the file untouched — the usual outcome — transfers no
 * content and does not even read the file on the node, because the request
 * carries the etag of the version already held.
 */
const FILE_POLL_MS = 30_000;

/**
 * Whether a failed read is the node's verdict rather than a passing blip.
 *
 * A 4xx means the node looked and answered: the file is gone (404), or is no
 * longer readable (403), or the path is no longer a regular file (400). That
 * is authoritative for this read — the cached bytes stop being worth showing —
 * though not permanent: a recreated file simply loads again on the next poll.
 *
 * Everything else — a 5xx, a dropped connection, a DNS failure — may well
 * succeed on the next refetch, and blanking a good pane over it is the worse
 * trade. These handlers emit no transient 4xx (no 408, no 429); if one ever
 * appears, this range is where it has to be excluded.
 */
export function isDefinitiveReadError(error: unknown): boolean {
  return error instanceof ApiError && error.status >= 400 && error.status < 500;
}

/** A file as the viewer needs it: the bytes, or why there are none. */
export interface FileContent {
  content: string;
  language: string;
  isBinary: boolean;
  isLarge: boolean;
  /**
   * Which version of the file this is, as the node names it. Carried in the
   * cache entry rather than in a side map so that returning the previous entry
   * on an unchanged poll also returns the etag to send with the next one, and
   * nothing has to be evicted alongside the query.
   */
  etag: string;
}

/**
 * One file's content, cached by absolute path.
 *
 * Keyed by path alone, not by session: the API returns absolute paths, so
 * sessions in different worktrees address different entries, and two sessions
 * sharing a worktree (which cloning produces) share one entry — correct for a
 * read, and one fetch instead of two.
 *
 * One request, because classification and bytes have to agree. Asking the node
 * how big a file is and then asking for its content are two looks at a path
 * that an agent can rewrite in between, which is how a file measured as small
 * text gets downloaded as something else entirely. The node now opens the file
 * once and answers both questions from that descriptor.
 *
 * Refetch-on-mount, refetch-on-focus, dedup and cache eviction all come from
 * the client defaults in `lib/query-client.ts`, and structural sharing keeps
 * the previous object when the bytes are identical, so an unchanged file
 * re-renders nothing — which is what makes a 30s poll invisible.
 */
export function useFileContentQuery(
  path: string,
  options?: { enabled?: boolean },
) {
  const { scope, baseUrl } = useActiveNode();
  const queryClient = useQueryClient();
  const queryKey = filesKeys.content(scope, path);

  // Set by `reload` and consumed by the next request, which is what makes the
  // reload button something a poll is not — see `reload` below.
  const forceRef = useRef(false);

  const query = useQuery({
    queryKey,
    queryFn: async (): Promise<FileContent> => {
      const force = forceRef.current;

      // Read the live cache rather than a render-time snapshot, so a poll
      // always validates against the newest version this client holds.
      const previous = queryClient.getQueryData<FileContent>(queryKey);

      const params = new URLSearchParams({ path });
      if (!force && previous?.etag) params.set("known", previous.etag);

      const view = await apiFetch<FileViewResponse>(
        baseUrl,
        `/api/node/files/content?${params}`,
        NO_HTTP_CACHE,
      );

      // Cleared here — after a body has actually arrived — and not where the
      // request was built. A reload is one user intent, but it can span
      // several requests: react-query serves `retry: 1` by calling this
      // function again, and a reload that exhausts its retries leaves the
      // 30s poll to try next. Clearing on the first attempt would put every
      // one of those back on the validator the reload exists to bypass, and
      // the node would answer `unchanged` against the very bytes the user
      // asked to replace — successfully, so the stale banner clears too.
      //
      // Only a request that consumed the flag may clear it. A poll already in
      // flight when the button is pressed read `force` as false, and letting
      // its response clear the flag would retire the reload before the request
      // it was set for was ever built — sending `known` on the retry, and
      // getting back the `unchanged` that the reload exists to escape.
      if (force) forceRef.current = false;

      if (view.unchanged) {
        // The same object, not an equal one. Reference identity is what keeps
        // Monaco untouched, and it is exact here rather than something
        // structural sharing has to rediscover by comparing the whole string.
        if (!previous) {
          // Unreachable: the node only answers `unchanged` to a `known` we
          // sent, and we only send one when there is an entry to return.
          throw new Error("node reported the file unchanged without a prior read");
        }
        return previous;
      }

      return {
        content: view.content,
        language: getLanguageFromPath(path),
        isBinary: view.isBinary,
        isLarge: view.isLarge,
        etag: view.etag,
      };
    },
    enabled: (options?.enabled ?? true) && path.trim().length > 0,
    refetchInterval: FILE_POLL_MS,
  });

  const { refetch } = query;

  /**
   * Re-read the file, ignoring the version this client already holds.
   *
   * The distinction from a plain `refetch` is the whole point. `ViewerETag` is
   * size and mtime, so a rewrite landing on both the same length and the same
   * nanosecond reads as unchanged — and a reload that re-sent that etag would
   * be answered `unchanged` forever, leaving the only escape hatch from that
   * blind spot no more able to open it than the poll that fell into it. Asking
   * unconditionally costs one re-transfer of a file the user explicitly asked
   * to see again.
   *
   * The flag it sets outlives this call: it is cleared by the first request
   * that comes back with a body, not by the first one that goes out.
   */
  const reload = useCallback(() => {
    forceRef.current = true;
    return refetch();
  }, [refetch]);

  return { ...query, reload };
}
