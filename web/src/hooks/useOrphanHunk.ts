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
}

const WINDOW = 10; // lines above and below the anchor

export function useOrphanHunk(params: {
  path: string;
  file: string;
  ref: string;
  anchorLine: number;
  enabled?: boolean;
}) {
  const { path, file, ref, anchorLine } = params;
  const start = Math.max(1, anchorLine - WINDOW);
  const end = anchorLine + WINDOW;
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
      };
    },
    enabled: (params.enabled ?? true) && anchorLine >= 1 && !!ref && !!file,
    staleTime: 60_000,
  });
}
