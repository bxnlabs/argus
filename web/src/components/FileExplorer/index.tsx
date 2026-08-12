import { useState, useCallback, useRef, useEffect, lazy, Suspense } from "react";
import {
  FolderOpen,
  Folder,
  Loader2,
  AlertCircle,
  ArrowLeft,
  RefreshCw,
} from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { FileTree } from "./FileTree";
import { FileTabs } from "./FileTabs";
const FileEditor = lazy(() =>
  import("./FileEditor").then((mod) => ({ default: mod.FileEditor })),
);
import { useFileEditor } from "./useFileEditor";
import { useViewport } from "@/hooks/useViewport";
import { useFilesQuery } from "@/data/files";
import { apiFetch, apiTextFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { FileMetaResponse } from "@/types";

interface FileExplorerProps {
  workingDirectory: string;
}

// A file once its meta and (if applicable) content have been read. Held here
// rather than in useFileEditor, which only tracks which paths are open.
interface OpenFileContent {
  path: string;
  content: string;
  language: string;
  isBinary: boolean;
  isLarge: boolean;
}

const LARGE_FILE_THRESHOLD = 5 * 1024 * 1024; // 5MB

function getLanguageFromPath(filePath: string): string {
  const ext = filePath.split(".").pop()?.toLowerCase() || "";
  const map: Record<string, string> = {
    ts: "typescript",
    tsx: "typescript",
    js: "javascript",
    jsx: "javascript",
    json: "json",
    md: "markdown",
    html: "html",
    htm: "html",
    css: "css",
    scss: "scss",
    less: "less",
    xml: "xml",
    yaml: "yaml",
    yml: "yaml",
    py: "python",
    go: "go",
    rs: "rust",
    sh: "shell",
    bash: "shell",
    zsh: "shell",
    sql: "sql",
    graphql: "graphql",
    dockerfile: "dockerfile",
    toml: "toml",
    ini: "ini",
    c: "c",
    cpp: "cpp",
    h: "c",
    hpp: "cpp",
    java: "java",
    rb: "ruby",
    php: "php",
    swift: "swift",
    kt: "kotlin",
    lua: "lua",
    r: "r",
  };
  return map[ext] || "plaintext";
}

export function FileExplorer({ workingDirectory }: FileExplorerProps) {
  const { isMobile } = useViewport();
  const { baseUrl } = useActiveNode();
  const { openPaths, activeFilePath, openFile, closeFile, setActiveFile } =
    useFileEditor();

  // Read content per open path, keyed by path. A path leaves this map when
  // its tab closes (see handleCloseFile).
  const [contents, setContents] = useState<Record<string, OpenFileContent>>({});
  // Which paths currently have a fetch in flight — per-path, not a single
  // shared flag, so one file's fetch resolving cannot clear another file's
  // still-loading state out from under it.
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  const contentsRef = useRef(contents);
  contentsRef.current = contents;
  const baseUrlRef = useRef(baseUrl);
  baseUrlRef.current = baseUrl;
  const openingPathsRef = useRef<Set<string>>(new Set());

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

  const loadFileContent = useCallback(async (path: string) => {
    if (contentsRef.current[path] || openingPathsRef.current.has(path)) return;
    openingPathsRef.current.add(path);
    setLoadingPaths((prev) => new Set(prev).add(path));
    try {
      const meta = await apiFetch<FileMetaResponse>(
        baseUrlRef.current,
        `/api/node/files/meta?path=${encodeURIComponent(path)}`,
      );

      const isLarge = meta.size > LARGE_FILE_THRESHOLD;
      const isBinary = meta.isBinary;

      let content = "";
      if (!isBinary && !isLarge) {
        const res = await apiTextFetch(
          baseUrlRef.current,
          `/api/node/files/content?path=${encodeURIComponent(path)}`,
        );
        content = await res.text();
      }

      setContents((prev) => ({
        ...prev,
        [path]: {
          path,
          content,
          language: getLanguageFromPath(path),
          isBinary,
          isLarge,
        },
      }));
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to open file",
      );
    } finally {
      openingPathsRef.current.delete(path);
      setLoadingPaths((prev) => {
        if (!prev.has(path)) return prev;
        const next = new Set(prev);
        next.delete(path);
        return next;
      });
    }
  }, []);

  const handleFileClick = useCallback(
    (path: string) => {
      openFile(path);
      void loadFileContent(path);
    },
    [openFile, loadFileContent],
  );

  const handleCloseFile = useCallback(
    (path: string) => {
      closeFile(path);
      setContents((prev) => {
        if (!(path in prev)) return prev;
        const next = { ...prev };
        delete next[path];
        return next;
      });
    },
    [closeFile],
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

  const activeFile = activeFilePath ? contents[activeFilePath] : undefined;
  const activeFileLoading = activeFilePath ? loadingPaths.has(activeFilePath) : false;
  const anyFileLoading = loadingPaths.size > 0;
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
                onClick={() => setActiveFile(null)}
              >
                <ArrowLeft className="h-5 w-5" />
              </Button>
              <div className="min-w-0 flex-1">
                <FileTabs
                  paths={openPaths}
                  activeFilePath={activeFilePath}
                  onSelect={setActiveFile}
                  onClose={handleCloseFile}
                />
              </div>
            </div>
            <div className="flex-1 overflow-hidden">
              {activeFileLoading && !activeFile ? (
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
                  />
                </Suspense>
              ) : null}
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
          {anyFileLoading && (
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
                onClose={handleCloseFile}
              />
            </div>
          )}

          <div className="flex-1 overflow-hidden">
            {activeFileLoading && !activeFile ? (
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
