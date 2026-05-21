import { type ParsedDiff, type DiffHunk, getDiffPathKey } from "@/lib/diff-parser";
import type { CommitFile, ReviewComment } from "@/types";

/**
 * An inline-rendered comment together with the diff that owns it. Resolving the
 * owning diff (and the file's path-key) exactly once during partitioning lets
 * every downstream consumer — render grouping, nav order, auto-expansion —
 * share one canonical mapping instead of re-deriving side+path matching.
 */
export interface InlineCommentEntry {
  comment: ReviewComment;
  /** Path-key of the owning diff (matches `getDiffPathKey`). */
  pathKey: string;
  /**
   * New-side line to auto-expand context around so the comment renders inline.
   * Present only for caseB entries (anchored entries are already in a hunk).
   * For L-side comments this is the old-side anchor translated to new-side.
   */
  autoExpandLine?: number;
}

/**
 * Result of partitioning review comments against the current compare data.
 *
 * - `anchored`: the comment's anchor line is present in some current hunk —
 *   renders inline at that line as today.
 * - `caseB`: the comment's file has a parsed diff and its (new-side) line is
 *   within the file, but the anchor line is not yet covered by any hunk. The
 *   caller auto-expands context around `autoExpandLine` so it renders inline.
 * - `unanchored`: the comment has no honest place inline (backend marked it
 *   unanchored, its file is missing from the compare set, or its line is beyond
 *   the file). Rendered in a dedicated section so the reviewer can read/prune.
 */
