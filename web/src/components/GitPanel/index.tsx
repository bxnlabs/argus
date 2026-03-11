import { useState, useCallback, useRef, useMemo, useEffect } from "react";
import {
  GitBranch,
  RefreshCw,
  Loader2,
  AlertCircle,
  ArrowUp,
  ArrowDown,
  ArrowLeft,
  FileCode,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { FileChanges } from "./FileChanges";
import { GitPanelTabs, type GitTab } from "./GitPanelTabs";
import { CommitHistory } from "./CommitHistory";
import { CompareView } from "./CompareView";
import { UnifiedDiff } from "@/components/DiffViewer/UnifiedDiff";
import {
  parseMultiFileDiff,
  getDiffFileName,
  getDiffPathKey,
} from "@/lib/diff-parser";
import { useViewport } from "@/hooks/useViewport";
import { useGitStatusQuery, useWorkingDiffQuery } from "@/data/git";
import type { GitFile } from "@/types";

interface GitPanelProps {
  workingDirectory: string;
}

export function GitPanel({ workingDirectory }: GitPanelProps) {
  const { isMobile } = useViewport();
  const [activeTab, setActiveTab] = useState<GitTab>("changes");

  const {
    data: status,
    isPending: loading,
    isError,
    error,
    isRefetching,
    refetch,
  } = useGitStatusQuery(workingDirectory);

  // Working-tree diff (full stacked diff for all changes)
  const {
    data: workingDiffData,
    isLoading: loadingDiff,
    isError: isDiffError,
    error: diffError,
    refetch: refetchDiff,
  } = useWorkingDiffQuery(workingDirectory, {
    enabled: activeTab === "changes",
  });

  const parsedDiffs = useMemo(() => {
    if (!workingDiffData?.diff) return [];
    return parseMultiFileDiff(workingDiffData.diff);
  }, [workingDiffData?.diff]);

  // Scroll-to-file state
  const [selectedPath, setSelectedPath] = useState<string | null>(null);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const [mobileShowDiffs, setMobileShowDiffs] = useState(false);

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

  // Clear state when working directory changes
  useEffect(() => {
    setSelectedPath(null);
    setMobileShowDiffs(false);
    diffRefs.current.clear();
  }, [workingDirectory]);

  // Clear stale selection when file disappears from status
  useEffect(() => {
    if (!selectedPath || !status) return;
    const allPaths = new Set([
      ...status.staged.map((f: GitFile) => f.path),
      ...status.unstaged.map((f: GitFile) => f.path),
      ...status.untracked.map((f: GitFile) => f.path),
    ]);
    if (!allPaths.has(selectedPath)) {
      setSelectedPath(null);
    }
  }, [selectedPath, status]);

  // Resizable panel state (desktop)
  const [listWidth, setListWidth] = useState(400);
  const containerRef = useRef<HTMLDivElement>(null);
  const isDragging = useRef(false);

  const handleRefresh = async () => {
    await refetch();
    if (activeTab === "changes") {
      refetchDiff();
    }
  };

  const handleFileClick = useCallback(
    (file: GitFile) => {
      setSelectedPath(file.path);
      if (isMobile) {
        setMobileShowDiffs(true);
      } else {
        const el = diffRefs.current.get(file.path);
        if (el) {
          el.scrollIntoView({ behavior: "smooth", block: "start" });
        }
      }
    },
    [isMobile],
  );

  // Scroll to selected file once diffs are rendered (mobile + desktop fallback)
  useEffect(() => {
    if (!selectedPath) return;
    if (isMobile && !mobileShowDiffs) return;
    const el = diffRefs.current.get(selectedPath);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [selectedPath, parsedDiffs, isMobile, mobileShowDiffs]);

  // Resize handle for desktop
  const handleMouseMoveRef = useRef<((e: MouseEvent) => void) | null>(null);
  const handleMouseUpRef = useRef<(() => void) | null>(null);

  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    isDragging.current = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current || !containerRef.current) return;
      const containerRect = containerRef.current.getBoundingClientRect();
      const newWidth = e.clientX - containerRect.left;
      setListWidth(Math.max(250, Math.min(600, newWidth)));
    };

    const handleMouseUp = () => {
      isDragging.current = false;
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      document.removeEventListener("mousemove", handleMouseMove);
      document.removeEventListener("mouseup", handleMouseUp);
      handleMouseMoveRef.current = null;
      handleMouseUpRef.current = null;
    };

    handleMouseMoveRef.current = handleMouseMove;
    handleMouseUpRef.current = handleMouseUp;
    document.addEventListener("mousemove", handleMouseMove);
    document.addEventListener("mouseup", handleMouseUp);
  }, []);

  useEffect(() => {
    return () => {
      if (handleMouseMoveRef.current) {
        document.removeEventListener("mousemove", handleMouseMoveRef.current);
      }
      if (handleMouseUpRef.current) {
        document.removeEventListener("mouseup", handleMouseUpRef.current);
      }
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      isDragging.current = false;
    };
  }, []);

  if (loading) {
    return (
      <div className="bg-background flex h-full w-full flex-col">
        <Header
          branch=""
          ahead={0}
          behind={0}
          onRefresh={handleRefresh}
          refreshing={false}
        />
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="bg-background flex h-full w-full flex-col">
        <Header
          branch=""
          ahead={0}
          behind={0}
          onRefresh={handleRefresh}
          refreshing={isRefetching}
        />
        <div className="flex flex-1 flex-col items-center justify-center p-4">
          <AlertCircle className="text-muted-foreground mb-2 h-8 w-8" />
          <p className="text-muted-foreground text-center text-sm">
            {error?.message ?? "Failed to load git status"}
          </p>
        </div>
      </div>
    );
  }

  if (!status) return null;

  const hasChanges =
    status.staged.length > 0 ||
    status.unstaged.length > 0 ||
    status.untracked.length > 0;

  const compareHeader = (
    <>
      <Header
        branch={status.branch}
        ahead={status.ahead}
        behind={status.behind}
        onRefresh={handleRefresh}
        refreshing={isRefetching}
      />
      <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />
    </>
  );

  const stackedDiffs = (
    <div className="space-y-3 p-3">
      {parsedDiffs.map((diff) => {
        const pathKey = getDiffPathKey(diff);
        const fileName = getDiffFileName(diff);
        return (
          <div key={pathKey} ref={setDiffRef(pathKey)}>
            <UnifiedDiff diff={diff} fileName={fileName} expanded={selectedPath === pathKey} />
          </div>
        );
      })}
    </div>
  );

  // --- Mobile layout ---
  if (isMobile) {
    // Compare tab
    if (activeTab === "compare") {
      return (
        <div className="bg-background relative flex h-full w-full flex-col">
          <CompareView
            workingDirectory={workingDirectory}
            currentBranch={status.branch}
            header={compareHeader}
          />
        </div>
      );
    }

    // History tab
    if (activeTab === "history") {
      return (
        <div className="bg-background flex h-full w-full flex-col">
          <CommitHistory
            workingDirectory={workingDirectory}
            header={
              <>
                <Header
                  branch={status.branch}
                  ahead={status.ahead}
                  behind={status.behind}
                  onRefresh={handleRefresh}
                  refreshing={isRefetching}
                />
                <GitPanelTabs
                  activeTab={activeTab}
                  onTabChange={setActiveTab}
                />
              </>
            }
          />
        </div>
      );
    }

    // Changes tab: full-screen stacked diff view when user taps a file
    if (mobileShowDiffs) {
      return (
        <div className="bg-background flex h-full w-full flex-col">
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
              <p className="truncate text-sm font-medium">Working Changes</p>
              {workingDiffData && (
                <p className="text-muted-foreground text-xs">
                  {workingDiffData.files.length} file
                  {workingDiffData.files.length !== 1 ? "s" : ""} changed
                </p>
              )}
            </div>
          </div>
          <div className="safe-area-bottom flex-1 overflow-auto">
            {loadingDiff ? (
              <div className="flex h-32 items-center justify-center">
                <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
              </div>
            ) : isDiffError ? (
              <div className="flex h-32 flex-col items-center justify-center p-4">
                <AlertCircle className="text-muted-foreground mb-2 h-6 w-6" />
                <p className="text-muted-foreground text-center text-sm">
                  {diffError?.message ?? "Failed to load diff"}
                </p>
              </div>
            ) : parsedDiffs.length > 0 ? (
              stackedDiffs
            ) : (
              <div className="text-muted-foreground flex h-32 flex-col items-center justify-center p-4">
                <FileCode className="mb-2 h-6 w-6 opacity-60" />
                <p className="text-sm">No diff content to display</p>
              </div>
            )}
          </div>
        </div>
      );
    }

    // Changes tab: file list (default mobile)
    return (
      <div className="bg-background flex h-full w-full flex-col">
        <Header
          branch={status.branch}
          ahead={status.ahead}
          behind={status.behind}
          onRefresh={handleRefresh}
          refreshing={isRefetching}
        />
        <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />
        <div className="safe-area-bottom flex-1 overflow-y-auto">
          {!hasChanges ? (
            <div className="flex h-32 items-center justify-center">
              <p className="text-muted-foreground text-sm">No changes</p>
            </div>
          ) : (
            <div className="py-2">
              {status.staged.length > 0 && (
                <FileChanges
                  files={status.staged}
                  title="Staged Changes"
                  selectedPath={selectedPath ?? undefined}
                  onFileClick={handleFileClick}
                />
              )}
              {status.unstaged.length > 0 && (
                <FileChanges
                  files={status.unstaged}
                  title="Changes"
                  selectedPath={selectedPath ?? undefined}
                  onFileClick={handleFileClick}
                />
              )}
              {status.untracked.length > 0 && (
                <FileChanges
                  files={status.untracked}
                  title="Untracked Files"
                  selectedPath={selectedPath ?? undefined}
                  onFileClick={handleFileClick}
                />
              )}
            </div>
          )}
        </div>
      </div>
    );
  }

  // --- Desktop layout ---

  // Compare tab
  if (activeTab === "compare") {
    return (
      <div ref={containerRef} className="bg-background flex h-full w-full flex-col">
        <CompareView
          workingDirectory={workingDirectory}
          currentBranch={status.branch}
          header={compareHeader}
          listWidth={listWidth}
          onResizeMouseDown={handleMouseDown}
        />
      </div>
    );
  }

  // History tab
  if (activeTab === "history") {
    return (
      <div ref={containerRef} className="bg-background flex h-full w-full flex-col">
        <CommitHistory
          workingDirectory={workingDirectory}
          header={
            <>
              <Header
                branch={status.branch}
                ahead={status.ahead}
                behind={status.behind}
                onRefresh={handleRefresh}
                refreshing={isRefetching}
              />
              <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />
            </>
          }
          listWidth={listWidth}
          onResizeMouseDown={handleMouseDown}
        />
      </div>
    );
  }

  // Changes tab: side-by-side with stacked diffs
  return (
    <div ref={containerRef} className="bg-background flex h-full w-full flex-col">
      <div className="flex min-h-0 flex-1">
        {/* Left panel - file list */}
        <div className="flex h-full min-w-0 flex-col" style={{ width: listWidth }}>
          <Header
            branch={status.branch}
            ahead={status.ahead}
            behind={status.behind}
            onRefresh={handleRefresh}
            refreshing={isRefetching}
          />
          <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />

          <div className="flex-1 overflow-y-auto">
            {!hasChanges ? (
              <div className="flex h-32 items-center justify-center">
                <p className="text-muted-foreground text-sm">No changes</p>
              </div>
            ) : (
              <div className="py-2">
                {status.staged.length > 0 && (
                  <FileChanges
                    files={status.staged}
                    title="Staged Changes"
                    selectedPath={selectedPath ?? undefined}
                    onFileClick={handleFileClick}
                  />
                )}
                {status.unstaged.length > 0 && (
                  <FileChanges
                    files={status.unstaged}
                    title="Changes"
                    selectedPath={selectedPath ?? undefined}
                    onFileClick={handleFileClick}
                  />
                )}
                {status.untracked.length > 0 && (
                  <FileChanges
                    files={status.untracked}
                    title="Untracked Files"
                    selectedPath={selectedPath ?? undefined}
                    onFileClick={handleFileClick}
                  />
                )}
              </div>
            )}
          </div>
        </div>

        {/* Resize handle */}
        <div
          className="bg-muted/50 hover:bg-primary/50 active:bg-primary w-1 flex-shrink-0 cursor-col-resize transition-colors"
          onMouseDown={handleMouseDown}
        />

        {/* Right panel - stacked diffs */}
        <div className="bg-muted/20 flex h-full min-w-0 flex-1 flex-col">
          {loadingDiff ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : isDiffError ? (
            <div className="flex flex-1 flex-col items-center justify-center p-4">
              <AlertCircle className="text-muted-foreground mb-2 h-8 w-8" />
              <p className="text-muted-foreground text-center text-sm">
                {diffError?.message ?? "Failed to load diff"}
              </p>
            </div>
          ) : parsedDiffs.length > 0 ? (
            <div className="flex-1 overflow-auto">{stackedDiffs}</div>
          ) : (
            <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center">
              <FileCode className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">
                {hasChanges
                  ? "Loading changes..."
                  : "No changes to display"}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// --- Header sub-component ---

interface HeaderProps {
  branch: string;
  ahead: number;
  behind: number;
  onRefresh: () => void;
  refreshing: boolean;
}

function Header({ branch, ahead, behind, onRefresh, refreshing }: HeaderProps) {
  return (
    <div className="flex items-center gap-2 px-3 py-1.5">
      <GitBranch className="text-muted-foreground h-4 w-4 flex-shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          {branch || "Git Status"}
        </p>
        {(ahead > 0 || behind > 0) && (
          <div className="text-muted-foreground flex items-center gap-2 text-xs">
            {ahead > 0 && (
              <span className="flex items-center gap-0.5">
                <ArrowUp className="h-3 w-3" />
                {ahead}
              </span>
            )}
            {behind > 0 && (
              <span className="flex items-center gap-0.5">
                <ArrowDown className="h-3 w-3" />
                {behind}
              </span>
            )}
          </div>
        )}
      </div>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRefresh}
        disabled={refreshing}
        className="h-6 w-6"
      >
        <RefreshCw className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`} />
      </Button>
    </div>
  );
}
