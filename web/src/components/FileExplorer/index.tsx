import { useState, useCallback, useRef, useEffect } from "react";
import {
  FolderOpen,
  Folder,
  Loader2,
  AlertCircle,
  ArrowLeft,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { FileTree } from "./FileTree";
import { FileTabs } from "./FileTabs";
import { FileContentView } from "./FileContentView";
import { useFileEditor } from "./useFileEditor";
import { useViewport } from "@/hooks/useViewport";
import { useFilesQuery, useFileContentQuery } from "@/data/files";

interface FileExplorerProps {
  workingDirectory: string;
}

export function FileExplorer({ workingDirectory }: FileExplorerProps) {
  const { isMobile } = useViewport();
  const { openPaths, activeFilePath, openFile, closeFile, setActiveFile } =
    useFileEditor();

  const {
    data: activeFile,
    isPending: fileLoading,
    isError: fileError,
    error: fileErrorObj,
    reload: reloadFile,
    isRefetching: fileRefetching,
  } = useFileContentQuery(activeFilePath ?? "");

  const {
    data: filesData,
    isPending: loading,
    isError,
    error,
    isRefetching,
    refetch,
  } = useFilesQuery(workingDirectory);

  // Resizable panel state (desktop)
  const [treeWidth, setTreeWidth] = useState(280);
  const containerRef = useRef<HTMLDivElement>(null);
  const isDragging = useRef(false);
  const handleMouseMoveRef = useRef<((e: MouseEvent) => void) | null>(null);
  const handleMouseUpRef = useRef<(() => void) | null>(null);

  const handleFileClick = useCallback(
    (path: string) => {
      openFile(path);
    },
    [openFile],
  );

  // Resize handle for desktop
  const handleMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    isDragging.current = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    const handleMouseMove = (e: MouseEvent) => {
      if (!isDragging.current || !containerRef.current) return;
      const containerRect = containerRef.current.getBoundingClientRect();
      const newWidth = e.clientX - containerRect.left;
      setTreeWidth(Math.max(200, Math.min(400, newWidth)));
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

  // Cleanup resize listeners on unmount
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

  const files = filesData?.files || [];

  // --- Loading state ---
  if (loading) {
    return (
      <div className="bg-background flex h-full w-full flex-col">
        <Header onRefresh={() => refetch()} refreshing={false} />
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      </div>
    );
  }

  // --- Error state ---
  if (isError) {
    return (
      <div className="bg-background flex h-full w-full flex-col">
        <Header onRefresh={() => refetch()} refreshing={isRefetching} />
        <div className="flex flex-1 flex-col items-center justify-center p-4">
          <AlertCircle className="text-muted-foreground mb-2 h-8 w-8" />
          <p className="text-muted-foreground text-center text-sm">
            {error?.message ?? "Failed to load directory"}
          </p>
        </div>
      </div>
    );
  }

  // --- Mobile layout ---
  if (isMobile) {
    const showEditor = !!activeFilePath;

    return (
      <div className="bg-background flex h-full w-full flex-col">
        {/* Editor view */}
        {showEditor && (
          <div className="flex h-full w-full flex-col">
            <div className="bg-muted/30 flex items-center gap-2 p-2">
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Back to files"
                onClick={() => setActiveFile(null)}
              >
                <ArrowLeft className="h-5 w-5" />
              </Button>
              <div className="min-w-0 flex-1">
                <FileTabs
                  paths={openPaths}
                  activeFilePath={activeFilePath}
                  onSelect={setActiveFile}
                  onClose={closeFile}
                />
              </div>
              {/* The tree's refresh button is on the screen behind this one, and
                  it reloads the listing, not the open file. */}
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Reload file"
                onClick={() => reloadFile()}
                disabled={fileRefetching}
              >
                <RefreshCw className={`h-4 w-4 ${fileRefetching ? "animate-spin" : ""}`} />
              </Button>
            </div>
            <div className="flex-1 overflow-hidden">
              <FileContentView
                data={activeFile}
                isPending={fileLoading}
                isError={fileError}
                error={fileErrorObj}
              />
            </div>
          </div>
        )}

        {/* Tree view — always mounted, hidden when editor is showing */}
        <div className={showEditor ? "hidden" : "flex h-full w-full flex-col"}>
          <Header onRefresh={() => refetch()} refreshing={isRefetching} />
          <p className="text-muted-foreground truncate px-3 pb-1 text-xs">
            {workingDirectory}
          </p>
          <div className="flex-1 overflow-y-auto">
            {files.length === 0 ? (
              <div className="text-muted-foreground flex h-32 items-center justify-center">
                <p className="text-sm">Empty directory</p>
              </div>
            ) : (
              <FileTree
                nodes={files}
                onFileClick={handleFileClick}
                selectedPath={activeFilePath}
              />
            )}
          </div>
        </div>
      </div>
    );
  }

  // --- Desktop layout ---
  return (
    <div
      ref={containerRef}
      className="bg-background flex h-full w-full flex-col"
    >
      <div className="flex min-h-0 flex-1">
        {/* Left panel - file tree */}
        <div className="flex h-full flex-col" style={{ width: treeWidth }}>
          <Header onRefresh={() => refetch()} refreshing={isRefetching} />
          <p className="text-muted-foreground truncate px-3 pb-1 text-xs">
            {workingDirectory}
          </p>
          <div className="flex-1 overflow-y-auto">
            {files.length === 0 ? (
              <div className="text-muted-foreground flex h-32 items-center justify-center">
                <p className="text-sm">Empty directory</p>
              </div>
            ) : (
              <FileTree
                nodes={files}
                onFileClick={handleFileClick}
                selectedPath={activeFilePath}
              />
            )}
          </div>
        </div>

        {/* Resize handle */}
        <div
          className="bg-muted/50 hover:bg-primary/50 active:bg-primary w-1 flex-shrink-0 cursor-col-resize transition-colors"
          onMouseDown={handleMouseDown}
        />

        {/* Right panel - tabs + editor */}
        <div className="bg-muted/20 flex h-full min-w-0 flex-1 flex-col">
          {openPaths.length > 0 && (
            <div className="bg-background/50">
              <FileTabs
                paths={openPaths}
                activeFilePath={activeFilePath}
                onSelect={setActiveFile}
                onClose={closeFile}
              />
            </div>
          )}

          <div className="flex-1 overflow-hidden">
            {activeFilePath ? (
              <FileContentView
                data={activeFile}
                isPending={fileLoading}
                isError={fileError}
                error={fileErrorObj}
              />
            ) : (
              <div className="text-muted-foreground flex h-full flex-col items-center justify-center">
                <Folder className="mb-4 h-12 w-12 opacity-50" />
                <p className="text-sm">Select a file to edit</p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

// --- Header ---

interface HeaderProps {
  onRefresh: () => void;
  refreshing: boolean;
}

function Header({ onRefresh, refreshing }: HeaderProps) {
  return (
    <div className="flex items-center gap-2 px-3 py-1.5">
      <FolderOpen className="text-muted-foreground h-4 w-4 flex-shrink-0" />
      <p className="flex-1 truncate text-sm font-medium">Files</p>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRefresh}
        disabled={refreshing}
        className="h-6 w-6"
        aria-label="Refresh files"
      >
        <RefreshCw
          className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
        />
      </Button>
    </div>
  );
}
