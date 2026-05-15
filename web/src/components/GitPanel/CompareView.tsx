import { useState, useRef, useCallback, useMemo, useEffect, useLayoutEffect, memo } from "react";
import {
  Loader2,
  GitCompareArrows,
  FilePlus,
  FileX,
  FileText,
  ArrowRight,
  ArrowLeft,
  AlertCircle,
  AlertTriangle,
} from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { LazyFileDiff } from "@/components/DiffViewer/LazyFileDiff";
import { parseMultiFileDiff, getDiffFileName, getDiffPathKey, type DiffLine, type DiffHunk } from "@/lib/diff-parser";
import { useCompareBranchesQuery, useCompareQuery, useGitCurrentBranchQuery } from "@/data/git";
import { useReviewQuery, useSaveReviewMutation } from "@/data/review";
import { reviewKeys } from "@/data/review/keys";
import { ReviewSubmitButton } from "./ReviewSubmitButton";
import { ReviewBodyCard } from "./ReviewBodyCard";
import { CommentNav } from "./CommentNav";
import { MobileCommentSheet } from "./MobileCommentSheet";
import { OutOfDiffSection } from "./OutOfDiffSection";
import { OutOfDiffFile } from "./OutOfDiffFile";
import { useViewport } from "@/hooks/useViewport";
import { fetchFileLines } from "@/data/git/file-lines";
import { computeOldToNewOffset, type ExpansionContext } from "@/hooks/useExpandableDiff";
import type { CommitFile, FileStatus, ReviewComment, Review, DiffPosition } from "@/types";

const EMPTY_COMMENTS: ReviewComment[] = [];

/** Fire-and-forget helper: fetch lines and insert a synthetic hunk for an
 * off-screen comment in a changed file (case b). On fetch failure, calls
 * `onFailure(commentId)` so the caller can route the comment to the orphan
 * section instead — no placeholder hunk is inserted. */
async function fetchFileLinesForSynthetic(
  repoPath: string,
  file: string,
  start: number,
  end: number,
  ref: string | undefined,
  currentHunks: DiffHunk[],
  pos: DiffPosition,
  insertHandler: (hunk: DiffHunk, insertIndex: number) => void,
  onFailure: (commentId: string) => void,
  commentId: string,
) {
  try {
    const result = await fetchFileLines({ path: repoPath, file, start, end, ref });

    const offset = pos.side === "L" ? computeOldToNewOffset(start, currentHunks) : 0;
    const oldStart = pos.side === "L" ? start : start + offset;
    const newStart = pos.side === "L" ? start - offset : start;

    const lines: DiffLine[] = result.lines.map((content, i) => ({
      type: "context" as const,
      content,
      newLineNumber: newStart + i,
      oldLineNumber: oldStart + i,
    }));

    const syntheticHunk: DiffHunk = {
      header: `@@ -${oldStart},${lines.length} +${newStart},${lines.length} @@`,
      oldStart,
      oldCount: lines.length,
      newStart,
      newCount: lines.length,
      lines,
      stableKey: `syn:${oldStart}:${newStart}`,
    };

    // Side-aware insertion index — scan `currentHunks` for the first hunk
    // whose start exceeds the anchor on the relevant side so the new hunk
    // lands in file order.
    let idx = currentHunks.length;
    for (let i = 0; i < currentHunks.length; i++) {
      const hunkLine = pos.side === "L" ? currentHunks[i].oldStart : currentHunks[i].newStart;
      if (hunkLine > start) { idx = i; break; }
    }
    insertHandler(syntheticHunk, idx);
  } catch {
    onFailure(commentId);
  }
}

interface CompareViewProps {
  workingDirectory: string;
  header?: React.ReactNode;
  listWidth?: number;
  onResizeMouseDown?: (e: React.MouseEvent) => void;
  /**
   * Notifies the parent (GitPanel) of the active compare base so the global
   * refresh button can include it in the fetch request. Required for fork
   * workflows where HEAD and the compare base track different remotes.
   */
  onBaseChange?: (base: string | null) => void;
}

