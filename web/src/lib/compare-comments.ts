import type { CompareFileView } from "@/types";
import type { ReviewComment } from "@/types/review";

/**
 * Returns comments in visual rendering order — the order in which they appear
 * top-to-bottom in the compare view DOM. Walks files in array order, hunks in
 * array order, lines in array order, and emits each comment at its anchor row.
 *
 * This sort is robust to duplicate path entries in `files` (real diff +
 * synthetic snippet FileView for the same path) because file array position is
 * the primary ordering key, not the file path.
 */
export function sortCommentsByRenderOrder(
  files: readonly CompareFileView[],
  comments: readonly ReviewComment[],
): ReviewComment[] {
  if (files.length === 0 || comments.length === 0) return [];

  const keyOf = (path: string, side: "L" | "R", line: number) =>
    `${path}|${side}|${line}`;

  // Pre-index comments by anchor key. L-side anchors against the pre-rename
  // path (oldPath ?? file); R-side anchors against the post-rename path (file).
  const byAnchor = new Map<string, ReviewComment[]>();
  for (const c of comments) {
    const path = c.line.from.side === "L" ? (c.oldPath ?? c.file) : c.file;
    const k = keyOf(path, c.line.from.side, c.line.from.line);
    const arr = byAnchor.get(k);
    if (arr) arr.push(c);
    else byAnchor.set(k, [c]);
  }

  const seen = new Set<string>();
  const out: ReviewComment[] = [];

  for (const f of files) {
    const lPath = f.oldPath ?? f.path;
    const rPath = f.path;
    for (const h of f.hunks) {
      for (const ln of h.lines) {
        if (ln.oldLineNumber != null) {
          const arr = byAnchor.get(keyOf(lPath, "L", ln.oldLineNumber));
          if (arr) {
            for (const c of arr) {
              if (!seen.has(c.id)) {
                seen.add(c.id);
                out.push(c);
              }
            }
          }
        }
        if (ln.newLineNumber != null) {
          const arr = byAnchor.get(keyOf(rPath, "R", ln.newLineNumber));
          if (arr) {
            for (const c of arr) {
              if (!seen.has(c.id)) {
                seen.add(c.id);
                out.push(c);
              }
            }
          }
        }
      }
    }
  }

  return out;
}
