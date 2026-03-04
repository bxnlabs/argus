import { useState, useCallback, useRef, useEffect } from "react";
import { toast } from "sonner";
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
import { DiffView } from "@/components/DiffViewer";
import { useViewport } from "@/hooks/useViewport";
import { useGitStatusQuery } from "@/data/git";
import { apiFetch } from "@/api/client";
import type { GitFile } from "@/types";

interface GitPanelProps {
  workingDirectory: string;
}

interface SelectedFile {
  file: GitFile;
  diff: string;
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

  // Selected file for diff view
  const [selectedFile, setSelectedFile] = useState<SelectedFile | null>(null);
  const [loadingDiff, setLoadingDiff] = useState(false);

  // Resizable panel state (desktop)
  const [listWidth, setListWidth] = useState(350);
  const containerRef = useRef<HTMLDivElement>(null);
  const isDragging = useRef(false);

  const handleRefresh = async () => {
    await refetch();
  };

  const handleFileClick = async (file: GitFile) => {
    setLoadingDiff(true);
    try {
      const isUntracked = file.status === "untracked";
      const params = new URLSearchParams({
        path: workingDirectory,
        file: file.path,
        staged: file.staged.toString(),
        ...(isUntracked && { untracked: "true" }),
      });

      const data = await apiFetch<{ diff: string }>(
        `/agent/api/git/diff?${params}`,
      );
      if (data.diff !== undefined) {
        setSelectedFile({ file, diff: data.diff });
      }
    } catch {
      toast.error("Failed to load diff");
    } finally {
      setLoadingDiff(false);
    }
  };

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
        <Header branch="" ahead={0} behind={0} onRefresh={handleRefresh} refreshing={false} />
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className="bg-background flex h-full w-full flex-col">
        <Header branch="" ahead={0} behind={0} onRefresh={handleRefresh} refreshing={isRefetching} />
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
                <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />
              </>
            }
          />
        </div>
      );
    }

    // Diff view when file is selected
    if (selectedFile) {
      return (
        <div className="bg-background flex h-full w-full flex-col">
          <div className="bg-muted/30 flex items-center gap-2 p-2">
            <Button variant="ghost" size="icon-sm" onClick={() => setSelectedFile(null)}>
              <ArrowLeft className="h-5 w-5" />
            </Button>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium">
                {selectedFile.file.path}
              </p>
            </div>
          </div>
          <div className="safe-area-bottom flex-1 overflow-auto p-3">
            {loadingDiff ? (
              <div className="flex h-32 items-center justify-center">
                <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
              </div>
            ) : (
              <DiffView diff={selectedFile.diff} fileName={selectedFile.file.path} />
            )}
          </div>
        </div>
      );
    }

    // File list (Changes tab)
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
                  onFileClick={handleFileClick}
                />
              )}
              {status.unstaged.length > 0 && (
                <FileChanges
                  files={status.unstaged}
                  title="Changes"
                  onFileClick={handleFileClick}
                />
              )}
              {status.untracked.length > 0 && (
                <FileChanges
                  files={status.untracked}
                  title="Untracked Files"
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

  // Changes tab: side-by-side
  return (
    <div ref={containerRef} className="bg-background flex h-full w-full flex-col">
      <div className="flex min-h-0 flex-1">
        {/* Left panel - file list */}
        <div className="flex h-full flex-col" style={{ width: listWidth }}>
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
                    selectedPath={selectedFile?.file.path}
                    onFileClick={handleFileClick}
                  />
                )}
                {status.unstaged.length > 0 && (
                  <FileChanges
                    files={status.unstaged}
                    title="Changes"
                    selectedPath={selectedFile?.file.path}
                    onFileClick={handleFileClick}
                  />
                )}
                {status.untracked.length > 0 && (
                  <FileChanges
                    files={status.untracked}
                    title="Untracked Files"
                    selectedPath={selectedFile?.file.path}
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

        {/* Right panel - diff viewer */}
        <div className="bg-muted/20 flex h-full min-w-0 flex-1 flex-col">
          {loadingDiff ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : selectedFile ? (
            <>
              <div className="bg-background/50 flex items-center gap-2 p-3">
                <FileCode className="text-muted-foreground h-4 w-4" />
                <span className="flex-1 truncate text-sm font-medium">
                  {selectedFile.file.path}
                </span>
              </div>
              <div className="flex-1 overflow-auto p-3">
                <DiffView diff={selectedFile.diff} fileName={selectedFile.file.path} />
              </div>
            </>
          ) : (
            <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center">
              <FileCode className="mb-4 h-12 w-12 opacity-50" />
              <p className="text-sm">Select a file to view diff</p>
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
