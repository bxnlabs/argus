import { useState, useCallback } from "react";
import {
  ChevronRight,
  ChevronDown,
  File,
  Folder,
  FolderOpen,
  Loader2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { apiFetch } from "@/api/client";
import type { FileNode, FilesResponse } from "@/types";

interface FileTreeProps {
  nodes: FileNode[];
  onFileClick: (path: string) => void;
  selectedPath?: string | null;
  depth?: number;
}

export function FileTree({
  nodes,
  onFileClick,
  selectedPath,
  depth = 0,
}: FileTreeProps) {
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [loadedChildren, setLoadedChildren] = useState<
    Map<string, FileNode[]>
  >(new Map());
  const [loadingDirs, setLoadingDirs] = useState<Set<string>>(new Set());

  const fetchChildren = useCallback(
    async (dirPath: string) => {
      if (loadedChildren.has(dirPath)) return;

      setLoadingDirs((prev) => new Set(prev).add(dirPath));
      try {
        const data = await apiFetch<FilesResponse>(
          `/api/node/files?path=${encodeURIComponent(dirPath)}`,
        );
        if (data.files) {
          setLoadedChildren((prev) => new Map(prev).set(dirPath, data.files));
        }
      } catch (err) {
        console.error("Failed to load directory:", err);
      } finally {
        setLoadingDirs((prev) => {
          const next = new Set(prev);
          next.delete(dirPath);
          return next;
        });
      }
    },
    [loadedChildren],
  );

  const toggleExpand = useCallback(
    async (path: string) => {
      const isCurrentlyExpanded = expanded.has(path);

      setExpanded((prev) => {
        const next = new Set(prev);
        if (next.has(path)) {
          next.delete(path);
        } else {
          next.add(path);
        }
        return next;
      });

      if (!isCurrentlyExpanded && !loadedChildren.has(path)) {
        await fetchChildren(path);
      }
    },
    [expanded, loadedChildren, fetchChildren],
  );

  return (
    <div className="w-full">
      {nodes.map((node) => {
        const isExpanded = expanded.has(node.path);
        const isDirectory = node.type === "directory";
        const isLoading = loadingDirs.has(node.path);
        const children = loadedChildren.get(node.path) || node.children;
        const isSelected = node.path === selectedPath;

        return (
          <div key={node.path}>
            <button
              onClick={() => {
                if (isDirectory) {
                  toggleExpand(node.path);
                } else {
                  onFileClick(node.path);
                }
              }}
              className={cn(
                "flex w-full items-center gap-1.5 px-2 py-1 text-left text-sm transition-colors",
                "min-h-[32px] md:min-h-[24px]",
                isSelected
                  ? "bg-accent text-accent-foreground"
                  : "hover:bg-accent/50",
              )}
              style={{ paddingLeft: `${depth * 12 + 8}px` }}
            >
              {/* Expand/collapse icon for directories */}
              {isDirectory && (
                <span className="flex h-4 w-4 flex-shrink-0 items-center justify-center">
                  {isLoading ? (
                    <Loader2 className="text-muted-foreground h-3 w-3 animate-spin" />
                  ) : isExpanded ? (
                    <ChevronDown className="text-muted-foreground h-4 w-4" />
                  ) : (
                    <ChevronRight className="text-muted-foreground h-4 w-4" />
                  )}
                </span>
              )}

              {/* Spacer for files to align with directory names */}
              {!isDirectory && <span className="w-4 flex-shrink-0" />}

              {/* Icon */}
              <span className="flex-shrink-0">
                {isDirectory ? (
                  isExpanded ? (
                    <FolderOpen className="text-muted-foreground h-4 w-4" />
                  ) : (
                    <Folder className="text-muted-foreground h-4 w-4" />
                  )
                ) : (
                  <File className="text-muted-foreground h-4 w-4" />
                )}
              </span>

              {/* Name */}
              <span
                className={cn(
                  "flex-1 truncate",
                  isDirectory ? "font-medium" : "",
                )}
              >
                {node.name}
              </span>

            </button>

            {/* Children */}
            {isDirectory && isExpanded && children && children.length > 0 && (
              <FileTree
                nodes={children}
                onFileClick={onFileClick}
                selectedPath={selectedPath}
                depth={depth + 1}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}