export interface CommentPartition {
  anchored: InlineCommentEntry[];
  caseB: InlineCommentEntry[];
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
 * Translates an old-side line number to its new-side equivalent by summing the
 * offsets of hunks that precede it.
 *
 * offset = (hunk.oldStart + hunk.oldCount) - (hunk.newStart + hunk.newCount)
 * newLine = oldLine - offset
 */
function translateOldToNew(oldLine: number, hunks: readonly DiffHunk[]): number {
  let offset = 0;
  for (const h of hunks) {
    if (h.oldStart + h.oldCount <= oldLine) {
      offset = h.oldStart + h.oldCount - (h.newStart + h.newCount);
    }
  }
  return oldLine - offset;
}

/**
 * Classifies each comment against the current compare data into one of three
 * buckets: anchored / caseB / unanchored. See {@link CommentPartition} for
 * bucket semantics.
 *
 * Classification rules (case-sensitive path matching; honors renames):
 *   - Backend marked it `anchorStatus === "unanchored"` → unanchored.
 *   - No candidate ParsedDiff for the comment's side+path → unanchored.
 *   - Any hunk line carries the anchor on the comment's side → anchored.
 *   - Else translate the anchor to the new side; if `1 <= newLine <= totalLines`
 *     → caseB (auto-expand around `newLine`). Otherwise → unanchored.
 *
 * The new-side translation matters for L-side comments: their stored line is an
 * old-side number, so it must be mapped before being range-checked against the
 * new-side `totalLines`.
 */
export function partitionComments(
  parsedDiffs: readonly ParsedDiff[],
  totalLines: Readonly<Record<string, number>>,
  comments: readonly ReviewComment[],
): CommentPartition {
  const anchored: InlineCommentEntry[] = [];
  const caseB: InlineCommentEntry[] = [];
  const unanchored: ReviewComment[] = [];

  for (const c of comments) {
    // The backend couldn't re-anchor this comment (file/snippet gone). It has
    // no honest inline location — surface it for read/prune.
    if (c.anchorStatus === "unanchored") {
      unanchored.push(c);
      continue;
    }

    const diff = findCandidateDiff(parsedDiffs, c);
    if (!diff) {
      unanchored.push(c);
      continue;
    }

    const side = c.line.from.side;
    const anchorLine = c.line.from.line;
    const pathKey = getDiffPathKey(diff);

    if (hunkLineExists(diff, side, anchorLine)) {
      anchored.push({ comment: c, pathKey });
      continue;
    }

    const newLine = side === "L" ? translateOldToNew(anchorLine, diff.hunks) : anchorLine;
    const total = totalLines[pathKey] ?? 0;
    if (total > 0 && newLine >= 1 && newLine <= total) {
      caseB.push({ comment: c, pathKey, autoExpandLine: newLine });
    } else {
      unanchored.push(c);
    }
  }

  return { anchored, caseB, unanchored };
}

/** A merged auto-expand window covering one or more nearby caseB anchors. */
export interface AutoExpandTarget {
  /** Center line to expand around (passed to `expandToLine`). */
  line: number;
  /** Radius such that `[line-radius, line+radius]` covers the merged window. */
  radius: number;
  /** IDs of the caseB comments this window surfaces. */
  commentIds: string[];
}

/**
 * Merges nearby auto-expand anchors so overlapping context windows become a
 * single fetch/synthetic-hunk instead of several that would overlap (and either
 * duplicate context or, if guarded, hide a comment). Anchors whose
 * `±radius` windows overlap or touch are coalesced; the resulting target's
 * center+radius spans the union of the merged windows.
 *
 * `hunks` are the file's existing new-side hunk ranges. Two anchors are never
 * merged across a real hunk that sits between them: their merged window would
 * straddle the hunk and its center could land inside it, which makes
 * `expandToLine`'s center-in-hunk early return fire and silently drop every
 * comment in the group. Since `expandToLine` clamps each window at hunk
 * boundaries anyway, anchors on opposite sides of a hunk can never overlap, so
 * splitting them costs nothing. (A pure deletion's new-side span is only 6
 * lines, so two anchors bracketing it sit exactly 7 apart — within the merge
 * threshold — which is how the straddle arises despite -U3 context.)
 */
export function coalesceAutoExpand(
  anchors: ReadonlyArray<{ line: number; commentId: string }>,
  radius: number,
  hunks: readonly { newStart: number; newCount: number }[] = [],
): AutoExpandTarget[] {
  if (anchors.length === 0) return [];
  const sorted = [...anchors].sort((a, b) => a.line - b.line);

  const hunkBetween = (lo: number, hi: number): boolean =>
    hunks.some((h) => h.newStart > lo && h.newStart + h.newCount - 1 < hi);

  const groups: { start: number; end: number; ids: string[]; lastLine: number }[] = [];
  for (const a of sorted) {
    const last = groups[groups.length - 1];
    // Merge when this anchor's window starts within (or adjacent to) the
    // current group's covered range AND no real hunk sits between them.
    if (last && a.line - radius <= last.end + 1 && !hunkBetween(last.lastLine, a.line)) {
      last.end = Math.max(last.end, a.line + radius);
      last.ids.push(a.commentId);
      last.lastLine = a.line;
    } else {
      groups.push({ start: a.line - radius, end: a.line + radius, ids: [a.commentId], lastLine: a.line });
    }
  }

  return groups.map((g) => ({
    line: Math.floor((g.start + g.end) / 2),
    radius: Math.ceil((g.end - g.start) / 2),
    commentIds: g.ids,
  }));
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
 *   2. Then unanchored comments, grouped by file path (see
 *      {@link sortUnanchoredCommentsByFile}).
 */
export function sortCommentsByRenderOrder(
  parsedDiffs: readonly ParsedDiff[],
  files: readonly CommitFile[],
  partition: CommentPartition,
): ReviewComment[] {
  const out: ReviewComment[] = [];

  // Resolve each file's diff once so L-side anchors can be translated to the
  // new side for ordering.
  const diffByKey = new Map<string, ParsedDiff>();
  for (const d of parsedDiffs) diffByKey.set(getDiffPathKey(d), d);

  // New-side render coordinate for an inline entry. The diff lays out lines in
  // new-side order, so mixing L-side comments (stored as old-side numbers) with
  // R-side/caseB comments requires translating L-side anchors first; otherwise
  // navigation jumps out of visual order. caseB entries already carry the
  // translated new-side line in `autoExpandLine`.
  const sortLineFor = (e: InlineCommentEntry): number => {
    if (e.autoExpandLine != null) return e.autoExpandLine;
    const { side, line } = e.comment.line.from;
    if (side !== "L") return line;
    const diff = diffByKey.get(e.pathKey);
    return diff ? translateOldToNew(line, diff.hunks) : line;
  };

  // Bucket inline entries by owning diff, tagging each with its new-side
  // coordinate (entries already carry the path-key from partitioning).
  const inlineByDiff = new Map<string, { comment: ReviewComment; sortLine: number }[]>();
  for (const e of [...partition.anchored, ...partition.caseB]) {
    const item = { comment: e.comment, sortLine: sortLineFor(e) };
    const arr = inlineByDiff.get(e.pathKey);
    if (arr) arr.push(item);
    else inlineByDiff.set(e.pathKey, [item]);
  }

  for (const d of parsedDiffs) {
    const pathKey = getDiffPathKey(d);
    const arr = inlineByDiff.get(pathKey);
    if (!arr) continue;
    arr.sort((a, b) => a.sortLine - b.sortLine);
    out.push(...arr.map((x) => x.comment));
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
