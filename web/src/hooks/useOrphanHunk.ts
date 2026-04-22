import { useQuery } from "@tanstack/react-query";
import { fetchFileLines } from "@/data/git/file-lines";

export interface OrphanHunkLine {
  content: string;
  lineNumber: number;
}

export interface OrphanHunk {
  file: string;
  ref: string;
  start: number;
  end: number;
  lines: OrphanHunkLine[];
  /** Total lines in the file at this ref — required by ExpandableUnifiedDiff
   * to know whether downward expansion can still yield more lines. */
  totalLines: number;
}

const WINDOW = 10; // lines above and below the anchor band

export function useOrphanHunk(params: {
  path: string;
  file: string;
  ref: string;
  /** Smallest anchor line in the group. */
  startLine: number;
  /** Largest anchor line in the group. Pass the same value as startLine for a
   * single-anchor group; the fetched window is [start - WINDOW, end + WINDOW]
   * so the resulting hunk covers every anchor in the group. */
  endLine: number;
  enabled?: boolean;
}) {
  const { path, file, ref, startLine, endLine } = params;
  const start = Math.max(1, startLine - WINDOW);
  const end = endLine + WINDOW;
  return useQuery({
    queryKey: ["orphanHunk", path, file, ref, start, end],
    queryFn: async (): Promise<OrphanHunk> => {
      const result = await fetchFileLines({ path, file, ref, start, end });
      return {
        file,
        ref,
        start: result.start,
        end: result.end,
        lines: result.lines.map((content, i) => ({
          content,
          lineNumber: result.start + i,
        })),
        totalLines: result.totalLines,
      };
    },
    enabled: (params.enabled ?? true) && startLine >= 1 && endLine >= startLine && !!ref && !!file,
    staleTime: 60_000,
  });
}
