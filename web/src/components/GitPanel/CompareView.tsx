import { useState, useRef, useCallback, useMemo, useEffect, memo } from "react";
import {
  Loader2,
  GitCompareArrows,
  FilePlus,
  FileX,
  FileText,
  ArrowRight,
  ArrowLeft,
  AlertCircle,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { UnifiedDiff } from "@/components/DiffViewer/UnifiedDiff";
import { parseMultiFileDiff, getDiffFileName, getDiffPathKey, type DiffLine } from "@/lib/diff-parser";
import { useCompareBranchesQuery, useCompareQuery, useGitCurrentBranchQuery } from "@/data/git";
import { useReviewQuery, useSaveReviewMutation } from "@/data/review";
import { reviewKeys } from "@/data/review/keys";
import { ReviewSubmitButton } from "./ReviewSubmitButton";
import { ReviewBodyCard } from "./ReviewBodyCard";
import { CommentNav } from "./CommentNav";
import { MobileCommentSheet } from "./MobileCommentSheet";
import { useViewport } from "@/hooks/useViewport";
import type { CommitFile, FileStatus, ReviewComment, Review } from "@/types";

const EMPTY_COMMENTS: ReviewComment[] = [];

interface CompareViewProps {
  workingDirectory: string;
  header?: React.ReactNode;
  listWidth?: number;
  onResizeMouseDown?: (e: React.MouseEvent) => void;
}

export function CompareView({ workingDirectory, header, listWidth, onResizeMouseDown }: CompareViewProps) {
  const { isMobile } = useViewport();
  const queryClient = useQueryClient();

  // Own branch subscription — excludes isRefetching
  const { data: currentBranch } = useGitCurrentBranchQuery(workingDirectory);

  const [baseBranch, setBaseBranch] = useState<string | null>(null);
  const [mobileShowDiffs, setMobileShowDiffs] = useState(false);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const [activeComment, setActiveComment] = useState<{
    file: string;
    from: number;
    to: number;
  } | null>(null);

  const {
    data: branchData,
    isLoading: loadingBranches,
    isError: branchError,
    error: branchErrorDetail,
  } = useCompareBranchesQuery(workingDirectory);

  // Reset repo-scoped state when working directory changes
  useEffect(() => {
    setBaseBranch(null);
    setSelectedPath(null);
    diffRefs.current.clear();
  }, [workingDirectory]);

  // Branches excluding the current one
  const availableBranches = useMemo(() => {
    if (!branchData?.branches) return [];
    if (!currentBranch) return branchData.branches;
    return branchData.branches.filter((b) => b !== currentBranch);
  }, [branchData?.branches, currentBranch]);

  // Set default base branch when branch data loads
  useEffect(() => {
    if (baseBranch !== null) return;
    if (!branchData) return;
    if (!currentBranch) return;
    const defaultBase =
      branchData.defaultBase && branchData.defaultBase !== currentBranch
        ? branchData.defaultBase
        : null;
    const fallback =
      availableBranches.find((b) => b === "main") ??
      availableBranches.find((b) => b === "master") ??
      availableBranches[0] ??
      null;
    setBaseBranch(defaultBase || fallback);
  }, [branchData, baseBranch, currentBranch, availableBranches]);

  // Clear baseBranch when currentBranch changes to match it (avoids self-compare)
  useEffect(() => {
    if (currentBranch && baseBranch && currentBranch === baseBranch) {
      setBaseBranch(null);
    }
  }, [currentBranch, baseBranch]);

  const {
    data: compareData,
    isLoading: loadingCompare,
    isError: compareError,
    error: compareErrorDetail,
  } = useCompareQuery(workingDirectory, baseBranch);

  const parsedDiffs = useMemo(() => {
    if (!compareData?.diff) return [];
    return parseMultiFileDiff(compareData.diff);
  }, [compareData?.diff]);

  const {
    data: reviewData,
  } = useReviewQuery(workingDirectory, currentBranch, baseBranch);

  const saveReview = useSaveReviewMutation(workingDirectory);

  const comments = reviewData?.comments ?? EMPTY_COMMENTS;

  // --- Pre-indexed comments by file (referentially stable per-file) ---
  const prevCommentsByFile = useRef(new Map<string, ReviewComment[]>());
  const commentsByFile = useMemo(() => {
    const next = new Map<string, ReviewComment[]>();
    for (const c of comments) {
      const arr = next.get(c.file);
      if (arr) arr.push(c);
      else next.set(c.file, [c]);
    }
    // Preserve previous array refs for files whose comments didn't change
    const prev = prevCommentsByFile.current;
    const stable = new Map<string, ReviewComment[]>();
    for (const [file, arr] of next) {
      const old = prev.get(file);
      if (old && old.length === arr.length && old.every((c, i) => c.id === arr[i].id && c.body === arr[i].body && c.submitted === arr[i].submitted)) {
        stable.set(file, old);
      } else {
        stable.set(file, arr);
      }
    }
    prevCommentsByFile.current = stable;
    return stable;
  }, [comments]);

  // Optimistically update the review cache and persist to server.
  // Accepts a functional updater so callers don't close over reviewData/comments.
  const saveAndUpdate = useCallback((updater: (prev: Review) => Review) => {
    if (!currentBranch || !baseBranch) return;
    const key = reviewKeys.forComparison(workingDirectory, currentBranch, baseBranch);
    const prev = queryClient.getQueryData<Review>(key);
    if (!prev) return;
    const updated = updater(prev);
    queryClient.setQueryData(key, updated);
    saveReview.mutate(updated);
  }, [queryClient, workingDirectory, currentBranch, baseBranch, saveReview]);

  // Comment navigation
  const [focusedCommentIdx, setFocusedCommentIdx] = useState(-1);
  const commentRefs = useRef<Map<string, HTMLElement>>(new Map());

  const sortedComments = useMemo(() => {
    if (!comments.length || !parsedDiffs.length) return [];
    const fileOrder = parsedDiffs.map((d) => getDiffPathKey(d));
    return [...comments].sort((a, b) => {
      const ai = fileOrder.indexOf(a.file);
      const bi = fileOrder.indexOf(b.file);
      if (ai !== bi) return ai - bi;
      return a.line.from - b.line.from;
    });
  }, [comments, parsedDiffs]);

  const scrollToComment = useCallback((index: number) => {
    const comment = sortedComments[index];
    if (!comment) return;
    setFocusedCommentIdx(index);
    const el = commentRefs.current.get(comment.id);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    const fileEl = diffRefs.current.get(comment.file);
    if (fileEl) {
      fileEl.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [sortedComments]);

  const handlePrevComment = useCallback(() => {
    const next = focusedCommentIdx <= 0 ? 0 : focusedCommentIdx - 1;
    scrollToComment(next);
  }, [focusedCommentIdx, scrollToComment]);

  const handleNextComment = useCallback(() => {
    const next = focusedCommentIdx >= sortedComments.length - 1
      ? sortedComments.length - 1
      : focusedCommentIdx + 1;
    scrollToComment(next);
  }, [focusedCommentIdx, sortedComments.length, scrollToComment]);

  const setCommentRef = useCallback((id: string, el: HTMLElement | null) => {
    if (el) {
      commentRefs.current.set(id, el);
    } else {
      commentRefs.current.delete(id);
    }
  }, []);

  const setDiffRef = useCallback(
    (path: string) => (el: HTMLDivElement | null) => {
      if (el) {
        diffRefs.current.set(path, el);
      } else {
        diffRefs.current.delete(path);
      }
    },
    [],
  );

  const scrollToFile = useCallback((path: string) => {
    setSelectedPath(path);
    if (isMobile) {
      setMobileShowDiffs(true);
    } else {
      const el = diffRefs.current.get(path);
      if (el) {
        el.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  }, [isMobile]);

  // --- Stable callbacks for UnifiedDiff ---
  const handleLineClick = useCallback((file: string, line: number) => {
    setActiveComment({ file, from: line, to: line });
  }, []);

  const clearActiveComment = useCallback(() => setActiveComment(null), []);

  // Stable per-file onLineClick handlers — avoids creating new closures in the render loop
  const fileLineClickHandlers = useMemo(() => {
    const map = new Map<string, (line: number) => void>();
    for (const diff of parsedDiffs) {
      const pathKey = getDiffPathKey(diff);
      map.set(pathKey, (line: number) => handleLineClick(pathKey, line));
    }
    return map;
  }, [parsedDiffs, handleLineClick]);

  // Stable activeCommentLine object — only changes when the actual values change
  const activeCommentLine = useMemo(
    () => activeComment ? { from: activeComment.from, to: activeComment.to } : null,
    [activeComment?.from, activeComment?.to],
  );

  const handleAddComment = useCallback((body: string) => {
    if (!activeComment) return;

    const diff = parsedDiffs.find((d) => getDiffPathKey(d) === activeComment.file);
    let snippet = "";
    if (diff) {
      for (const hunk of diff.hunks) {
        for (const line of hunk.lines) {
          if (line.newLineNumber === activeComment.from) {
            snippet = line.content;
            break;
          }
        }
        if (snippet) break;
      }
    }

    const lineNum = activeComment.from;
    const newComment: ReviewComment = {
      id: `rc_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
      file: activeComment.file,
      line: { from: lineNum, to: lineNum },
      snippet,
      body,
      submitted: false,
      createdAt: new Date().toISOString(),
    };

    saveAndUpdate((prev) => ({
      ...prev,
      comments: [...prev.comments, newComment],
    }));
    setActiveComment(null);
  }, [activeComment, parsedDiffs, saveAndUpdate]);

  const handleDeleteComment = useCallback((id: string) => {
    saveAndUpdate((prev) => ({
      ...prev,
      comments: prev.comments.filter((c) => c.id !== id),
    }));
  }, [saveAndUpdate]);

  const handleSubmitComments = useCallback((generalCommentBody: string) => {
    saveAndUpdate((prev) => ({
      ...prev,
      comments: prev.comments.map((c) => ({ ...c, submitted: true })),
      body: generalCommentBody
        ? {
            body: generalCommentBody,
            submitted: true,
            createdAt: prev.body?.createdAt ?? new Date().toISOString(),
          }
        : undefined,
    }));
  }, [saveAndUpdate]);

  const handleGeneralCommentChange = useCallback((body: string) => {
    saveAndUpdate((prev) => ({
      ...prev,
      body: {
        body,
        submitted: false,
        createdAt: prev.body?.createdAt ?? new Date().toISOString(),
      },
    }));
  }, [saveAndUpdate]);

  const handleDeleteBody = useCallback(() => {
    saveAndUpdate((prev) => ({ ...prev, body: undefined }));
  }, [saveAndUpdate]);

  // Scroll to selected file once diffs are rendered and refs are ready
  useEffect(() => {
    if (!selectedPath || !mobileShowDiffs) return;
    const el = diffRefs.current.get(selectedPath);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [selectedPath, parsedDiffs, mobileShowDiffs]);

  if (loadingBranches) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
      </div>
    );
  }

  if (branchError) {
    return (
      <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
        <AlertCircle className="mb-2 h-8 w-8 opacity-50" />
        <p className="text-sm">
          {branchErrorDetail instanceof Error
            ? branchErrorDetail.message
            : "Failed to load branches"}
        </p>
      </div>
    );
  }

  if (!baseBranch && availableBranches.length === 0) {
    return (
      <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
        <GitCompareArrows className="mb-2 h-8 w-8 opacity-50" />
        <p className="text-sm">No branches available to compare</p>
        <p className="text-xs">
          Create another branch or set an upstream tracking branch
        </p>
      </div>
    );
  }

  const pendingCount = comments.filter((c) => !c.submitted).length;

  const branchSelector = (
    <div className="flex items-center gap-2 px-3 py-2">
      <span className="text-muted-foreground text-xs">Base:</span>
      <select
        value={baseBranch ?? ""}
        onChange={(e) => setBaseBranch(e.target.value)}
        className="bg-muted border-border rounded border px-2 py-1 text-xs"
      >
        {availableBranches.map((branch) => (
          <option key={branch} value={branch}>
            {branch}
          </option>
        ))}
      </select>
    </div>
  );

  const summary = compareData ? (
    <div className="text-muted-foreground border-border/50 border-b px-3 py-1.5 text-xs">
      {compareData.files.length} file{compareData.files.length !== 1 ? "s" : ""} changed
      {compareData.totalAdditions > 0 && (
        <span className="ml-2 text-green-500">+{compareData.totalAdditions}</span>
      )}
      {compareData.totalDeletions > 0 && (
        <span className="ml-1 text-red-500">-{compareData.totalDeletions}</span>
      )}
    </div>
  ) : null;

  const fileList = compareData?.files.length ? (
    <div className="flex-1 overflow-y-auto">
      {compareData.files.map((file) => (
        <CompareFileRow
          key={file.path}
          file={file}
          isSelected={selectedPath === file.path}
          onClick={() => scrollToFile(file.path)}
        />
      ))}
    </div>
  ) : null;

  // --- Shared diff rendering helper ---
  const renderDiffs = (wrapLines = true, showAddComment = true) => (
    <div className="space-y-3 pt-3">
      {reviewData?.body?.body && (
        <ReviewBodyCard body={reviewData.body} onDelete={handleDeleteBody} />
      )}
      {parsedDiffs.map((diff) => {
        const pathKey = getDiffPathKey(diff);
        const fileName = getDiffFileName(diff);
        const fileComments = commentsByFile.get(pathKey) ?? EMPTY_COMMENTS;
        const fileActiveCommentLine = activeComment?.file === pathKey ? activeCommentLine : null;
        return (
          <div key={pathKey} ref={setDiffRef(pathKey)}>
            <UnifiedDiff
              diff={diff}
              fileName={fileName}
              expanded
              wrapLines={wrapLines}
              comments={fileComments}
              activeCommentLine={fileActiveCommentLine}
              onLineClick={fileLineClickHandlers.get(pathKey)}
              onAddComment={showAddComment ? handleAddComment : undefined}
              onCancelComment={showAddComment ? clearActiveComment : undefined}
              onDeleteComment={handleDeleteComment}
              onCommentRef={setCommentRef}
            />
          </div>
        );
      })}
    </div>
  );

  const diffPane = (
    <div className="flex-1 overflow-y-auto px-3 pb-3">
      {loadingCompare ? (
        <div className="flex h-32 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      ) : compareError ? (
        <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
          <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
          <p className="text-sm">
            {compareErrorDetail instanceof Error
              ? compareErrorDetail.message
              : "Failed to compare branches"}
          </p>
        </div>
      ) : parsedDiffs.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
          <GitCompareArrows className="mb-4 h-12 w-12 opacity-50" />
          <p className="text-sm">No changes between branches</p>
        </div>
      ) : (
        renderDiffs()
      )}
    </div>
  );

  // Mobile: full-screen diff view when user taps a file
  if (isMobile && mobileShowDiffs) {
    return (
      <div className="relative flex h-full flex-col">
        <div className="bg-muted/30 flex items-center gap-2 p-2">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setMobileShowDiffs(false)}
            aria-label="Back to file list"
          >
            <ArrowLeft className="h-5 w-5" />
          </Button>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">
              {baseBranch ? `Changes from ${baseBranch}` : "Compare"}
            </p>
            {compareData && (
              <p className="text-muted-foreground text-xs">
                {compareData.files.length} file{compareData.files.length !== 1 ? "s" : ""} changed
              </p>
            )}
          </div>
          {baseBranch && (
            <ReviewSubmitButton
              pendingCount={pendingCount}
              generalComment={reviewData?.body?.body ?? ""}
              onGeneralCommentChange={handleGeneralCommentChange}
              onSubmit={handleSubmitComments}
            />
          )}
        </div>
        <div className="safe-area-bottom flex-1 overflow-auto">
          {loadingCompare ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : compareError ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
              <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">
                {compareErrorDetail instanceof Error
                  ? compareErrorDetail.message
                  : "Failed to compare branches"}
              </p>
            </div>
          ) : (
            <div className="p-3">{renderDiffs(false, false)}</div>
          )}
        </div>
        {sortedComments.length > 0 && (
          <div className="pointer-events-none absolute right-3 bottom-3 z-10">
            <div className="pointer-events-auto">
              <CommentNav
                currentIndex={focusedCommentIdx}
                total={sortedComments.length}
                onPrev={handlePrevComment}
                onNext={handleNextComment}
                variant="pill"
              />
            </div>
          </div>
        )}
        <MobileCommentSheet
          activeComment={activeComment}
          activeLines={activeComment ? (() => {
            const diff = parsedDiffs.find((d) => getDiffPathKey(d) === activeComment.file);
            if (!diff) return [];
            const lines: DiffLine[] = [];
            for (const hunk of diff.hunks) {
              for (const line of hunk.lines) {
                if (
                  line.newLineNumber != null &&
                  line.newLineNumber >= activeComment.from &&
                  line.newLineNumber <= activeComment.to
                ) {
                  lines.push(line);
                }
              }
            }
            return lines;
          })() : []}
          onAddComment={handleAddComment}
          onCancel={clearActiveComment}
        />
      </div>
    );
  }

  // Mobile: file list view (default)
  if (isMobile) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        {branchSelector}
        {summary}
        <div className="safe-area-bottom flex-1 overflow-y-auto">
          {loadingCompare ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : compareError ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
              <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">
                {compareErrorDetail instanceof Error
                  ? compareErrorDetail.message
                  : "Failed to compare branches"}
              </p>
            </div>
          ) : compareData?.files.length ? (
            compareData.files.map((file) => (
              <CompareFileRow
                key={file.path}
                file={file}
                isSelected={selectedPath === file.path}
                onClick={() => scrollToFile(file.path)}
              />
            ))
          ) : compareData ? (
            <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
              <GitCompareArrows className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">No changes between branches</p>
            </div>
          ) : null}
        </div>
      </div>
    );
  }

  // Desktop layout
  return (
    <div className="flex min-h-0 flex-1">
      {/* Left sidebar */}
      <div className="flex h-full min-w-0 flex-col" style={{ width: listWidth }}>
        {header}
        {branchSelector}
        {summary}
        {fileList}
      </div>

      {/* Resize handle */}
      <div
        className="bg-muted/50 hover:bg-primary/50 active:bg-primary w-1 flex-shrink-0 cursor-col-resize transition-colors"
        onMouseDown={onResizeMouseDown}
      />

      {/* Right pane */}
      <div className="bg-muted/20 flex min-w-0 flex-1 flex-col">
        {baseBranch && (
          <div className="border-border sticky top-0 z-10 flex items-center gap-2 border-b bg-inherit px-3 py-2">
            <CommentNav
              currentIndex={focusedCommentIdx}
              total={sortedComments.length}
              onPrev={handlePrevComment}
              onNext={handleNextComment}
            />
            <span className="text-muted-foreground flex-1 text-xs">
              {pendingCount > 0
                ? `${pendingCount} pending`
                : ""}
            </span>
            <ReviewSubmitButton
              pendingCount={pendingCount}
              generalComment={reviewData?.body?.body ?? ""}
              onGeneralCommentChange={handleGeneralCommentChange}
              onSubmit={handleSubmitComments}
            />
          </div>
        )}
        {diffPane}
      </div>
    </div>
  );
}

function CompareFileRow({
  file,
  isSelected,
  onClick,
}: {
  file: CommitFile;
  isSelected: boolean;
  onClick: () => void;
}) {
  const StatusIcon = getStatusIcon(file.status);

  return (
    <button
      onClick={onClick}
      className={cn(
        "hover:bg-muted/70 flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors",
        isSelected && "bg-primary/10 hover:bg-primary/20",
      )}
    >
      <StatusIcon
        className={cn("h-4 w-4 flex-shrink-0", getStatusColor(file.status))}
      />
      <span className="flex-1 truncate text-sm">
        {file.oldPath ? (
          <span className="flex items-center gap-1">
            <span className="text-muted-foreground">{file.oldPath}</span>
            <ArrowRight className="h-3 w-3" />
            <span>{file.path}</span>
          </span>
        ) : (
          file.path
        )}
      </span>
      <div className="flex flex-shrink-0 items-center gap-1 text-xs">
        {file.additions > 0 && (
          <span className="text-green-500">+{file.additions}</span>
        )}
        {file.deletions > 0 && (
          <span className="text-red-500">-{file.deletions}</span>
        )}
      </div>
    </button>
  );
}

function getStatusIcon(status: FileStatus) {
  switch (status) {
    case "added":
      return FilePlus;
    case "deleted":
      return FileX;
    case "renamed":
      return ArrowRight;
    default:
      return FileText;
  }
}

function getStatusColor(status: FileStatus): string {
  switch (status) {
    case "added":
      return "text-green-500";
    case "deleted":
      return "text-red-500";
    case "renamed":
      return "text-yellow-500";
    default:
      return "text-muted-foreground";
  }
}
