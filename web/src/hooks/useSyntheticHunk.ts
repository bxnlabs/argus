import { useQuery } from "@tanstack/react-query";
import { fetchFileLines } from "@/data/git/file-lines";

export interface SyntheticHunkLine {
  content: string;
  lineNumber: number;
}

// A synthetic hunk represents file content fetched directly from a git ref to
// host comments whose anchor isn't covered by any rendered diff hunk — either
// because the file is outside the compare diff, or because the comment lives
// outside the rendered hunks of an in-diff file (e.g. a comment placed via
// context expansion that has since rolled out of view).
export interface SyntheticHunk {
  file: string;
  ref: string;
  start: number;
  end: number;
  lines: SyntheticHunkLine[];
  /** Total lines in the file at this ref — required by ExpandableUnifiedDiff
   * to know whether downward expansion can still yield more lines. */
  totalLines: number;
}

const WINDOW = 10; // lines above and below the anchor band

export function useSyntheticHunk(params: {
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
    queryKey: ["syntheticHunk", path, file, ref, start, end],
    queryFn: async ({ signal }): Promise<SyntheticHunk> => {
      const result = await fetchFileLines({ path, file, ref, start, end, signal });
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
