import { memo, useMemo } from "react";
import { Loader2, FileWarning } from "lucide-react";
import { ApiError } from "@/api/client";
import { ExpandableUnifiedDiff } from "@/components/DiffViewer/ExpandableUnifiedDiff";
import { InlineCommentCard } from "@/components/DiffViewer/InlineCommentCard";
import type { ExpansionContext } from "@/hooks/useExpandableDiff";
import { useSyntheticHunk } from "@/hooks/useSyntheticHunk";
import { useLazyMount } from "@/hooks/useLazyMount";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";

interface Props {
  workingDirectory: string;
  groupKey: string;
  displayFile: string;
  /** New-side path. Used as the fetch path for R-side groups. */
  file: string;
  /** Old-side path (renames). Used as the fetch path for L-side groups so we
   * don't ask baseRef for a path that only exists post-rename. */
  oldPath?: string;
  comments: ReviewComment[];
  headRef: string;
  baseRef: string;
  onDeleteComment: (id: string) => void;
  onEditComment: (id: string, body: string) => void;
  onEditCommentRequest?: (comment: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
}

// Stable no-op for onLineClick — never changes identity.
function noop(_position: DiffPosition) {}

// Cap on the synthetic hunk's core span (min..max anchor). useSyntheticHunk
// expands ±10 lines on each side, yielding a 500-line request at the cap —
// matching the backend's maxLineSpan. Kept conservative so schema changes
// elsewhere don't silently push requests over the limit.
const MAX_FETCH_SPAN = 480;

export const OutOfDiffFile = memo(function OutOfDiffFile(props: Props) {
  const { ref: mountRef, shouldMount } = useLazyMount("300px");

  // When a group has comments on both sides we render the majority side as
  // the diff and push the minority into DegradedView — a single fetch can
  // only target one ref, and splitting the group upstream would duplicate the
  // file header. Ties prefer R (new side) to match the usual review mental
  // model.
  const { primarySide, primaryComments, secondaryComments } = useMemo(() => {
    const byL: ReviewComment[] = [];
    const byR: ReviewComment[] = [];
    for (const c of props.comments) {
      if (c.line.from.side === "L") byL.push(c);
      else byR.push(c);
    }
    if (byL.length > 0 && byR.length === 0) {
      return { primarySide: "L" as const, primaryComments: byL, secondaryComments: [] as ReviewComment[] };
    }
    if (byR.length >= byL.length) {
      return { primarySide: "R" as const, primaryComments: byR, secondaryComments: byL };
    }
    return { primarySide: "L" as const, primaryComments: byL, secondaryComments: byR };
  }, [props.comments]);

  const hasComments = primaryComments.length > 0;
  const fetchRef = primarySide === "L" ? props.baseRef : props.headRef;
  // Pick the path that matches fetchRef: L-side groups point at baseRef, which
  // only has the old path for renamed files. Falling back to file keeps the
  // non-rename case working (oldPath is empty there).
  const fetchFile = primarySide === "L" && props.oldPath ? props.oldPath : props.file;

  // Bounding window across all primary-side anchor lines so every comment
  // lands inside the rendered hunk. Capped at MAX_FETCH_SPAN to stay under
  // the backend's file-lines span limit (see maxLineSpan in operations.go);
  // comments past the cap fall through to DegradedView alongside
  // minority-side comments so the whole group doesn't degrade when comments
  // are sparse.
  const primaryLines = primaryComments.map((c) => c.line.from.line);
  const startLine = hasComments ? Math.min(...primaryLines) : 0;
  const desiredEnd = hasComments ? Math.max(...primaryLines) : 0;
  const endLine = Math.min(desiredEnd, startLine + MAX_FETCH_SPAN - 1);

  const { inlinePrimary, outOfWindowPrimary } = useMemo(() => {
    const inside: ReviewComment[] = [];
    const outside: ReviewComment[] = [];
    for (const c of primaryComments) {
      const line = c.line.from.line;
      if (line >= startLine && line <= endLine) inside.push(c);
      else outside.push(c);
    }
    return { inlinePrimary: inside, outOfWindowPrimary: outside };
  }, [primaryComments, startLine, endLine]);

  const { data, isLoading, error } = useSyntheticHunk({
    path: props.workingDirectory,
    file: fetchFile,
    ref: fetchRef,
    startLine,
    endLine,
    enabled: shouldMount && hasComments && !!fetchRef,
  });

  // Treat any fetch error as a signal to drop into DegradedView, but classify
  // the cause so the status text doesn't lie when the file is binary, too
  // large, or the request just transiently failed. Only 404 means the path
  // is actually gone at this ref.
  const fetchFailed = shouldMount && hasComments && !!error;
  const fetchErrorKind: "missing" | "unrenderable" | "transient" | null = fetchFailed
    ? error instanceof ApiError
      ? error.status === 404
        ? "missing"
        : error.status === 413 || error.status === 422
          ? "unrenderable"
          : "transient"
      : "transient"
    : null;

  const syntheticDiff = useMemo<ParsedDiff | null>(() => {
    if (!data) return null;
    const lines: DiffLine[] = data.lines.map((l) => ({
      type: "context",
      content: l.content,
      oldLineNumber: l.lineNumber,
      newLineNumber: l.lineNumber,
    }));
    const hunk: DiffHunk = {
      header: `@@ ${data.start}-${data.end} @@`,
      oldStart: data.start,
      oldCount: lines.length,
      newStart: data.start,
      newCount: lines.length,
      lines,
      stableKey: `out-of-diff-${props.groupKey}-${data.start}`,
    };
    return {
      oldFile: fetchFile,
      newFile: fetchFile,
      hunks: [hunk],
      additions: 0,
      deletions: 0,
      isBinary: false,
      isNew: false,
      isDeleted: false,
      isRenamed: false,
    };
  }, [data, fetchFile, props.groupKey]);

  // ExpansionContext: tells useExpandableDiff where to fetch lines when the user
  // clicks "expand". We point at the same ref/path used to fetch the synthetic hunk.
  // ref is optional in ExpansionContext so it's fine to pass undefined when fetchRef is "".
  const expansionContext = useMemo<ExpansionContext>(
    () => ({
      repoPath: props.workingDirectory,
      filePath: fetchFile,
      ref: fetchRef || undefined,
    }),
    [props.workingDirectory, fetchFile, fetchRef],
  );

  // After the fetch resolves, partition inlinePrimary against the actual
  // returned range: the backend clamps end to file length without erroring,
  // so a comment whose anchor is in the requested window but past EOF would
  // otherwise have no rendered line to host its card and disappear.
  const { hostedPrimary, pastEOFPrimary } = useMemo(() => {
    if (!data) return { hostedPrimary: inlinePrimary, pastEOFPrimary: [] as ReviewComment[] };
    const hosted: ReviewComment[] = [];
    const past: ReviewComment[] = [];
    for (const c of inlinePrimary) {
      const line = c.line.from.line;
      if (line >= data.start && line <= data.end) hosted.push(c);
      else past.push(c);
    }
    return { hostedPrimary: hosted, pastEOFPrimary: past };
  }, [data, inlinePrimary]);

  // The synthetic hunk renders both old/new line numbers as the same value
  // (it's context-only), so collapse the comment's anchor range to a single
  // line on the primary side to ensure ExpandableUnifiedDiff can host the
  // card on a real rendered line.
  const remappedPrimary = useMemo<ReviewComment[]>(() => {
    if (!data) return hostedPrimary;
    return hostedPrimary.map((c) => {
      const side = c.line.from.side;
      const line = c.line.from.line;
      return {
        ...c,
        line: {
          from: { side, line },
          to: { side, line },
        },
      };
    });
  }, [data, hostedPrimary]);

  const degradedComments = useMemo(
    () => [...secondaryComments, ...outOfWindowPrimary, ...pastEOFPrimary],
    [secondaryComments, outOfWindowPrimary, pastEOFPrimary],
  );

  const statusText = fetchErrorKind === "missing"
    ? "file no longer exists at this ref — showing stored snippet"
    : fetchErrorKind === "unrenderable"
      ? "file cannot be rendered inline (binary or too large) — showing stored snippet"
      : fetchErrorKind === "transient"
        ? "could not load file at this ref — showing stored snippet"
        : hasComments && fetchRef
          ? `not in compare diff — shown from ${fetchRef.slice(0, 7)}`
          : "not in compare diff — snippet shown (no ref available)";

  return (
    <div ref={mountRef} data-out-of-diff-key={props.groupKey} className="border-border mb-3 rounded border">
      <div className="bg-muted text-muted-foreground flex items-center gap-2 border-b px-3 py-2 text-xs">
        <FileWarning className="h-3.5 w-3.5" />
        <span className="font-medium">{props.displayFile}</span>
        <span className="italic">{statusText}</span>
      </div>
      {!shouldMount ? (
        // Pre-mount placeholder: keeps the outer div sized for the
        // IntersectionObserver without registering comment refs. Without this
        // gate, DegradedView would render pre-mount and populate commentRefs,
        // causing scrollToComment to short-circuit before its out-of-diff
        // polling branch runs.
        <div className="flex items-center justify-center py-6">
          <Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
        </div>
      ) : fetchFailed || !hasComments ? (
        <DegradedView
          comments={props.comments}
          onDelete={props.onDeleteComment}
          onEdit={props.onEditComment}
          onEditRequest={props.onEditCommentRequest}
          onCommentRef={props.onCommentRef}
        />
      ) : isLoading ? (
        <div className="flex items-center justify-center py-6">
          <Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
        </div>
      ) : error || !data || !syntheticDiff ? (
        <DegradedView
          comments={props.comments}
          onDelete={props.onDeleteComment}
          onEdit={props.onEditComment}
          onEditRequest={props.onEditCommentRequest}
          onCommentRef={props.onCommentRef}
        />
      ) : (
        <>
          <ExpandableUnifiedDiff
            diff={syntheticDiff}
            totalLines={data.totalLines}
            expansionContext={expansionContext}
            fileName={props.displayFile}
            comments={remappedPrimary}
            onLineClick={noop}
            onDeleteComment={props.onDeleteComment}
            onEditComment={props.onEditComment}
            onEditCommentRequest={props.onEditCommentRequest}
            onCommentRef={props.onCommentRef}
          />
          {degradedComments.length > 0 && (
            <div className="border-border border-t">
              <DegradedView
                comments={degradedComments}
                onDelete={props.onDeleteComment}
                onEdit={props.onEditComment}
                onEditRequest={props.onEditCommentRequest}
                onCommentRef={props.onCommentRef}
              />
            </div>
          )}
        </>
      )}
    </div>
  );
});

function DegradedView({
  comments,
  onDelete,
  onEdit,
  onEditRequest,
  onCommentRef,
}: {
  comments: ReviewComment[];
  onDelete: (id: string) => void;
  onEdit: (id: string, body: string) => void;
  onEditRequest?: (c: ReviewComment) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
}) {
  return (
    <div className="space-y-3 py-3">
      {comments.map((c) => (
        <div key={c.id} ref={(el) => onCommentRef?.(c.id, el)} className="space-y-2">
          <pre className="bg-muted mx-3 overflow-x-auto rounded px-2 py-1 text-xs">
            {c.snippet}
          </pre>
          <InlineCommentCard
            comment={c}
            onDelete={onDelete}
            onEdit={onEdit}
            onEditRequest={onEditRequest}
          />
        </div>
      ))}
    </div>
  );
}
