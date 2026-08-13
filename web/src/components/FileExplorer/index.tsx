import { useCallback } from "react";
import { AlertCircle, ArrowLeft, FolderOpen, Loader2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { FileTree } from "./FileTree";
import { FileContentView } from "./FileContentView";
import { FileTabs } from "./FileTabs";
import { useFileEditor } from "./useFileEditor";
import { useFilesQuery, useFileContentQuery } from "@/data/files";

interface FileExplorerProps {
  workingDirectory: string;
}

/**
 * Mobile file explorer. Desktop renders FileTreeSidebar + EditorCenter in the
 * dock instead, so this is a single drill-in column: tree, then editor.
 */
export function FileExplorer({ workingDirectory }: FileExplorerProps) {
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

  const handleFileClick = useCallback(
    (path: string) => {
      openFile(path);
    },
    [openFile],
  );

  // Whether the user has drilled into a file, independent of whether it has
  // loaded yet — FileContentView owns the pending/error/content branching once
  // the view is shown.
  const showEditor = !!activeFilePath;

  const files = filesData?.files || [];

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

      {/* Tree view — always mounted, hidden (not unmounted) when the editor is
          showing. FileTree keeps its expand/collapse state and fetched
          subdirectories in local useState, so unmounting it on drill-in would
          collapse the tree and refire a fetch per folder every time the user
          backs out of a file.

          Hiding the panel hides its loading and error states too, which is
          deliberate: a listing that fails while a file is open is an error
          about a different resource, and this used to replace the file the
          user was reading with "Failed to load directory". Same rule as
          FileContentView — a blip somewhere else doesn't get to take the
          content off screen. If the node is genuinely gone the content query
          fails as well, and FileContentView reports that; the listing error
          surfaces on the way back, next to the refresh that fixes it. */}
      <div className={showEditor ? "hidden" : "flex h-full w-full flex-col"}>
        <div className="flex items-center gap-2 px-3 py-1.5">
          <FolderOpen className="text-muted-foreground h-4 w-4 flex-shrink-0" />
          <p className="flex-1 truncate text-sm font-medium">Files</p>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => refetch()}
            disabled={isRefetching}
            className="h-6 w-6"
            aria-label="Refresh files"
          >
            <RefreshCw className={`h-4 w-4 ${isRefetching ? "animate-spin" : ""}`} />
          </Button>
        </div>

        {loading ? (
          <div className="flex flex-1 items-center justify-center">
            <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
          </div>
        ) : isError ? (
          <div className="flex flex-1 flex-col items-center justify-center p-4">
            <AlertCircle className="text-muted-foreground mb-2 h-8 w-8" />
            <p className="text-muted-foreground text-center text-sm">
              {error?.message ?? "Failed to load directory"}
            </p>
          </div>
        ) : (
          <>
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
          </>
        )}
      </div>
    </div>
  );
}
