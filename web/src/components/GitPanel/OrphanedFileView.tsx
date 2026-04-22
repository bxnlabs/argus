import { memo, useMemo } from "react";
import { Loader2, FileWarning } from "lucide-react";
import { ExpandableUnifiedDiff } from "@/components/DiffViewer/ExpandableUnifiedDiff";
import type { ExpansionContext } from "@/hooks/useExpandableDiff";
import { useOrphanHunk } from "@/hooks/useOrphanHunk";
import { useLazyMount } from "@/hooks/useLazyMount";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { ReviewComment, DiffPosition } from "@/types";

interface Props {
  workingDirectory: string;
  groupKey: string;
  displayFile: string;
  file: string;
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

// Cap on the orphan hunk's core span (min..max anchor). useOrphanHunk expands
// ±10 lines on each side, yielding a 500-line request at the cap — matching
// the backend's maxLineSpan. Kept conservative so schema changes elsewhere
// don't silently push requests over the limit.
const MAX_FETCH_SPAN = 480;

export const OrphanedFileView = memo(function OrphanedFileView(props: Props) {
  const { ref: mountRef, shouldMount } = useLazyMount("300px");

  const fileDeleted = props.comments.some((c) => c.orphanDeleted);

  // Partition comments by anchor state. The diff hunk can only surface comments
  // with a located orphanLine; everything else falls through to DegradedView so
  // the backend's "still flagged Orphaned so the UI can surface it" contract
  // holds even when the snippet couldn't be re-anchored.
  const { anchored, unanchorable } = useMemo(() => {
    const a: ReviewComment[] = [];
    const u: ReviewComment[] = [];
    for (const c of props.comments) {
      if (c.orphanLine && c.orphanLine >= 1) a.push(c);
      else u.push(c);
    }
    return { anchored: a, unanchorable: u };
  }, [props.comments]);

  // When a group has anchors on both sides we render the majority side as the
  // diff and push the minority into DegradedView — a single fetch can only
  // target one ref, and splitting the group upstream would duplicate the file
  // header. Ties prefer R (new side) to match the usual review mental model.
  const { primarySide, primaryAnchored, secondaryAnchored } = useMemo(() => {
    const byL: ReviewComment[] = [];
    const byR: ReviewComment[] = [];
    for (const c of anchored) {
      const side = c.orphanSide ?? c.line.from.side;
      if (side === "L") byL.push(c);
      else byR.push(c);
    }
    if (byL.length > 0 && byR.length === 0) {
      return { primarySide: "L" as const, primaryAnchored: byL, secondaryAnchored: [] as ReviewComment[] };
    }
    if (byR.length >= byL.length) {
      return { primarySide: "R" as const, primaryAnchored: byR, secondaryAnchored: byL };
    }
    return { primarySide: "L" as const, primaryAnchored: byL, secondaryAnchored: byR };
  }, [anchored]);

  const hasAnchor = primaryAnchored.length > 0;
  const fetchRef = primarySide === "L" ? props.baseRef : props.headRef;

  // Bounding window across all primary-side anchors so every anchored comment
  // lands inside the rendered hunk — not just the nearest one to minLine. The
  // fetched window is capped at MAX_FETCH_SPAN to stay under the backend's
  // file-lines span limit (see maxLineSpan in operations.go); anchors past the
  // cap fall through to DegradedView alongside unanchorable and minority-side
  // comments so the whole group doesn't degrade when orphans are sparse.
  const primaryLines = primaryAnchored.map((c) => c.orphanLine ?? 0);
  const startLine = hasAnchor ? Math.min(...primaryLines) : 0;
  const desiredEnd = hasAnchor ? Math.max(...primaryLines) : 0;
  const endLine = Math.min(desiredEnd, startLine + MAX_FETCH_SPAN - 1);

  const { inlinePrimary, outOfWindowPrimary } = useMemo(() => {
    const inside: ReviewComment[] = [];
    const outside: ReviewComment[] = [];
    for (const c of primaryAnchored) {
      const line = c.orphanLine ?? 0;
      if (line >= startLine && line <= endLine) inside.push(c);
      else outside.push(c);
    }
    return { inlinePrimary: inside, outOfWindowPrimary: outside };
  }, [primaryAnchored, startLine, endLine]);

  const { data, isLoading, error } = useOrphanHunk({
    path: props.workingDirectory,
    file: props.file,
    ref: fetchRef,
    startLine,
    endLine,
    enabled: shouldMount && hasAnchor && !!fetchRef,
  });

  const syntheticDiff = useMemo<ParsedDiff | null>(() => {
    if (!data) return null;
    const lines: DiffLine[] = data.lines.map((l) => ({
      type: "context",
      content: l.content,
      oldLineNumber: l.lineNumber,
      newLineNumber: l.lineNumber,
    }));
    const hunk: DiffHunk = {
      header: `@@ orphan ${data.start}-${data.end} @@`,
      oldStart: data.start,
      oldCount: lines.length,
      newStart: data.start,
      newCount: lines.length,
      lines,
      stableKey: `orphan-${props.groupKey}-${data.start}`,
    };
    return {
      oldFile: props.file,
      newFile: props.file,
      hunks: [hunk],
      additions: 0,
      deletions: 0,
      isBinary: false,
      isNew: false,
      isDeleted: false,
      isRenamed: false,
    };
  }, [data, props.file, props.groupKey]);

  // ExpansionContext: tells useExpandableDiff where to fetch lines when the user
  // clicks "expand". We point at the same ref used to fetch the orphan hunk.
  // ref is optional in ExpansionContext so it's fine to pass undefined when fetchRef is "".
  const expansionContext = useMemo<ExpansionContext>(
    () => ({
      repoPath: props.workingDirectory,
      filePath: props.file,
      ref: fetchRef || undefined,
    }),
    [props.workingDirectory, props.file, fetchRef],
  );

  const remappedPrimary = useMemo<ReviewComment[]>(() => {
    if (!data) return inlinePrimary;
    return inlinePrimary.map((c) => {
      const side = c.orphanSide ?? c.line.from.side;
      return {
        ...c,
        line: {
          from: { side, line: c.orphanLine! },
          to: { side, line: c.orphanLine! },
        },
      };
    });
  }, [data, inlinePrimary]);

  const degradedComments = useMemo(
    () => [...unanchorable, ...secondaryAnchored, ...outOfWindowPrimary],
    [unanchorable, secondaryAnchored, outOfWindowPrimary],
  );

  const statusText = fileDeleted
    ? "file no longer exists at these refs — showing stored snippet"
    : hasAnchor && fetchRef
      ? `not in compare diff — shown from ${fetchRef.slice(0, 7)}`
      : "not in compare diff — snippet shown (original line could not be located)";

  return (
    <div ref={mountRef} data-orphan-key={props.groupKey} className="border-border mb-3 rounded border">
      <div className="bg-muted text-muted-foreground flex items-center gap-2 border-b px-3 py-2 text-xs">
        <FileWarning className="h-3.5 w-3.5" />
        <span className="font-medium">{props.displayFile}</span>
        <span className="italic">{statusText}</span>
      </div>
      {fileDeleted || !hasAnchor ? (
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
    <div className="space-y-3 px-3 py-3">
      {comments.map((c) => (
        <div key={c.id} ref={(el) => onCommentRef?.(c.id, el)} className="space-y-2">
          <pre className="bg-muted overflow-x-auto rounded px-2 py-1 text-xs">
            {c.snippet}
          </pre>
          <div className="text-sm">{c.body}</div>
          <div className="flex gap-2 text-xs">
            <button
              className="text-destructive hover:underline"
              onClick={() => onDelete(c.id)}
            >
              Delete
            </button>
            {onEditRequest && (
              <button
                className="text-muted-foreground hover:underline"
                onClick={() => onEditRequest(c)}
              >
                Edit
              </button>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
