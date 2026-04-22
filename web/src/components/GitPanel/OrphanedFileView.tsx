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

export const OrphanedFileView = memo(function OrphanedFileView(props: Props) {
  const { ref: mountRef, shouldMount } = useLazyMount("300px");

  const anchors = props.comments
    .map((c) => ({
      id: c.id,
      line: c.orphanLine ?? 0,
      side: c.orphanSide ?? c.line.from.side,
    }))
    .filter((a) => a.line >= 1);

  const hasAnchor = anchors.length > 0;
  const minLine = hasAnchor ? Math.min(...anchors.map((a) => a.line)) : 0;
  const fetchRef = anchors[0]?.side === "L" ? props.baseRef : props.headRef;

  const { data, isLoading, error } = useOrphanHunk({
    path: props.workingDirectory,
    file: props.file,
    ref: fetchRef,
    anchorLine: minLine,
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

  const remappedComments = useMemo<ReviewComment[]>(() => {
    if (!data) return props.comments;
    return props.comments.map((c) => {
      if (!c.orphaned || !c.orphanLine) return c;
      const side = c.orphanSide ?? c.line.from.side;
      return {
        ...c,
        line: {
          from: { side, line: c.orphanLine },
          to: { side, line: c.orphanLine },
        },
      };
    });
  }, [data, props.comments]);

  const fileDeleted = props.comments.some((c) => c.orphanDeleted);

  return (
    <div ref={mountRef} data-orphan-key={props.groupKey} className="border-border mb-3 rounded border">
      <div className="bg-muted text-muted-foreground flex items-center gap-2 border-b px-3 py-2 text-xs">
        <FileWarning className="h-3.5 w-3.5" />
        <span className="font-medium">{props.displayFile}</span>
        <span className="italic">
          {fileDeleted
            ? "file no longer exists at these refs — showing stored snippet"
            : `not in compare diff — shown from ${fetchRef.slice(0, 7)}`}
        </span>
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
      ) : error || !syntheticDiff ? (
        <DegradedView
          comments={props.comments}
          onDelete={props.onDeleteComment}
          onEdit={props.onEditComment}
          onEditRequest={props.onEditCommentRequest}
          onCommentRef={props.onCommentRef}
        />
      ) : (
        <ExpandableUnifiedDiff
          diff={syntheticDiff}
          totalLines={syntheticDiff.hunks[0].lines.length}
          expansionContext={expansionContext}
          fileName={props.displayFile}
          comments={remappedComments}
          onLineClick={noop}
          onDeleteComment={props.onDeleteComment}
          onEditComment={props.onEditComment}
          onEditCommentRequest={props.onEditCommentRequest}
          onCommentRef={props.onCommentRef}
        />
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
