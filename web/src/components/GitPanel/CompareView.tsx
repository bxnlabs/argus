import { useState, useRef, useCallback, useMemo, useEffect, useLayoutEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
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
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { LazyFileDiff } from "@/components/DiffViewer/LazyFileDiff";
import { getDiffFileName, getDiffPathKey, parseMultiFileDiff, type DiffLine, type DiffHunk } from "@/lib/diff-parser";
import {
  partitionComments,
  sortCommentsByRenderOrder,
  sortUnanchoredCommentsByFile,
  coalesceAutoExpand,
  type AutoExpandTarget,
  type InlineCommentEntry,
} from "@/lib/compare-comments";
import { useCompareBranchesQuery, useCompareQuery, useGitCurrentBranchQuery } from "@/data/git";
import { useReviewQuery, useSaveReviewMutation, reviewKeys } from "@/data/review";
import { ReviewSubmitButton } from "./ReviewSubmitButton";
import { ReviewBodyCard } from "./ReviewBodyCard";
import { CommentNav } from "./CommentNav";
import { MobileCommentSheet } from "./MobileCommentSheet";
import { UnanchoredCommentSection } from "./UnanchoredCommentSection";
import { useViewport } from "@/hooks/useViewport";
import { type ExpansionContext } from "@/hooks/useExpandableDiff";
import type { CommitFile, FileStatus, ReviewComment, Review, DiffPosition } from "@/types";

const EMPTY_COMMENTS: ReviewComment[] = [];

/** Context window expanded around a caseB anchor on mount. */
const AUTO_EXPAND_RADIUS = 3;


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
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const expandedHunksRef = useRef<Map<string, DiffHunk[]>>(new Map());
  const unanchoredSectionRef = useRef<HTMLDivElement>(null);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  // Bumped on each scroll-to-file request so repeat clicks on the same path
  // re-trigger the scroll effect.
  const [scrollRequestId, setScrollRequestId] = useState(0);
  // Bumped to request a scroll to the unanchored section (from the sidebar).
  const [unanchoredScrollReq, setUnanchoredScrollReq] = useState(0);
  const scrollToFileRequestRef = useRef(0);
  // True while a scroll-to-file is in flight. Force-mounts every diff so
  // scrollHeight exceeds target.offsetTop + clientHeight; otherwise the
  // browser clamps scrollTop short of the target when files after it are
  // still lazy placeholders. Cleared one frame after the scroll lands.
  const [isScrollPending, setIsScrollPending] = useState(false);
  const [activeComment, setActiveComment] = useState<{
    file: string;
    position: DiffPosition;
  } | null>(null);
  const activeCommentRef = useRef(activeComment);
  activeCommentRef.current = activeComment;
  const [editingComment, setEditingComment] = useState<ReviewComment | null>(null);
  // caseB comments whose auto-expansion failed (EOF/empty/fetch error). They
  // have no inline home, so they fall back to the unanchored section.
  const [failedCommentIds, setFailedCommentIds] = useState<Set<string>>(() => new Set());

  const {
    data: branchData,
    isLoading: loadingBranches,
    isError: branchError,
    error: branchErrorDetail,
  } = useCompareBranchesQuery(workingDirectory);

  // Reset repo-scoped state when working directory changes.
  useEffect(() => {
    setBaseBranch(null);
    setSelectedPath(null);
    setIsScrollPending(false);
    setEditingComment(null);
    setFocusedCommentId(null);
    setFailedCommentIds(new Set());
    lastFocusedIdxRef.current = -1;
    pendingScrollIdRef.current = null;
    diffRefs.current.clear();
    expandedHunksRef.current.clear();
  }, [workingDirectory]);

  // A base-branch switch changes the comparison and remounts file diffs, so a
  // navigation target recorded against the previous compare is stale. Clear it
  // so a same-id comment in the new compare can't trigger an unexpected scroll.
  useEffect(() => {
    pendingScrollIdRef.current = null;
  }, [baseBranch]);

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

  const parsedDiffs = useMemo(() => {
    // Clear stale expanded hunks when diff data changes (e.g. base branch switch)
    expandedHunksRef.current.clear();
    return compareData?.diff ? parseMultiFileDiff(compareData.diff) : [];
  }, [compareData?.diff]);

  const { data: reviewData } = useReviewQuery(
    workingDirectory,
    currentBranch,
    baseBranch,
    compareData?.headRef,
    compareData?.baseRef,
  );

  // Index of the selected file in the rendered diff order. -1 when nothing is
  // selected, or when the selected path is no longer present in parsedDiffs
  // (e.g. after the base branch changes).
  const selectedIdx = useMemo(() => {
    if (!selectedPath) return -1;
    return parsedDiffs.findIndex((d) => getDiffPathKey(d) === selectedPath);
  }, [selectedPath, parsedDiffs]);

  const saveReview = useSaveReviewMutation(workingDirectory);

  const comments = reviewData?.comments ?? EMPTY_COMMENTS;

  // Partition comments against the current compare data so the view can
  // render inline (anchored + caseB) vs. unanchored buckets distinctly.
  // caseB comments live in a file's diff but on a line not yet in any hunk;
  // the auto-expand wiring below surfaces them inline by expanding context.
  const partition = useMemo(
    () => partitionComments(parsedDiffs, compareData?.totalLines ?? {}, comments),
    [parsedDiffs, compareData?.totalLines, comments],
  );

  // Drop auto-expand failures whenever the diff data changes so each compare
  // re-evaluates fresh (keeping the empty set's reference avoids a re-render).
  useEffect(() => {
    setFailedCommentIds((prev) => (prev.size === 0 ? prev : new Set()));
  }, [parsedDiffs]);

  // Apply auto-expand failures: caseB comments that couldn't be surfaced inline
  // move to the unanchored section so none silently disappear.
  const effectivePartition = useMemo(() => {
    if (failedCommentIds.size === 0) return partition;
    const caseB: InlineCommentEntry[] = [];
    const moved: ReviewComment[] = [];
    for (const e of partition.caseB) {
      if (failedCommentIds.has(e.comment.id)) moved.push(e.comment);
      else caseB.push(e);
    }
    if (moved.length === 0) return partition;
    return {
      anchored: partition.anchored,
      caseB,
      unanchored: [...partition.unanchored, ...moved],
    };
  }, [partition, failedCommentIds]);

  // Pre-indexed inline comments (anchored + caseB) by file path-key — the key
  // `LazyFileDiff` uses for its `comments` prop. Entries already carry the
  // resolved path-key from partitioning, so there's no side+path re-derivation.
  const commentsByFile = useMemo(() => {
    const next = new Map<string, ReviewComment[]>();
    const push = (key: string, c: ReviewComment) => {
      const arr = next.get(key);
      if (arr) arr.push(c);
      else next.set(key, [c]);
    };
    for (const e of effectivePartition.anchored) push(e.pathKey, e.comment);
    for (const e of effectivePartition.caseB) push(e.pathKey, e.comment);
    return next;
  }, [effectivePartition.anchored, effectivePartition.caseB]);

  // Coalesced auto-expand windows per file. caseB entries already carry the
  // resolved new-side line; nearby anchors are merged so their context windows
  // never overlap (which would duplicate context or hide a comment).
  const autoExpandTargetsByFile = useMemo(() => {
    const anchorsByKey = new Map<string, { line: number; commentId: string }[]>();
    for (const e of effectivePartition.caseB) {
      if (e.autoExpandLine == null) continue;
      const item = { line: e.autoExpandLine, commentId: e.comment.id };
      const arr = anchorsByKey.get(e.pathKey);
      if (arr) arr.push(item);
      else anchorsByKey.set(e.pathKey, [item]);
    }
    // Coalesce against each file's real hunks so two anchors bracketing a hunk
    // aren't merged into a window centered inside it (which would silently drop
    // both comments — see coalesceAutoExpand).
    const hunksByKey = new Map<string, DiffHunk[]>();
    for (const d of parsedDiffs) hunksByKey.set(getDiffPathKey(d), d.hunks);
    const map = new Map<string, AutoExpandTarget[]>();
    for (const [key, anchors] of anchorsByKey) {
      map.set(key, coalesceAutoExpand(anchors, AUTO_EXPAND_RADIUS, hunksByKey.get(key) ?? []));
    }
    return map;
  }, [effectivePartition.caseB, parsedDiffs]);

  // caseB comments whose auto-expansion can't cover their anchor route to the
  // unanchored section. The child reports the affected comment IDs directly
  // (carried on the target), so there's no center-line reverse lookup.
  const handleAutoExpandFailed = useCallback((commentIds: string[]) => {
    setFailedCommentIds((prev) => {
      const next = new Set(prev);
      for (const id of commentIds) next.add(id);
      return next;
    });
  }, []);

  // Exact key of the review query this view reads (must match useReviewQuery's
  // key, including the headRef/baseRef suffix), so optimistic writes land on the
  // entry the component renders from.
  const reviewQueryKey = useMemo(
    () => [
      ...reviewKeys.forComparison(workingDirectory, currentBranch ?? "", baseBranch ?? ""),
      compareData?.headRef ?? "",
      compareData?.baseRef ?? "",
    ],
    [workingDirectory, currentBranch, baseBranch, compareData?.headRef, compareData?.baseRef],
  );

  // Compute a new Review payload, optimistically write it to the query cache,
  // then persist. Reading and writing the live cache (rather than the render-time
  // `reviewData` snapshot) is what makes rapid sequential edits compose: each
  // mutation sees the previous one's result even before the save's invalidation
  // round-trips. Without it, two quick edits both derive from the same stale
  // snapshot and the second POST clobbers the first.
  // Returns whether the write was applied. Callers that capture fresh user
  // input (e.g. a new comment) should keep their editor open on a `false`
  // result so the input isn't silently lost while the review is still loading.
  const saveAndUpdate = useCallback((updater: (prev: Review) => Review): boolean => {
    if (!currentBranch || !baseBranch) return false;
    // Bail until the review GET has settled. The backend returns an empty
    // review (200) when none exists, so a settled query always yields a
    // defined cache; `cached` is undefined only mid-load. Writing from an
    // empty fallback in that window would POST a review containing only the
    // new edit and clobber the existing on-disk comments (which aren't even
    // rendered yet because reviewData is still undefined).
    const cached = queryClient.getQueryData<Review>(reviewQueryKey);
    if (!cached) return false;
    const newReview = updater(cached);
    queryClient.setQueryData(reviewQueryKey, newReview);
    saveReview.mutate(newReview);
    return true;
  }, [queryClient, reviewQueryKey, currentBranch, baseBranch, saveReview]);

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
  // A navigation target whose inline element hasn't mounted yet (e.g. a caseB
  // comment whose auto-expanded hunk is still being inserted). setCommentRef
  // finishes the scroll once the element registers.
  const pendingScrollIdRef = useRef<string | null>(null);

  const sortedComments = useMemo(
    () => sortCommentsByRenderOrder(parsedDiffs, compareData?.files ?? [], effectivePartition),
    [parsedDiffs, compareData?.files, effectivePartition],
  );

  const unanchoredSorted = useMemo(
    () => sortUnanchoredCommentsByFile(effectivePartition.unanchored, compareData?.files ?? []),
    [effectivePartition.unanchored, compareData?.files],
  );

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

  const scrollToComment = useCallback((index: number) => {
    const comment = sortedComments[index];
    if (!comment) return;
    setFocusedCommentId(comment.id);
    lastFocusedIdxRef.current = index;
    const el = commentRefs.current.get(comment.id);
    if (el) {
      pendingScrollIdRef.current = null;
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    // The inline element isn't mounted yet (e.g. a caseB comment whose
    // auto-expanded hunk is still being inserted). Remember it so setCommentRef
    // can finish the scroll once it registers, and nudge to the file meanwhile.
    pendingScrollIdRef.current = comment.id;
    const fileEl = diffRefs.current.get(comment.file);
    if (fileEl) {
      fileEl.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [sortedComments]);

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
      // Finish a navigation that targeted this comment before it had mounted.
      if (pendingScrollIdRef.current === id) {
        pendingScrollIdRef.current = null;
        el.scrollIntoView({ behavior: "smooth", block: "center" });
      }
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
    setIsScrollPending(true);
    if (isMobile) {
      setMobileShowDiffs(true);
    }
    // Bumping the request id triggers the unified scroll effect below, which
    // runs in useLayoutEffect AFTER the force-mount propagation commits.
    // Re-firing on repeat clicks to the same path is intentional.
    setScrollRequestId((n) => n + 1);
  }, [isMobile]);

  // Jump to the unanchored-comments section from the sidebar. On mobile this
  // also flips to the diff view (where the section is rendered); the scroll
  // itself runs in the effect below once the section is mounted.
  const scrollToUnanchored = useCallback(() => {
    if (isMobile) setMobileShowDiffs(true);
    setUnanchoredScrollReq((n) => n + 1);
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

    const saved = saveAndUpdate((prev) => ({
      ...prev,
      comments: [...prev.comments, newComment],
    }));
    // Keep the editor open if the write was skipped (review still loading), so
    // the user's just-typed comment isn't discarded without feedback.
    if (saved) setActiveComment(null);
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
    if (!selectedPath) return;
    // Drop the pending flag on any early-return path so predecessors don't
    // stay in the expensive all-mounted state if the effect can't execute
    // the scroll (mobile back-nav, target not in the current diff set, etc).
    if (isMobile && !mobileShowDiffs) {
      setIsScrollPending(false);
      return;
    }
    const el = diffRefs.current.get(selectedPath);
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
  }, [scrollRequestId, selectedPath, isMobile, mobileShowDiffs]);

  // Scroll to the unanchored section when requested from the sidebar. The
  // section is always rendered (not lazy), so a single rAF to let the layout
  // settle — including a just-mounted mobile diff view — is enough.
  useLayoutEffect(() => {
    if (unanchoredScrollReq === 0) return;
    if (!unanchoredSectionRef.current) return;
    const handle = requestAnimationFrame(() => {
      unanchoredSectionRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    });
    return () => cancelAnimationFrame(handle);
  }, [unanchoredScrollReq]);

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
  const fileList = hasChangedFiles || unanchoredSorted.length > 0 ? (
    <div className="flex-1 overflow-y-auto">
      <div>
        {compareData?.files.map((file) => (
          <CompareFileRow
            key={file.path}
            file={file}
            isSelected={selectedPath === file.path}
            onClick={() => scrollToFile(file.path)}
          />
        ))}
      </div>
      {unanchoredSorted.length > 0 && (
        <UnanchoredNavRow
          count={unanchoredSorted.length}
          onClick={scrollToUnanchored}
        />
      )}
    </div>
  ) : null;

  // Comments with no inline anchor in the current compare. Rendered below the
  // diff list, and also surfaced standalone when there are no changed files at
  // all (otherwise the only path to them — drilling into a diff — wouldn't
  // exist). Delete-only: a comment whose code is gone has no live location to
  // re-edit against, so editing is intentionally not offered here.
  const unanchoredSection =
    unanchoredSorted.length > 0 ? (
      <div ref={unanchoredSectionRef}>
        <UnanchoredCommentSection
          comments={unanchoredSorted}
          onDeleteComment={handleDeleteComment}
          onCommentRef={setCommentRef}
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
        const fileAutoExpandTargets = autoExpandTargetsByFile.get(pathKey);
        // Persistent force-mount for predecessors up to and including the
        // selected file so their heights are real BEFORE scrollIntoView.
        const inScrollTargetRange = selectedIdx >= 0 && idx <= selectedIdx;
        return (
          <div
            key={`${pathKey}@${compareData?.headRef ?? ""}~${compareData?.baseRef ?? ""}`}
            ref={setDiffRef(pathKey)}
          >
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
              expansionContext={fileExpansionContexts.get(pathKey)!}
              autoExpandTargets={fileAutoExpandTargets}
              onAutoExpandFailed={handleAutoExpandFailed}
            />
          </div>
        );
      })}
      {unanchoredSection}
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
      ) : parsedDiffs.length === 0 && !unanchoredSection ? (
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
          {/* Gate on a settled review: until reviewData loads, saveAndUpdate
              no-ops, so a typed review message would be silently dropped (and
              ReviewSubmitButton's generalComment effect would overwrite the
              draft once the GET resolves). */}
          {baseBranch && reviewData && (
            <ReviewSubmitButton
              pendingCount={pendingCount}
              generalComment={reviewData.body?.body ?? ""}
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
          ) : hasChangedFiles ? (
            <>
              {compareData!.files.map((file) => (
                <CompareFileRow
                  key={file.path}
                  file={file}
                  isSelected={selectedPath === file.path}
                  onClick={() => scrollToFile(file.path)}
                />
              ))}
              {unanchoredSorted.length > 0 && (
                <UnanchoredNavRow
                  count={unanchoredSorted.length}
                  onClick={scrollToUnanchored}
                />
              )}
            </>
          ) : unanchoredSection ? (
            <div className="p-3">{unanchoredSection}</div>
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
            {/* Gate on a settled review: until reviewData loads, saveAndUpdate
                no-ops, so a typed review message would be silently dropped (and
                ReviewSubmitButton's generalComment effect would overwrite the
                draft once the GET resolves). CommentNav stays visible. */}
            {reviewData && (
              <ReviewSubmitButton
                pendingCount={pendingCount}
                generalComment={reviewData.body?.body ?? ""}
                onGeneralCommentChange={handleGeneralCommentChange}
                onSubmit={handleSubmitComments}
              />
            )}
          </div>
        )}
        {diffPane}
      </div>
    </div>
  );
}

function UnanchoredNavRow({ count, onClick }: { count: number; onClick: () => void }) {
  return (
    <button
      onClick={onClick}
      className="hover:bg-muted/70 mt-1 flex w-full items-center gap-2 border-t-2 border-yellow-500/40 px-3 py-2 text-left transition-colors"
    >
      <AlertTriangle className="h-4 w-4 flex-shrink-0 text-yellow-500" />
      <span className="flex-1 truncate text-sm">Unanchored comments</span>
      <span className="text-muted-foreground text-xs">{count}</span>
    </button>
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