export function CompareView({ workingDirectory, header, listWidth, onResizeMouseDown, onBaseChange }: CompareViewProps) {
  const { isMobile } = useViewport();
  const queryClient = useQueryClient();

  // Own branch subscription — excludes isRefetching
  const { data: currentBranch } = useGitCurrentBranchQuery(workingDirectory);

  const [baseBranch, setBaseBranch] = useState<string | null>(null);
  const [mobileShowDiffs, setMobileShowDiffs] = useState(false);
  const [selectedOutOfDiffKey, setSelectedOutOfDiffKey] = useState<string | null>(null);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const expandedHunksRef = useRef<Map<string, DiffHunk[]>>(new Map());
  const insertSyntheticHandlers = useRef<Map<string, (hunk: DiffHunk, idx: number) => void>>(new Map());
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  // In-memory set of comment IDs whose case-(b) synthetic-hunk fetch failed.
  // Routes the comment into the orphan section for this session. Cleared on
  // compare-context change (see the workingDirectory reset effect).
  const [failedSyntheticIds, setFailedSyntheticIds] = useState<Set<string>>(() => new Set());
  // Bumped on each scroll-to-file request so repeat clicks on the same path
  // re-trigger the scroll effect.
  const [scrollRequestId, setScrollRequestId] = useState(0);
  const scrollToFileRequestRef = useRef(0);
  // True while a scroll-to-file is in flight. Force-mounts every diff so
  // scrollHeight exceeds target.offsetTop + clientHeight; otherwise the
  // browser clamps scrollTop short of the target when files after it are
  // still lazy placeholders. Cleared one frame after the scroll lands.
  const [isScrollPending, setIsScrollPending] = useState(false);
  // Records which map (file diffs vs out-of-diff groups) the most recent scroll
  // request targets. Read by the unified scroll effect to look up the target
  // element in the correct ref map without branching on which piece of state
  // changed most recently.
  const lastScrollTargetKindRef = useRef<"file" | "outOfDiff">("file");
  const [activeComment, setActiveComment] = useState<{
    file: string;
    position: DiffPosition;
  } | null>(null);
  const activeCommentRef = useRef(activeComment);
  activeCommentRef.current = activeComment;
  const [editingComment, setEditingComment] = useState<ReviewComment | null>(null);

  const {
    data: branchData,
    isLoading: loadingBranches,
    isError: branchError,
    error: branchErrorDetail,
  } = useCompareBranchesQuery(workingDirectory);

  // Reset repo-scoped state when working directory changes. The
  // scrollRequestRef bump invalidates any tick of the out-of-diff poll that was
  // already queued in the event loop before clearInterval landed, so the
  // callback bails via its requestId check if it does fire one last time.
  useEffect(() => {
    setBaseBranch(null);
    setSelectedPath(null);
    setFailedSyntheticIds(new Set());
    setIsScrollPending(false);
    setEditingComment(null);
    setFocusedCommentId(null);
    lastFocusedIdxRef.current = -1;
    diffRefs.current.clear();
    expandedHunksRef.current.clear();
    insertSyntheticHandlers.current.clear();
    scrollRequestRef.current++;
    if (outOfDiffPollRef.current !== null) {
      window.clearInterval(outOfDiffPollRef.current);
      outOfDiffPollRef.current = null;
    }
  }, [workingDirectory]);

  // Clear any in-flight out-of-diff poll on unmount so it can't outlive the view.
  // Same pattern as above: bump scrollRequestRef to defeat any already-queued
  // tick that clearInterval might not cancel.
  useEffect(() => {
    return () => {
      scrollRequestRef.current++;
      if (outOfDiffPollRef.current !== null) {
        window.clearInterval(outOfDiffPollRef.current);
        outOfDiffPollRef.current = null;
      }
    };
  }, []);

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

  // Mirror the active base up to GitPanel so its refresh button can include
  // the right remote in the fetch request. Tracked here (not lifted to
  // GitPanel) because the resolution logic depends on branch data that's
  // already loaded inside this component.
  useEffect(() => {
    onBaseChange?.(baseBranch);
  }, [baseBranch, onBaseChange]);

  const {
    data: compareData,
    isLoading: loadingCompare,
    isError: compareError,
    error: compareErrorDetail,
  } = useCompareQuery(workingDirectory, baseBranch);

  // Invalidate any in-flight out-of-diff-mount poll when the compare context
  // changes. Without this, a poll started just before a branch switch could
  // fire in the new review and scroll to a coincidentally same-id comment.
  // The scrollRequestRef bump makes the poll's next tick bail; the interval
  // clear is belt+suspenders so we don't leave dead ticks running.
  useEffect(() => {
    scrollRequestRef.current++;
    if (outOfDiffPollRef.current !== null) {
      window.clearInterval(outOfDiffPollRef.current);
      outOfDiffPollRef.current = null;
    }
  }, [baseBranch, compareData?.headRef, compareData?.baseRef]);

  const parsedDiffs = useMemo(() => {
    if (!compareData?.diff) return [];
    // Clear stale expanded hunks when diff data changes (e.g. base branch switch)
    expandedHunksRef.current.clear();
    return parseMultiFileDiff(compareData.diff);
  }, [compareData?.diff]);

  // Index of the selected file in the rendered diff order. -1 when nothing is
  // selected, or when the selected path is no longer present in parsedDiffs
  // (e.g. after the base branch changes).
  const selectedIdx = useMemo(() => {
    if (!selectedPath) return -1;
    return parsedDiffs.findIndex((d) => getDiffPathKey(d) === selectedPath);
  }, [selectedPath, parsedDiffs]);

  const {
    data: reviewData,
  } = useReviewQuery(workingDirectory, currentBranch, baseBranch, compareData?.headRef, compareData?.baseRef);

  const saveReview = useSaveReviewMutation(workingDirectory);

  const comments = reviewData?.comments ?? EMPTY_COMMENTS;

  // Set of paths represented in the compare diff (both new paths and old
  // paths for renames). A comment whose file is not in this set is "out of
  // diff" — its anchor isn't hosted by any rendered file row in the diff
  // pane, so we render it via a synthetic hunk fetched from the relevant ref.
  const diffPathSet = useMemo(() => {
    const s = new Set<string>();
    if (compareData) {
      for (const f of compareData.files) {
        s.add(f.path);
        if (f.oldPath) s.add(f.oldPath);
      }
    }
    return s;
  }, [compareData]);
  const isInDiff = useCallback(
    (c: ReviewComment) =>
      (diffPathSet.has(c.file) || (!!c.oldPath && diffPathSet.has(c.oldPath))) &&
      !failedSyntheticIds.has(c.id),
    [diffPathSet, failedSyntheticIds],
  );

  // Map every alias path (oldPath included) to the file's canonical pathKey.
  // CommitFile.path already matches what getDiffPathKey returns for the
  // corresponding ParsedDiff (newFile for modify/rename/add, oldFile for
  // delete), so a single lookup resolves any pre-rename comment whose
  // `c.file` points at the old name to the rendered diff's key.
  const canonicalKeyByPath = useMemo(() => {
    const m = new Map<string, string>();
    if (compareData) {
      for (const f of compareData.files) {
        m.set(f.path, f.path);
        if (f.oldPath) m.set(f.oldPath, f.path);
      }
    }
    return m;
  }, [compareData]);
  // Resolve a comment to the canonical compare key. Falls back to c.file so
  // out-of-diff comments still get a stable identifier — they won't match any
  // rendered diff anyway, but the key drives outOfDiff group identity too.
  const canonicalKeyForComment = useCallback(
    (c: ReviewComment): string => {
      const fromFile = canonicalKeyByPath.get(c.file);
      if (fromFile) return fromFile;
      if (c.oldPath) {
        const fromOld = canonicalKeyByPath.get(c.oldPath);
        if (fromOld) return fromOld;
      }
      return c.file;
    },
    [canonicalKeyByPath],
  );

  // Only partition once the compare diff has actually loaded. Otherwise an
  // empty diffPathSet would tag every comment as out-of-diff and the desktop
  // sidebar would render a bogus OutOfDiffSection while the right pane is
  // still showing its loader. Treating the pre-data state as "all visible"
  // keeps comment-by-file lookups empty and keeps the out-of-diff UI hidden
  // until classification is meaningful.
  const canClassify = !!compareData && !loadingCompare && !compareError;
  const { visibleComments, outOfDiffComments } = useMemo(() => {
    if (!canClassify) {
      return { visibleComments: comments, outOfDiffComments: EMPTY_COMMENTS };
    }
    const visible: ReviewComment[] = [];
    const outOfDiff: ReviewComment[] = [];
    for (const c of comments) {
      if (isInDiff(c)) visible.push(c);
      else outOfDiff.push(c);
    }
    return { visibleComments: visible, outOfDiffComments: outOfDiff };
  }, [comments, isInDiff, canClassify]);

  const outOfDiffGroups = useMemo(() => {
    const byKey = new Map<string, { displayFile: string; comments: ReviewComment[] }>();
    for (const c of outOfDiffComments) {
      const key = c.oldPath ? `${c.oldPath}→${c.file}` : c.file;
      const displayFile = c.oldPath ? `${c.oldPath} → ${c.file}` : c.file;
      let g = byKey.get(key);
      if (!g) {
        g = { displayFile, comments: [] };
        byKey.set(key, g);
      }
      g.comments.push(c);
    }
    return Array.from(byKey, ([key, g]) => ({ key, ...g })).sort((a, b) =>
      a.key < b.key ? -1 : a.key > b.key ? 1 : 0,
    );
  }, [outOfDiffComments]);

  // --- Pre-indexed comments by file (referentially stable per-file) ---
  // Bucketed by canonical compare key so comments authored before a rename
  // (c.file = old name, no c.oldPath) still land under the rendered diff's
  // pathKey — without this remap the bucket is keyed by the old name and
  // renderDiffs.commentsByFile.get(pathKey) returns undefined.
  const prevCommentsByFile = useRef(new Map<string, ReviewComment[]>());
  const commentsByFile = useMemo(() => {
    const next = new Map<string, ReviewComment[]>();
    for (const c of visibleComments) {
      const key = canonicalKeyForComment(c);
      const arr = next.get(key);
      if (arr) arr.push(c);
      else next.set(key, [c]);
    }
    // Preserve previous array refs for files whose comments didn't change
    const prev = prevCommentsByFile.current;
    const stable = new Map<string, ReviewComment[]>();
    for (const [file, arr] of next) {
      const old = prev.get(file);
      if (old && old.length === arr.length && old.every((c, i) => c.id === arr[i].id && c.body === arr[i].body && c.submitted === arr[i].submitted && c.anchorStatus === arr[i].anchorStatus && c.line.from.line === arr[i].line.from.line && c.line.from.side === arr[i].line.from.side)) {
        stable.set(file, old);
      } else {
        stable.set(file, arr);
      }
    }
    prevCommentsByFile.current = stable;
    return stable;
  }, [visibleComments, canonicalKeyForComment]);

  // Optimistically update the review cache and persist to server.
  // Accepts a functional updater so callers don't close over reviewData/comments.
  const headRef = compareData?.headRef ?? "";
  const baseRef = compareData?.baseRef ?? "";
  const reviewQueryKey = useMemo(
    () => [...reviewKeys.forComparison(workingDirectory, currentBranch ?? "", baseBranch ?? ""), headRef, baseRef],
    [workingDirectory, currentBranch, baseBranch, headRef, baseRef],
  );
  const saveAndUpdate = useCallback((updater: (prev: Review) => Review) => {
    if (!currentBranch || !baseBranch) return;
    const prev = queryClient.getQueryData<Review>(reviewQueryKey);
    if (!prev) return;
    const updated = updater(prev);
    queryClient.setQueryData(reviewQueryKey, updated);
    saveReview.mutate(updated);
  }, [queryClient, reviewQueryKey, currentBranch, baseBranch, saveReview]);

  // Adds a comment ID to failedSyntheticIds so isInDiff routes it to the
  // orphan section. The state itself is declared higher up so that isInDiff,
  // defined earlier in the component, can close over the latest value.
  const markSyntheticFailed = useCallback((commentId: string) => {
    setFailedSyntheticIds((prev) => {
      if (prev.has(commentId)) return prev;
      const next = new Set(prev);
      next.add(commentId);
      return next;
    });
  }, []);

  // Resolve hunks for a file, preferring expanded hunks from the ref
  const getHunksForFile = useCallback((pathKey: string): DiffHunk[] => {
    const expanded = expandedHunksRef.current.get(pathKey);
    if (expanded && expanded.length > 0) return expanded;
    const diff = parsedDiffs.find((d) => getDiffPathKey(d) === pathKey);
    return diff?.hunks ?? [];
  }, [parsedDiffs]);

  // Comment navigation — track by stable comment ID so index stays correct
  // across deletes, reorders, and orphan-routing filter changes.
  const [focusedCommentId, setFocusedCommentId] = useState<string | null>(null);
  const commentRefs = useRef<Map<string, HTMLElement>>(new Map());

  const sortedComments = useMemo(() => {
    if (!parsedDiffs.length && !outOfDiffComments.length) return [];

    // --- Visible (in-diff) comments, ordered by file then diff-row index ---
    const fileOrder = parsedDiffs.map((d) => getDiffPathKey(d));

    // Build a line position index for each file
    const linePositions = new Map<string, Map<string, number>>();
    for (const diff of parsedDiffs) {
      const pathKey = getDiffPathKey(diff);
      const posMap = new Map<string, number>();
      let rowIdx = 0;
      for (const hunk of diff.hunks) {
        for (const line of hunk.lines) {
          if (line.oldLineNumber != null) posMap.set(`L${line.oldLineNumber}`, rowIdx);
          if (line.newLineNumber != null) posMap.set(`R${line.newLineNumber}`, rowIdx);
          rowIdx++;
        }
      }
      linePositions.set(pathKey, posMap);
    }

    const visible = [...visibleComments]
      .sort((a, b) => {
        const aKey = canonicalKeyForComment(a);
        const bKey = canonicalKeyForComment(b);
        const ai = fileOrder.indexOf(aKey);
        const bi = fileOrder.indexOf(bKey);
        if (ai !== bi) return ai - bi;
        const posA = linePositions.get(aKey);
        const posB = linePositions.get(bKey);
        const keyA = `${a.line.from.side}${a.line.from.line}`;
        const keyB = `${b.line.from.side}${b.line.from.line}`;
        const idxA = posA?.get(keyA) ?? a.line.from.line;
        const idxB = posB?.get(keyB) ?? b.line.from.line;
        return idxA - idxB;
      });

    // --- Out-of-diff comments, ordered by group key then stored line anchor ---
    const outOfDiff = [...outOfDiffComments].sort((a, b) => {
      const aKey = a.oldPath ? `${a.oldPath}→${a.file}` : a.file;
      const bKey = b.oldPath ? `${b.oldPath}→${b.file}` : b.file;
      if (aKey !== bKey) return aKey < bKey ? -1 : 1;
      return a.line.from.line - b.line.from.line;
    });

    return [...visible, ...outOfDiff];
  }, [visibleComments, outOfDiffComments, parsedDiffs, canonicalKeyForComment]);

  // Tracks the last visited position so navigation can resume near it when
  // the focused comment is deleted. Updated from scrollToComment and from
  // a sync effect that mirrors the live focused index.
  const lastFocusedIdxRef = useRef(-1);

  // Derive focused index from the tracked ID. Returns -1 when the focused
  // comment no longer exists, which keeps Next enabled so the user can
  // resume navigation even when only one comment remains.
  const focusedCommentIdx = useMemo(
    () => focusedCommentId
      ? sortedComments.findIndex((c) => c.id === focusedCommentId)
      : -1,
    [focusedCommentId, sortedComments],
  );

  // Keep lastFocusedIdxRef aligned with the focused comment's live position
  // so inserts/deletes/filters that move it don't leave the fallback stale.
  useEffect(() => {
    if (focusedCommentIdx !== -1) {
      lastFocusedIdxRef.current = focusedCommentIdx;
    }
  }, [focusedCommentIdx]);

  // True once a comment has been focused but then removed from the visible
  // list. Distinct from the initial unfocused state (focusedCommentId === null)
  // even though both leave focusedCommentIdx at -1.
  const isFocusStale = focusedCommentId !== null && focusedCommentIdx === -1;

  // Ref for reading the latest sortedComments inside async continuations
  // (avoids stale-closure scrolls after synthetic-hunk fetches settle).
  const sortedCommentsRef = useRef(sortedComments);
  sortedCommentsRef.current = sortedComments;

  // Monotonic token so async continuations from superseded scrollToComment
  // calls bail out instead of scrolling back to a stale target.
  const scrollRequestRef = useRef(0);

  // Tracks the in-flight out-of-diff-mount poll so we can clear it on unmount or
  // compare-context change. Without this, a poll started just before a branch
  // switch could fire in the new review and scroll to a coincidentally same-id
  // comment the user didn't ask for.
  const outOfDiffPollRef = useRef<number | null>(null);

  const scrollToComment = useCallback(async (index: number) => {
    const comment = sortedComments[index];
    if (!comment) return;
    const requestId = ++scrollRequestRef.current;
    setFocusedCommentId(comment.id);
    lastFocusedIdxRef.current = index;

    // Ensure the comment's line is visible in the current hunks before scrolling.
    // Uses the canonical key so a comment whose c.file points at a pre-rename
    // alias still resolves to the rendered diff's hunks and refs.
    const pathKey = canonicalKeyForComment(comment);
    const pos = comment.line.to;
    const currentHunks = getHunksForFile(pathKey);
    let visible = false;
    for (const hunk of currentHunks) {
      for (const line of hunk.lines) {
        if (pos.side === "L" && line.oldLineNumber === pos.line) { visible = true; break; }
        if (pos.side === "R" && line.newLineNumber === pos.line) { visible = true; break; }
      }
      if (visible) break;
    }

    if (!visible) {
      const handler = insertSyntheticHandlers.current.get(pathKey);
      if (handler) {
        const ref = pos.side === "L" ? compareData?.baseRef : compareData?.headRef;
        // L-side fetch wants the path that exists at baseRef (oldPath when
        // present, otherwise c.file is already the L-side path). R-side fetch
        // wants the path that exists at headRef — the canonical key, so a
        // pre-rename comment whose c.file is the old name still resolves to
        // the new name at headRef.
        const file = pos.side === "L"
          ? (comment.oldPath ?? comment.file)
          : pathKey;
        const start = Math.max(1, pos.line - 3);
        const end = pos.line + 3;
        await fetchFileLinesForSynthetic(
          workingDirectory, file, start, end, ref, currentHunks, pos, handler,
          markSyntheticFailed, comment.id,
        );
        // Wait for React to re-render after synthetic hunk insertion
        await new Promise<void>((resolve) => setTimeout(resolve, 50));
      }
    }

    // Bail if a newer scrollToComment has superseded this one, or if the
    // target was deleted during the async settle.
    if (requestId !== scrollRequestRef.current) return;
    if (!sortedCommentsRef.current.some((c) => c.id === comment.id)) return;

    const el = commentRefs.current.get(comment.id);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    if (!isInDiff(comment)) {
      const key = comment.oldPath ? `${comment.oldPath}→${comment.file}` : comment.file;
      const groupEl = outOfDiffRefs.current.get(key);
      if (groupEl) {
        groupEl.scrollIntoView({ behavior: "smooth", block: "start" });
      }
      // Out-of-diff groups lazy-mount their body and fetch hunk data async,
      // so the comment's DOM ref isn't populated yet on first visit. Poll
      // briefly for the ref to appear and re-scroll so Prev/Next lands on the
      // specific comment rather than the group header. Bail if a newer scroll
      // request supersedes this one mid-poll. We retain the id so the
      // compare-reset effect and component unmount can cancel an in-flight poll.
      if (outOfDiffPollRef.current !== null) {
        window.clearInterval(outOfDiffPollRef.current);
      }
      const startedAt = Date.now();
      const pollId = window.setInterval(() => {
        if (requestId !== scrollRequestRef.current) {
          window.clearInterval(pollId);
          if (outOfDiffPollRef.current === pollId) outOfDiffPollRef.current = null;
          return;
        }
        const targetEl = commentRefs.current.get(comment.id);
        if (targetEl) {
          window.clearInterval(pollId);
          if (outOfDiffPollRef.current === pollId) outOfDiffPollRef.current = null;
          targetEl.scrollIntoView({ behavior: "smooth", block: "center" });
          return;
        }
        if (Date.now() - startedAt >= 1500) {
          window.clearInterval(pollId);
          if (outOfDiffPollRef.current === pollId) outOfDiffPollRef.current = null;
        }
      }, 100);
      outOfDiffPollRef.current = pollId;
      return;
    }
    const fileEl = diffRefs.current.get(pathKey);
    if (fileEl) {
      fileEl.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [sortedComments, getHunksForFile, compareData?.baseRef, compareData?.headRef, workingDirectory, markSyntheticFailed, canonicalKeyForComment]);

  const handlePrevComment = useCallback(() => {
    // Stale focus (previous comment was deleted): target the slot that was
    // BEFORE the deleted comment so Prev actually moves backward. Contrast
    // with handleNextComment, which targets the slot at the deleted index
    // (now occupied by what was the next comment).
    if (focusedCommentIdx === -1) {
      if (sortedComments.length === 0) return;
      const target = Math.max(
        0,
        Math.min(lastFocusedIdxRef.current - 1, sortedComments.length - 1),
      );
      scrollToComment(target);
      return;
    }
    const next = focusedCommentIdx <= 0 ? 0 : focusedCommentIdx - 1;
    scrollToComment(next);
  }, [focusedCommentIdx, sortedComments.length, scrollToComment]);

  const handleNextComment = useCallback(() => {
    // Stale focus (previous comment was deleted): resume from the last
    // visited position clamped into the surviving list, so single-comment
    // remainders stay reachable and middle deletes don't snap to the top.
    if (focusedCommentIdx === -1) {
      if (sortedComments.length === 0) return;
      const fallback = Math.min(
        Math.max(0, lastFocusedIdxRef.current),
        sortedComments.length - 1,
      );
      scrollToComment(fallback);
      return;
    }
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

  const outOfDiffRefs = useRef<Map<string, HTMLElement>>(new Map());
  const setOutOfDiffRef = useCallback(
    (key: string) => (el: HTMLElement | null) => {
      if (el) outOfDiffRefs.current.set(key, el);
      else outOfDiffRefs.current.delete(key);
    },
    [],
  );

  const scrollToFile = useCallback((path: string) => {
    setSelectedPath(path);
    lastScrollTargetKindRef.current = "file";
    setIsScrollPending(true);
    if (isMobile) {
      setMobileShowDiffs(true);
    }
    // Bumping the request id triggers the unified scroll effect below, which
    // runs in useLayoutEffect AFTER the force-mount propagation commits.
    // Re-firing on repeat clicks to the same path is intentional.
    setScrollRequestId((n) => n + 1);
  }, [isMobile]);

  // Route out-of-diff clicks through the unified scroll pipeline so the
  // groups benefit from the same force-mount-predecessors stabilization as
  // file diffs. Without this, the bare scrollIntoView would drift as lazy
  // LazyFileDiff entries between the current viewport and the out-of-diff
  // section mount and expand mid-animation.
  // Defined here (above the early returns) so the hook is unconditionally
  // invoked every render — declaring it after the loadingBranches/branchError
  // returns would change the hook count between renders and trip React's
  // hook-order check.
  const scrollToOutOfDiff = useCallback((key: string) => {
    setSelectedOutOfDiffKey(key);
    lastScrollTargetKindRef.current = "outOfDiff";
    setIsScrollPending(true);
    if (isMobile) {
      setMobileShowDiffs(true);
    }
    setScrollRequestId((n) => n + 1);
  }, [isMobile]);

  // --- Stable callbacks for UnifiedDiff ---
  const handleLineClick = useCallback((file: string, position: DiffPosition) => {
    setActiveComment({ file, position });
  }, []);

  const clearActiveComment = useCallback(() => setActiveComment(null), []);
  const handleEditCommentRequest = useCallback((comment: ReviewComment) => setEditingComment(comment), []);
  const clearEditingComment = useCallback(() => setEditingComment(null), []);

  // Stable per-file expanded hunks change handler — writes to ref, no re-render
  const handleExpandedHunksChange = useCallback((pathKey: string, hunks: DiffHunk[]) => {
    expandedHunksRef.current.set(pathKey, hunks);
  }, []);

  const fileExpandedHunksHandlers = useMemo(() => {
    const map = new Map<string, (hunks: DiffHunk[]) => void>();
    for (const diff of parsedDiffs) {
      const pathKey = getDiffPathKey(diff);
      map.set(pathKey, (hunks: DiffHunk[]) => handleExpandedHunksChange(pathKey, hunks));
    }
    return map;
  }, [parsedDiffs, handleExpandedHunksChange]);

  // Stable per-file registration callbacks for synthetic hunk insertion
  const fileRegisterInsertSyntheticHandlers = useMemo(() => {
    const map = new Map<string, (handler: (hunk: DiffHunk, insertIndex: number) => void) => void>();
    for (const diff of parsedDiffs) {
      const pathKey = getDiffPathKey(diff);
      map.set(pathKey, (handler: (hunk: DiffHunk, insertIndex: number) => void) => {
        insertSyntheticHandlers.current.set(pathKey, handler);
      });
    }
    return map;
  }, [parsedDiffs]);

  // Stable per-file expansion contexts — avoids creating new objects in the render loop
  const fileExpansionContexts = useMemo(() => {
    const map = new Map<string, ExpansionContext>();
    for (const diff of parsedDiffs) {
      const pathKey = getDiffPathKey(diff);
      map.set(pathKey, {
        repoPath: workingDirectory,
        filePath: pathKey,
        ref: compareData?.headRef,
      });
    }
    return map;
  }, [parsedDiffs, workingDirectory, compareData?.headRef]);

  // After review data loads, ensure all comments are visible by inserting synthetic hunks
  useEffect(() => {
    if (!visibleComments.length || !parsedDiffs.length) return;

    // Group comments by file and ensure visibility for each
    for (const [pathKey, fileComments] of commentsByFile) {
      const handler = insertSyntheticHandlers.current.get(pathKey);
      if (!handler) continue;

      // Always use the best available hunks: prefer expanded hunks, fall back to parsed diff hunks
      const expanded = expandedHunksRef.current.get(pathKey);
      const diff = parsedDiffs.find((d) => getDiffPathKey(d) === pathKey);
      const currentHunks = (expanded && expanded.length > 0) ? expanded : (diff?.hunks ?? []);
      if (currentHunks.length === 0) continue;

      for (const comment of fileComments) {
        const pos = comment.line.to;
        let visible = false;
        for (const hunk of currentHunks) {
          for (const line of hunk.lines) {
            if (pos.side === "L" && line.oldLineNumber === pos.line) { visible = true; break; }
            if (pos.side === "R" && line.newLineNumber === pos.line) { visible = true; break; }
          }
          if (visible) break;
        }
        if (!visible) {
          const ref = pos.side === "L" ? compareData?.baseRef : compareData?.headRef;
          // L-side fetch wants the path that exists at baseRef (oldPath when
          // present, else c.file). R-side fetch wants the canonical key so
          // pre-rename comments still resolve to the new name at headRef.
          const file = pos.side === "L"
            ? (comment.oldPath ?? comment.file)
            : pathKey;
          const start = Math.max(1, pos.line - 3);
          const end = pos.line + 3;

          fetchFileLinesForSynthetic(
            workingDirectory, file, start, end, ref, currentHunks, pos, handler,
            markSyntheticFailed, comment.id,
          );
        }
      }
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visibleComments, parsedDiffs, compareData?.headRef, compareData?.baseRef]);

  // Stable per-file onLineClick handlers — avoids creating new closures in the render loop
  const fileLineClickHandlers = useMemo(() => {
    const map = new Map<string, (position: DiffPosition) => void>();
    for (const diff of parsedDiffs) {
      const pathKey = getDiffPathKey(diff);
      map.set(pathKey, (position: DiffPosition) => handleLineClick(pathKey, position));
    }
    return map;
  }, [parsedDiffs, handleLineClick]);

  // Stable activeCommentLine object — only changes when the actual values change
  const activeCommentLine = useMemo(
    () => activeComment ? { position: activeComment.position } : null,
    [activeComment?.position?.side, activeComment?.position?.line],
  );

  const handleAddComment = useCallback((body: string) => {
    const activeComment = activeCommentRef.current;
    if (!activeComment) return;

    const hunks = getHunksForFile(activeComment.file);
    const pos = activeComment.position;
    let snippet = "";
    let snippetContext = "";

    // Find the anchor line's content
    for (const hunk of hunks) {
      for (const line of hunk.lines) {
        const match = pos.side === "L"
          ? line.oldLineNumber === pos.line
          : line.newLineNumber === pos.line;
        if (match) {
          snippet = line.content;
          break;
        }
      }
      if (snippet) break;
    }

    // Extract same-side snippet context from contiguous line numbers across hunks.
    // Uses ±10 lines to match the backend's context-matching window size.
    // Only includes lines whose line numbers are consecutive on the anchor's side,
    // so non-contiguous hunks don't produce a misleading context string.
    if (snippet) {
      // Collect all same-side (lineNum, content) pairs across hunks
      const allLines: { num: number; content: string }[] = [];
      let anchorIdx = -1;
      for (const hunk of hunks) {
        for (const line of hunk.lines) {
          const lineNum = pos.side === "L" ? line.oldLineNumber : line.newLineNumber;
          if (lineNum != null) {
            allLines.push({ num: lineNum, content: line.content });
            if (lineNum === pos.line) {
              anchorIdx = allLines.length - 1;
            }
          }
        }
      }
      if (anchorIdx >= 0) {
        // Walk backward from anchor, stopping when line numbers are non-consecutive
        let cStart = anchorIdx;
        for (let i = anchorIdx - 1; i >= 0 && anchorIdx - i <= 10; i--) {
          if (allLines[i + 1].num - allLines[i].num !== 1) break;
          cStart = i;
        }
        // Walk forward from anchor, stopping when line numbers are non-consecutive
        let cEnd = anchorIdx;
        for (let i = anchorIdx + 1; i < allLines.length && i - anchorIdx <= 10; i++) {
          if (allLines[i].num - allLines[i - 1].num !== 1) break;
          cEnd = i;
        }
        snippetContext = allLines.slice(cStart, cEnd + 1).map((l) => l.content).join("\n");
      }
    }

    // Determine oldPath for renames
    let oldPath: string | undefined;
    if (pos.side === "L") {
      const diff = parsedDiffs.find((d) => getDiffPathKey(d) === activeComment.file);
      if (diff?.isRenamed) {
        oldPath = diff.oldFile;
      }
    }

    const newComment: ReviewComment = {
      id: `rc_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
      file: activeComment.file,
      ...(oldPath ? { oldPath } : {}),
      line: { from: pos, to: pos },
      snippet,
      ...(snippetContext ? { snippetContext } : {}),
      body,
      submitted: false,
      createdAt: new Date().toISOString(),
    };

    saveAndUpdate((prev) => ({
      ...prev,
      comments: [...prev.comments, newComment],
    }));
    setActiveComment(null);
  }, [getHunksForFile, saveAndUpdate, parsedDiffs]);

  const handleDeleteComment = useCallback((id: string) => {
    saveAndUpdate((prev) => ({
      ...prev,
      comments: prev.comments.filter((c) => c.id !== id),
    }));
  }, [saveAndUpdate]);

  const handleEditComment = useCallback(
    (id: string, body: string) => {
      saveAndUpdate((prev) => ({
        ...prev,
        comments: prev.comments.map((c) =>
          c.id === id
            ? { ...c, body, submitted: false }
            : c,
        ),
      }));
    },
    [saveAndUpdate],
  );

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

  // Unified scroll-to-selected-file effect. Runs in useLayoutEffect AFTER the
  // commit that applies forceMount to predecessor diffs (see renderDiffs
  // below), so preceding LazyFileDiff instances have real heights before we
  // measure. Uses behavior: "auto" (instant) because smooth scrolling during
  // active reflow is what caused the original drift-to-wrong-file bug —
  // placeholders kept inflating mid-animation and pushed the target down.
  useLayoutEffect(() => {
    const kind = lastScrollTargetKindRef.current;
    const key = kind === "outOfDiff" ? selectedOutOfDiffKey : selectedPath;
    if (!key) return;
    // Drop the pending flag on any early-return path so predecessors don't
    // stay in the expensive all-mounted state if the effect can't execute
    // the scroll (mobile back-nav, target not in the current diff set, etc).
    if (isMobile && !mobileShowDiffs) {
      setIsScrollPending(false);
      return;
    }
    const el = kind === "outOfDiff"
      ? outOfDiffRefs.current.get(key)
      : diffRefs.current.get(key);
    if (!el) {
      setIsScrollPending(false);
      return;
    }
    const rid = ++scrollToFileRequestRef.current;
    // One rAF lets the browser finalize layout for newly-mounted children
    // before we measure. useLayoutEffect alone is not enough because
    // ExpandableUnifiedDiff may render its hunks in a secondary effect.
    let releaseHandle: number | null = null;
    const handle = requestAnimationFrame(() => {
      if (rid !== scrollToFileRequestRef.current) return;
      el.scrollIntoView({ behavior: "auto", block: "start" });
      // Wait one more frame before clearing isScrollPending so useLazyMount's
      // IntersectionObserver has time to latch successors that came into view
      // with the scroll. Without that delay, those successors unmount back to
      // 44px placeholders in the same paint cycle, scrollHeight shrinks, and
      // the browser can re-clamp scrollTop short of the target.
      releaseHandle = requestAnimationFrame(() => {
        if (rid !== scrollToFileRequestRef.current) return;
        setIsScrollPending(false);
      });
    });
    return () => {
      cancelAnimationFrame(handle);
      if (releaseHandle !== null) cancelAnimationFrame(releaseHandle);
    };
  }, [scrollRequestId, selectedPath, selectedOutOfDiffKey, isMobile, mobileShowDiffs]);

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
    <div className="text-muted-foreground border-border/50 flex flex-wrap items-center gap-x-2 gap-y-1 border-b px-3 py-1.5 text-xs">
      <span>
        {compareData.files.length} file{compareData.files.length !== 1 ? "s" : ""} changed
        {compareData.totalAdditions > 0 && (
          <span className="ml-2 text-green-500">+{compareData.totalAdditions}</span>
        )}
        {compareData.totalDeletions > 0 && (
          <span className="ml-1 text-red-500">-{compareData.totalDeletions}</span>
        )}
      </span>
      {compareData.baseBehindBy > 0 && baseBranch && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="inline-flex cursor-help items-center gap-1 rounded border border-amber-500/40 bg-amber-500/10 px-1.5 py-0.5 text-[11px] text-amber-600 dark:text-amber-400">
              <AlertTriangle className="h-3 w-3" />
              {baseBranch} is {compareData.baseBehindBy} commit
              {compareData.baseBehindBy === 1 ? "" : "s"} behind {compareData.baseUpstream}
            </span>
          </TooltipTrigger>
          <TooltipContent side="bottom" className="max-w-xs">
            Diff is computed against <code>{compareData.baseUpstream}</code>. Pull the latest in this worktree to compare against local <code>{baseBranch}</code> instead.
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  ) : null;

  const compareErrorView = (
    <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
      <AlertCircle className="mb-4 h-12 w-12 opacity-50" />
      <p className="text-sm">
        {compareErrorDetail instanceof Error
          ? compareErrorDetail.message
          : "Failed to compare branches"}
      </p>
    </div>
  );

  const hasChangedFiles = !!compareData?.files.length;
  const hasOutOfDiff = outOfDiffGroups.length > 0;
  const fileList = hasChangedFiles || hasOutOfDiff ? (
    <div className="flex-1 overflow-y-auto">
      {hasChangedFiles && (
        <div>
          {compareData!.files.map((file) => (
            <CompareFileRow
              key={file.path}
              file={file}
              isSelected={selectedPath === file.path}
              onClick={() => scrollToFile(file.path)}
            />
          ))}
        </div>
      )}
      <OutOfDiffSection
        groups={outOfDiffGroups}
        selectedKey={selectedOutOfDiffKey ?? undefined}
        onFileClick={scrollToOutOfDiff}
      />
    </div>
  ) : null;

  // --- Shared diff rendering helper ---
  const renderDiffs = (wrapLines = true, showAddComment = true, editCommentRequestHandler?: (comment: ReviewComment) => void) => (
    <div className="space-y-3 pt-3">
      {reviewData?.body?.body && (
        <ReviewBodyCard body={reviewData.body} onDelete={handleDeleteBody} />
      )}
      {parsedDiffs.map((diff, idx) => {
        const pathKey = getDiffPathKey(diff);
        const fileName = getDiffFileName(diff);
        const fileComments = commentsByFile.get(pathKey) ?? EMPTY_COMMENTS;
        const fileActiveCommentLine = activeComment?.file === pathKey ? activeCommentLine : null;
        // Persistent force-mount for predecessors up to and including the
        // selected file so their heights are real BEFORE scrollIntoView.
        const inScrollTargetRange = selectedIdx >= 0 && idx <= selectedIdx;
        return (
          <div key={pathKey} ref={setDiffRef(pathKey)}>
            <LazyFileDiff
              diff={diff}
              fileName={fileName}
              wrapLines={wrapLines}
              // isScrollPending extends force-mount to files AFTER the target
              // during the scroll so scrollHeight is large enough for the
              // scroll to land. Released immediately after the scroll.
              forceMount={
                commentsByFile.has(pathKey) ||
                inScrollTargetRange ||
                isScrollPending
              }
              comments={fileComments}
              activeCommentLine={fileActiveCommentLine}
              onLineClick={fileLineClickHandlers.get(pathKey)}
              onAddComment={showAddComment ? handleAddComment : undefined}
              onCancelComment={showAddComment ? clearActiveComment : undefined}
              onDeleteComment={handleDeleteComment}
              onEditComment={handleEditComment}
              onEditCommentRequest={editCommentRequestHandler}
              onCommentRef={setCommentRef}
              totalLines={compareData?.totalLines[pathKey] ?? 0}
              onExpandedHunksChange={fileExpandedHunksHandlers.get(pathKey)}
              onRegisterInsertSynthetic={fileRegisterInsertSyntheticHandlers.get(pathKey)}
              expansionContext={fileExpansionContexts.get(pathKey)!}
            />
          </div>
        );
      })}
      {outOfDiffGroups.map((g) => {
        const first = g.comments[0];
        return (
          <div key={g.key} ref={setOutOfDiffRef(g.key)}>
            <OutOfDiffFile
              workingDirectory={workingDirectory}
              groupKey={g.key}
              displayFile={g.displayFile}
              file={first.file}
              oldPath={first.oldPath}
              comments={g.comments}
              headRef={compareData?.headRef ?? ""}
              baseRef={compareData?.baseRef ?? ""}
              onDeleteComment={handleDeleteComment}
              onEditComment={handleEditComment}
              onEditCommentRequest={editCommentRequestHandler}
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
        compareErrorView
      ) : parsedDiffs.length === 0 && outOfDiffGroups.length === 0 ? (
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
            compareErrorView
          ) : (
            <div className="p-3">{renderDiffs(false, false, handleEditCommentRequest)}</div>
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
                isStale={isFocusStale}
              />
            </div>
          </div>
        )}
        <MobileCommentSheet
          activeComment={activeComment}
          activeLines={activeComment ? (() => {
            const hunks = getHunksForFile(activeComment.file);
            const pos = activeComment.position;
            const lines: DiffLine[] = [];
            for (const hunk of hunks) {
              for (const line of hunk.lines) {
                const match = pos.side === "L"
                  ? line.oldLineNumber === pos.line
                  : line.newLineNumber === pos.line;
                if (match) {
                  lines.push(line);
                }
              }
            }
            return lines;
          })() : []}
          onAddComment={handleAddComment}
          onCancel={clearActiveComment}
          editingComment={editingComment}
          onEditComment={handleEditComment}
          onCancelEdit={clearEditingComment}
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
            compareErrorView
          ) : hasChangedFiles || hasOutOfDiff ? (
            <>
              {hasChangedFiles &&
                compareData!.files.map((file) => (
                  <CompareFileRow
                    key={file.path}
                    file={file}
                    isSelected={selectedPath === file.path}
                    onClick={() => scrollToFile(file.path)}
                  />
                ))}
              <OutOfDiffSection
                groups={outOfDiffGroups}
                selectedKey={selectedOutOfDiffKey ?? undefined}
                onFileClick={scrollToOutOfDiff}
              />
            </>
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
          <div className="border-border sticky top-0 z-30 flex items-center gap-2 border-b bg-inherit px-3 py-2">
            <CommentNav
              currentIndex={focusedCommentIdx}
              total={sortedComments.length}
              onPrev={handlePrevComment}
              onNext={handleNextComment}
              isStale={isFocusStale}
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
