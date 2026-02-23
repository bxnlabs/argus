import { useState, useCallback, useRef, useEffect, lazy, Suspense } from "react";
import {
  FolderOpen,
  Folder,
  Loader2,
  AlertCircle,
  ArrowLeft,
  Save,
  RefreshCw,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FileTree } from "./FileTree";
import { FileTabs } from "./FileTabs";
const FileEditor = lazy(() =>
  import("./FileEditor").then((mod) => ({ default: mod.FileEditor })),
);
import { useFileEditor } from "./useFileEditor";
import { useViewport } from "@/hooks/useViewport";
import { useFilesQuery } from "@/data/files";

interface FileExplorerProps {
  workingDirectory: string;
}

export function FileExplorer({ workingDirectory }: FileExplorerProps) {
  const { isMobile } = useViewport();
  const {
    openFiles,
    activeFilePath,
    loading: fileLoading,
    saving,
    openFile,
    closeFile,
    setActiveFile,
    updateContent,
    saveFile,
    isDirty,
    getFile,
  } = useFileEditor();

  const {
    data: filesData,
    isPending: loading,
    isError,
    error,
    isRefetching,
    refetch,
  } = useFilesQuery(workingDirectory);

  // Pending close state for unsaved-changes dialog
  const [pendingClose, setPendingClose] = useState<string | null>(null);

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

  const handleCloseFile = useCallback(
    (path: string) => {
      if (isDirty(path)) {
        setPendingClose(path);
      } else {
        closeFile(path);
      }
    },
    [isDirty, closeFile],
  );

  const handleConfirmClose = useCallback(() => {
    if (!pendingClose) return;
    closeFile(pendingClose);
    setPendingClose(null);
  }, [pendingClose, closeFile]);

  const handleSaveAndClose = useCallback(async () => {
    if (!pendingClose) return;
    const saved = await saveFile(pendingClose);
    if (saved) {
      closeFile(pendingClose);
    }
    setPendingClose(null);
  }, [pendingClose, saveFile, closeFile]);

  const handleSave = useCallback(async () => {
    if (activeFilePath) {
      await saveFile(activeFilePath);
    }
  }, [activeFilePath, saveFile]);

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

  const activeFile = activeFilePath ? getFile(activeFilePath) : undefined;
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
    const isCurrentDirty = activeFilePath ? isDirty(activeFilePath) : false;
    const showEditor = !!activeFile;

    return (
      <div className="bg-background flex h-full w-full flex-col">
        {/* Editor view */}
        {showEditor && (
          <div className="flex h-full w-full flex-col">
            <div className="bg-muted/30 flex items-center gap-2 p-2">
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setActiveFile(null)}
              >
                <ArrowLeft className="h-5 w-5" />
              </Button>
              <div className="min-w-0 flex-1">
                <FileTabs
                  files={openFiles}
                  activeFilePath={activeFilePath}
                  onSelect={setActiveFile}
                  onClose={handleCloseFile}
                  isDirty={isDirty}
                />
              </div>
              {isCurrentDirty && (
                <Button
                  variant="default"
                  size="sm"
                  onClick={handleSave}
                  disabled={saving}
                  className="flex-shrink-0"
                >
                  <Save className="mr-1 h-4 w-4" />
                  Save
                </Button>
              )}
            </div>
            <div className="flex-1 overflow-hidden">
              {fileLoading ? (
                <div className="flex h-full items-center justify-center">
                  <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
                </div>
              ) : (
                <Suspense fallback={<EditorSkeleton />}>
                  <FileEditor
                    content={activeFile.content}
                    language={activeFile.language}
                    isBinary={activeFile.isBinary}
                    isLarge={activeFile.isLarge}
                    onChange={(c) => updateContent(activeFile.path, c)}
                    onSave={handleSave}
                  />
                </Suspense>
              )}
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
          {fileLoading && (
            <div className="bg-background/80 fixed inset-0 z-50 flex flex-col items-center justify-center gap-3 backdrop-blur-sm">
              <Loader2 className="text-primary h-8 w-8 animate-spin" />
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setActiveFile(null)}
              >
                Cancel
              </Button>
            </div>
          )}
        </div>

        <UnsavedChangesDialog
          open={!!pendingClose}
          fileName={pendingClose?.split("/").pop() || ""}
          onCancel={() => setPendingClose(null)}
          onDiscard={handleConfirmClose}
          onSave={handleSaveAndClose}
        />
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
          {openFiles.length > 0 && (
            <div className="bg-background/50">
              <FileTabs
                files={openFiles}
                activeFilePath={activeFilePath}
                onSelect={setActiveFile}
                onClose={handleCloseFile}
                isDirty={isDirty}
              />
            </div>
          )}

          <div className="flex-1 overflow-hidden">
            {fileLoading ? (
              <div className="flex h-full items-center justify-center">
                <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
              </div>
            ) : activeFile ? (
              <Suspense fallback={<EditorSkeleton />}>
                <FileEditor
                  content={activeFile.content}
                  language={activeFile.language}
                  isBinary={activeFile.isBinary}
                  isLarge={activeFile.isLarge}
                  onChange={(c) => updateContent(activeFile.path, c)}
                  onSave={handleSave}
                />
              </Suspense>
            ) : (
              <div className="text-muted-foreground flex h-full flex-col items-center justify-center">
                <Folder className="mb-4 h-12 w-12 opacity-50" />
                <p className="text-sm">Select a file to edit</p>
              </div>
            )}
          </div>
        </div>
      </div>

      <UnsavedChangesDialog
        open={!!pendingClose}
        fileName={pendingClose?.split("/").pop() || ""}
        onCancel={() => setPendingClose(null)}
        onDiscard={handleConfirmClose}
        onSave={handleSaveAndClose}
      />
    </div>
  );
}

// --- Editor Skeleton ---

function EditorSkeleton() {
  return (
    <div className="flex h-full flex-col gap-2 p-4 pt-2">
      {[70, 45, 90, 60, 30, 80, 55, 40, 75, 50].map((w, i) => (
        <div key={i} className="flex items-center gap-3">
          <div className="bg-muted h-3 w-5 animate-pulse rounded" />
          <div
            className="bg-muted h-3 animate-pulse rounded"
            style={{ width: `${w}%` }}
          />
        </div>
      ))}
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
      >
        <RefreshCw
          className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
        />
      </Button>
    </div>
  );
}

// --- Unsaved Changes Dialog ---

interface UnsavedChangesDialogProps {
  open: boolean;
  fileName: string;
  onCancel: () => void;
  onDiscard: () => void;
  onSave: () => void;
}

function UnsavedChangesDialog({
  open,
  fileName,
  onCancel,
  onDiscard,
  onSave,
}: UnsavedChangesDialogProps) {
  return (
    <Dialog open={open} onOpenChange={(isOpen: boolean) => !isOpen && onCancel()}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Unsaved changes</DialogTitle>
          <DialogDescription>
            {fileName} has unsaved changes. What would you like to do?
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={onDiscard}>
            Discard
          </Button>
          <Button onClick={onSave}>Save</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
