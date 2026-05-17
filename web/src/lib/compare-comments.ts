import { type ParsedDiff, getDiffPathKey } from "@/lib/diff-parser";
import type { CommitFile, ReviewComment } from "@/types";

/**
 * Result of partitioning review comments against the current compare data.
 *
 * - `anchored`: the comment's anchor line is present in some current hunk —
 *   renders inline at that line as today.
 * - `caseB`: the comment's file has a parsed diff and `line <= totalLines`,
 *   but the anchor line is not yet covered by any hunk. The caller is expected
 *   to auto-expand context around the anchor so the comment renders inline.
 * - `unanchored`: the comment has no honest place inline (file missing from
 *   the compare set, totalLines missing/0, or anchor beyond EOF). The caller
 *   should render these in a dedicated section below the diff list so the
 *   reviewer can read and prune them.
 */
export interface CommentPartition {
  anchored: ReviewComment[];
  caseB: ReviewComment[];
  unanchored: ReviewComment[];
}

/**
 * Pick the ParsedDiff whose authored side matches the comment's file/oldPath.
 *
 * L-side anchors against the pre-rename path (oldPath ?? file) and matches
 * against `diff.oldFile` (falling back to `newFile` when oldFile is empty,
 * e.g. for new files where there is no L-side history).
 *
 * R-side anchors against the post-rename path (file) and matches against
 * `diff.newFile`.
 */
function findCandidateDiff(
  parsedDiffs: readonly ParsedDiff[],
  comment: ReviewComment,
): ParsedDiff | undefined {
  const side = comment.line.from.side;
  if (side === "L") {
    const key = comment.oldPath ?? comment.file;
    return parsedDiffs.find((d) => (d.oldFile || d.newFile) === key);
  }
  return parsedDiffs.find((d) => d.newFile === comment.file);
}

/** True iff some hunk line has the right side's lineNumber equal to `line`. */
function hunkLineExists(diff: ParsedDiff, side: "L" | "R", line: number): boolean {
  for (const h of diff.hunks) {
    for (const ln of h.lines) {
      const num = side === "L" ? ln.oldLineNumber : ln.newLineNumber;
      if (num === line) return true;
    }
  }
  return false;
}

/**
 * Classifies each comment against the current compare data into one of three
 * buckets: anchored / caseB / unanchored. See {@link CommentPartition} for
 * bucket semantics.
 *
 * Classification rules (case-sensitive path matching; honors renames):
 *   - Find a candidate ParsedDiff for the comment's side+path. If none → unanchored.
 *   - Else if any hunk line carries the anchor on the comment's side → anchored.
 *   - Else if `line <= totalLines[pathKey]` → caseB.
 *   - Else → unanchored.
 */
export function partitionComments(
  parsedDiffs: readonly ParsedDiff[],
  totalLines: Readonly<Record<string, number>>,
  comments: readonly ReviewComment[],
): CommentPartition {
  const anchored: ReviewComment[] = [];
  const caseB: ReviewComment[] = [];
  const unanchored: ReviewComment[] = [];

  for (const c of comments) {
    const diff = findCandidateDiff(parsedDiffs, c);
    if (!diff) {
      unanchored.push(c);
      continue;
    }

    const side = c.line.from.side;
    const anchorLine = c.line.from.line;

    if (hunkLineExists(diff, side, anchorLine)) {
      anchored.push(c);
      continue;
    }

    const pathKey = getDiffPathKey(diff);
    const total = totalLines[pathKey] ?? 0;
    if (total > 0 && anchorLine <= total) {
      caseB.push(c);
    } else {
      unanchored.push(c);
    }
  }

  return { anchored, caseB, unanchored };
}

/**
 * Returns comments in visual rendering order — the order in which they appear
 * top-to-bottom in the compare view DOM.
 *
 * Order:
 *   1. For each diff in `parsedDiffs` order: anchored + caseB comments for
 *      that file, sorted by anchor line on the comment's authored side.
 *      caseB comments are positioned where they'll appear AFTER auto-expand,
 *      so they sort by anchor line alongside anchored comments.
 *   2. Then unanchored comments, grouped by file path. Files in
 *      `compareData.files` come first (in that order); files not in compare
 *      data come after in first-encounter order. Within each file, sorted
 *      by line number.
 */
export function sortCommentsByRenderOrder(
  parsedDiffs: readonly ParsedDiff[],
  files: readonly CommitFile[],
  partition: CommentPartition,
): ReviewComment[] {
  const out: ReviewComment[] = [];

  // --- Inline: anchored + caseB, grouped by parsed diff order ---
  // Inline comments are bucketed by the parsed-diff path-key. L-side comments
  // on a renamed file still bucket under the new path (the diff's pathKey),
  // so they sort alongside their R-side siblings in the same file.
  const inlineByDiff = new Map<string, ReviewComment[]>();
  const inline = [...partition.anchored, ...partition.caseB];
  for (const c of inline) {
    // Bucket by the diff that owns this comment.
    let pathKey: string | null = null;
    for (const d of parsedDiffs) {
      const matchesL =
        c.line.from.side === "L" &&
        (d.oldFile || d.newFile) === (c.oldPath ?? c.file);
      const matchesR = c.line.from.side === "R" && d.newFile === c.file;
      if (matchesL || matchesR) {
        pathKey = getDiffPathKey(d);
        break;
      }
    }
    if (pathKey == null) continue; // safety — should have been unanchored
    const arr = inlineByDiff.get(pathKey);
    if (arr) arr.push(c);
    else inlineByDiff.set(pathKey, [c]);
  }

  for (const d of parsedDiffs) {
    const pathKey = getDiffPathKey(d);
    const arr = inlineByDiff.get(pathKey);
    if (!arr) continue;
    arr.sort((a, b) => a.line.from.line - b.line.from.line);
    out.push(...arr);
  }

  out.push(...sortUnanchoredCommentsByFile(partition.unanchored, files));
  return out;
}

/**
 * Orders unanchored comments by file, then by line within each file.
 *
 * Files present in `compareData.files` are ordered first (in that order;
 * post-rename `path` is matched first, then `oldPath`). Files not in the
 * compare set are appended in first-encounter order from the comment list.
 * Within a file, comments are sorted by anchor line.
 */
export function sortUnanchoredCommentsByFile(
  unanchored: readonly ReviewComment[],
  files: readonly CommitFile[],
): ReviewComment[] {
  if (unanchored.length === 0) return [];

  const byFile = new Map<string, ReviewComment[]>();
  for (const c of unanchored) {
    const key = c.line.from.side === "L" ? c.oldPath ?? c.file : c.file;
    const arr = byFile.get(key);
    if (arr) arr.push(c);
    else byFile.set(key, [c]);
  }

  const fileOrder: string[] = [];
  const seen = new Set<string>();
  for (const f of files) {
    if (byFile.has(f.path) && !seen.has(f.path)) {
      fileOrder.push(f.path);
      seen.add(f.path);
    }
    if (f.oldPath && byFile.has(f.oldPath) && !seen.has(f.oldPath)) {
      fileOrder.push(f.oldPath);
      seen.add(f.oldPath);
    }
  }
  for (const key of byFile.keys()) {
    if (!seen.has(key)) {
      fileOrder.push(key);
      seen.add(key);
    }
  }

  const out: ReviewComment[] = [];
  for (const key of fileOrder) {
    const arr = byFile.get(key)!;
    arr.sort((a, b) => a.line.from.line - b.line.from.line);
    out.push(...arr);
  }
  return out;
}
