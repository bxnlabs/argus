import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { Input } from "@/components/ui/input";
import { cn, contractTilde } from "@/lib/utils";
import { Folder, File, CornerLeftUp, Home, X, Loader2, Check, ChevronRight } from "lucide-react";
import { useFilesQuery, useFileSearchQuery } from "@/data/files";
import { useViewport } from "@/hooks/useViewport";
import type { FileNode } from "@/types";
import { ApiError } from "@/api/client";

interface FileBrowserProps {
  open: boolean;
  onSelect: (absolutePath: string) => void;
  onClose: () => void;
  mode: "directory" | "all";
  placeholder?: string;
  initialQuery?: string;
  headerExtra?: React.ReactNode;
  searchPath?: string;
}

function formatSize(bytes?: number) {
  if (bytes == null) return "";
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

export function FileBrowser({
  open,
  onSelect,
  onClose,
  mode,
  placeholder,
  initialQuery,
  headerExtra,
  searchPath,
}: FileBrowserProps) {
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const [homePath, setHomePath] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { isMobile } = useViewport();

  // --- Mode detection ---
  const isPathMode = query.startsWith("~") || query.startsWith("/");

  // --- Path parsing (for path traversal mode) ---
  const { directoryToList, filterSegment } = useMemo(() => {
    if (!isPathMode) return { directoryToList: "", filterSegment: "" };

    const lastSlash = query.lastIndexOf("/");
    if (lastSlash === -1) {
      // Just "~" -> list home
      return { directoryToList: query, filterSegment: "" };
    }

    const dir = query.slice(0, lastSlash) || "/";
    const filter = query.slice(lastSlash + 1);
    return { directoryToList: dir, filterSegment: filter };
  }, [query, isPathMode]);

  // --- Browse root ---
  // searchPath (the session working directory) is both the search root AND the
  // directory listed when the input is empty ("base listing"). Browsing is
  // active in explicit path mode OR in the base listing.
  const baseDir = searchPath ? searchPath.replace(/\/+$/, "") || "/" : "";
  const isBaseListing = !isPathMode && !query.trim() && !!baseDir;
  const isBrowsing = isPathMode || isBaseListing;
  const browseDir = isPathMode ? directoryToList : baseDir;

  // --- Debounce for search mode ---
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (!query.trim() || isPathMode) {
      setDebouncedQuery("");
      return;
    }
    debounceRef.current = setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 300);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [query, isPathMode]);

  // --- API queries ---
  const filesQuery = useFilesQuery(browseDir, {
    enabled: open && isBrowsing && !!browseDir,
  });

  const searchQuery = useFileSearchQuery(debouncedQuery, {
    enabled: open && !isPathMode && !!debouncedQuery,
    type: mode === "directory" ? "directory" : "",
    searchPath,
  });

  // Learn homePath once for tilde contraction in both modes
  const homeQuery = useFilesQuery("~", { enabled: !homePath });

  useEffect(() => {
    if (homeQuery.data?.path && !homePath) {
      setHomePath(homeQuery.data.path);
    }
  }, [homeQuery.data, homePath]);

  // --- Compute results ---
  const { results, isLoading } = useMemo(() => {
    if (isBrowsing) {
      const allFiles = filesQuery.data?.files ?? [];
      const filtered =
        mode === "directory"
          ? allFiles.filter((f) => f.type === "directory")
          : allFiles;
      const matched = filterSegment
        ? filtered.filter((f) =>
            f.name.toLowerCase().startsWith(filterSegment.toLowerCase()),
          )
        : filtered;
      return { results: matched, isLoading: filesQuery.isLoading };
    }

    if (debouncedQuery) {
      return {
        results: searchQuery.data?.results ?? [],
        isLoading: searchQuery.isLoading,
      };
    }

    return { results: [], isLoading: false };
  }, [
    isBrowsing,
    filesQuery.data,
    filesQuery.isLoading,
    filterSegment,
    mode,
    debouncedQuery,
    searchQuery.data,
    searchQuery.isLoading,
  ]);

  // --- Parent entry ---
  const parentEntry = useMemo(() => {
    if (!isBrowsing) return null;
    // Don't show at root
    const dir = browseDir;
    if (dir === "/" || dir === "~") return null;

    const normalized = dir.endsWith("/") ? dir.slice(0, -1) : dir;
    const lastSlash = normalized.lastIndexOf("/");
    if (lastSlash < 0) return null; // no slash -> not a navigable path
    const parentPath = lastSlash === 0 ? "/" : normalized.slice(0, lastSlash);

    return {
      name: "..",
      path: parentPath,
      type: "directory" as const,
      isParentEntry: true,
    };
  }, [isBrowsing, browseDir]);

  const displayItems = useMemo(() => {
    if (parentEntry) return [parentEntry, ...results];
    return results;
  }, [parentEntry, results]);

  // --- Breadcrumbs ---
  const breadcrumbs = useMemo(() => {
    if (!isBrowsing || !browseDir) return [];

    const display = homePath
      ? contractTilde(browseDir, homePath)
      : browseDir;

    if (display === "/" || display === "~") {
      return [{ label: display, path: display }];
    }

    const isTilde = display.startsWith("~");
    const parts = display.replace(/^~\/?/, "").replace(/^\//, "").split("/").filter(Boolean);
    const crumbs: { label: string; path: string }[] = [];

    if (isTilde) {
      crumbs.push({ label: "~", path: "~" });
    } else {
      crumbs.push({ label: "/", path: "/" });
    }

    let accumulated = isTilde ? "~" : "";
    for (const part of parts) {
      accumulated += "/" + part;
      crumbs.push({ label: part, path: accumulated });
    }

    // Collapse middle when > 4 segments
    if (crumbs.length > 4) {
      return [crumbs[0], { label: "\u2026", path: "" }, ...crumbs.slice(-2)];
    }

    return crumbs;
  }, [isBrowsing, browseDir, homePath]);

  // --- Reset state on open ---
  useEffect(() => {
    if (open) {
      setQuery(initialQuery ?? "");
      setDebouncedQuery("");
      setSelectedIndex(null);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open, initialQuery]);

  // Reset selected index on results change or query error
  useEffect(() => {
    setSelectedIndex(null);
  }, [results, filesQuery.error]);

  // Auto-scroll selected item into view on keyboard navigation
  useEffect(() => {
    if (selectedIndex !== null) {
      selectedRef.current?.scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex]);

  // Move cursor to end after programmatic query changes
  const moveCursorToEnd = useCallback(() => {
    requestAnimationFrame(() => {
      if (inputRef.current) {
        const len = inputRef.current.value.length;
        inputRef.current.setSelectionRange(len, len);
      }
    });
  }, []);

  const goHome = useCallback(() => {
    setQuery("~/");
    setSelectedIndex(null);
    moveCursorToEnd();
    setTimeout(() => inputRef.current?.focus(), 0);
  }, [moveCursorToEnd]);

  const handleBreadcrumbClick = useCallback(
    (path: string) => {
      if (!path) return;
      setQuery(path + (path === "/" ? "" : "/"));
      setSelectedIndex(null);
      moveCursorToEnd();
      setTimeout(() => inputRef.current?.focus(), 0);
    },
    [moveCursorToEnd],
  );

  // --- Navigation handlers ---
  const navigateUp = useCallback(() => {
    setSelectedIndex((prev) => {
      if (prev === null) return null;
      return prev === 0 ? null : prev - 1;
    });
  }, []);

  const navigateDown = useCallback(() => {
    if (displayItems.length === 0) return;
    setSelectedIndex((prev) =>
      prev === null ? 0 : Math.min(prev + 1, displayItems.length - 1),
    );
  }, [displayItems.length]);

  const drillIntoItem = useCallback(
    (item: { name: string; path: string }) => {
      if (isPathMode) {
        const base = directoryToList === "/" ? "/" : directoryToList + "/";
        setQuery(base + item.name + "/");
      } else {
        const base = homePath
          ? contractTilde(item.path, homePath)
          : item.path;
        setQuery(base.replace(/\/+$/, "") + "/");
      }
      setSelectedIndex(null);
      moveCursorToEnd();
    },
    [isPathMode, directoryToList, homePath, moveCursorToEnd],
  );

  const navigateToParent = useCallback(
    (parentPath: string) => {
      setQuery(parentPath + (parentPath === "/" ? "" : "/"));
      setSelectedIndex(null);
      moveCursorToEnd();
    },
    [moveCursorToEnd],
  );

  const tabComplete = useCallback(() => {
    const dirs = results.filter((r) => r.type === "directory");
    if (dirs.length === 1) {
      drillIntoItem(dirs[0]);
    }
  }, [results, drillIntoItem]);

  const handleItemClick = useCallback(
    (item: {
      name: string;
      path: string;
      type: string;
      isParentEntry?: boolean;
    }) => {
      if ("isParentEntry" in item && item.isParentEntry) {
        navigateToParent(item.path);
      } else if (item.type === "directory") {
        drillIntoItem(item);
      } else if (mode === "all") {
        onSelect(item.path);
      }
    },
    [navigateToParent, drillIntoItem, mode, onSelect],
  );

  const handleEnter = useCallback(() => {
    if (selectedIndex !== null) {
      const item = displayItems[selectedIndex];
      if (!item) return;
      handleItemClick(item as Parameters<typeof handleItemClick>[0]);
    } else if (isPathMode) {
      if (mode === "all" && filterSegment) {
        // Prefer the first matching file from results; fall back to the typed path
        const match = results.find(
          (r) => r.type !== "directory" && r.name.toLowerCase() === filterSegment.toLowerCase(),
        );
        onSelect(match ? match.path : query);
      } else if (filesQuery.data?.path) {
        onSelect(filesQuery.data.path);
      }
    }
  }, [
    selectedIndex,
    displayItems,
    handleItemClick,
    mode,
    isPathMode,
    filterSegment,
    results,
    query,
    filesQuery.data,
    onSelect,
  ]);

  // --- Keyboard navigation ---
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      switch (e.key) {
        case "ArrowDown":
          e.preventDefault();
          navigateDown();
          break;
        case "ArrowUp":
          e.preventDefault();
          navigateUp();
          break;
        case "Tab":
          e.preventDefault();
          tabComplete();
          break;
        case "Enter":
          e.preventDefault();
          handleEnter();
          break;
        case "Escape":
          e.preventDefault();
          onClose();
          break;
      }
    },
    [navigateDown, navigateUp, tabComplete, handleEnter, onClose],
  );

  const handleQueryChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      setQuery(e.target.value);
      setSelectedIndex(null);
    },
    [],
  );

  return (
    <div
      className={cn(
        "flex min-h-0 min-w-0 flex-col",
        isMobile && "flex-1 min-h-0",
      )}
    >
      {/* Search Input Row */}
      <div className="border-border flex items-center gap-2 border-b p-3">
        <button
          onClick={goHome}
          className="text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5"
          aria-label="Home directory"
        >
          <Home className="h-4 w-4" />
        </button>
        <Input
          ref={inputRef}
          placeholder={placeholder ?? "Search..."}
          value={query}
          onChange={handleQueryChange}
          onKeyDown={handleKeyDown}
          className="h-10"
        />
        {headerExtra}
        {isMobile && (
          <button
            onClick={handleEnter}
            className="text-primary hover:text-primary/80 flex-shrink-0 rounded-md p-1.5"
            aria-label="Select"
          >
            <Check className="h-4 w-4" />
          </button>
        )}
        <button
          onClick={onClose}
          className="text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5"
          aria-label="Close"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Breadcrumbs */}
      {isBrowsing && breadcrumbs.length > 0 && (
        <div className="border-border flex items-center gap-0.5 overflow-hidden border-b px-3 py-1.5">
          {breadcrumbs.map((crumb, i) => (
            <span key={crumb.path || `ellipsis-${i}`} className="flex items-center gap-0.5">
              {i > 0 && (
                <ChevronRight className="text-muted-foreground h-3 w-3 flex-shrink-0" />
              )}
              {crumb.path ? (
                <button
                  onClick={() => handleBreadcrumbClick(crumb.path)}
                  className={cn(
                    "text-muted-foreground truncate rounded px-1 py-0.5 text-xs transition-colors hover:bg-accent hover:text-foreground",
                    i === breadcrumbs.length - 1 && "text-foreground font-medium",
                  )}
                >
                  {crumb.label === "~" ? (
                    <Home className="h-3 w-3" />
                  ) : (
                    crumb.label
                  )}
                </button>
              ) : (
                <span className="text-muted-foreground px-1 text-xs">{crumb.label}</span>
              )}
            </span>
          ))}
        </div>
      )}

      {/* Results List */}
      <div
        className={cn(
          "overflow-y-auto overscroll-contain py-2",
          isMobile ? "min-h-0 flex-1" : "h-[300px]",
        )}
      >
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
          </div>
        ) : filesQuery.error && isBrowsing ? (
          <>
            {displayItems.map((item, index) => {
              const isParent =
                "isParentEntry" in item && item.isParentEntry;
              if (!isParent) return null;
              return (
                <button
                  ref={index === selectedIndex ? selectedRef : undefined}
                  key={item.path + "-parent"}
                  aria-label="Go to parent directory"
                  onClick={() => navigateToParent(item.path)}
                  className={cn(
                    "flex w-full items-center gap-3 px-4 py-1.5 text-left transition-colors",
                    index === selectedIndex
                      ? "bg-accent"
                      : "hover:bg-accent/50",
                  )}
                >
                  <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md bg-blue-500/20 text-blue-400">
                    <CornerLeftUp className="h-3.5 w-3.5" />
                  </div>
                  <span className="min-w-0 flex-1 truncate font-medium">..</span>
                </button>
              );
            })}
            <div className="text-muted-foreground px-4 py-8 text-center text-sm">
              {filesQuery.error instanceof ApiError && filesQuery.error.status === 403
                ? "Permission denied"
                : "Could not load directory"}
            </div>
          </>
        ) : displayItems.length === 0 ? (
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            {!query.trim() && !isBaseListing
              ? "Looking for something?"
              : isBrowsing && !filterSegment
                ? mode === "directory"
                  ? "No directories found"
                  : "Empty directory"
                : `No matches for \u201c${isPathMode ? filterSegment : query}\u201d`}
          </div>
        ) : (
          displayItems.map((item, index) => {
            const isParent =
              "isParentEntry" in item && item.isParentEntry;
            const isDir = item.type === "directory";

            return (
              <button
                ref={
                  index === selectedIndex ? selectedRef : undefined
                }
                key={item.path + (isParent ? "-parent" : "")}
                onClick={() => handleItemClick(item)}
                className={cn(
                  "flex w-full items-center gap-3 px-4 py-1.5 text-left transition-colors",
                  index === selectedIndex
                    ? "bg-accent"
                    : "hover:bg-accent/50",
                )}
              >
                <div
                  className={cn(
                    "flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md",
                    isDir || isParent
                      ? "bg-blue-500/20 text-blue-400"
                      : "bg-muted text-muted-foreground",
                  )}
                >
                  {isParent ? (
                    <CornerLeftUp className="h-3.5 w-3.5" />
                  ) : isDir ? (
                    <Folder className="h-3.5 w-3.5" />
                  ) : (
                    <File className="h-3.5 w-3.5" />
                  )}
                </div>
                {!isBrowsing && !isParent ? (
                  <div className="min-w-0 flex-1">
                    <div className="truncate font-medium">
                      {item.name}
                    </div>
                    <div className="text-muted-foreground truncate text-xs">
                      {(homePath
                        ? contractTilde(item.path, homePath)
                        : item.path
                      ).replace(/\/+$/, "") +
                        (isDir ? "/" : "")}
                    </div>
                  </div>
                ) : (
                  <span className="min-w-0 flex-1 truncate font-medium">
                    {item.name}
                  </span>
                )}
                {mode === "all" &&
                  !isParent &&
                  !isDir &&
                  "size" in item && (
                    <span className="text-muted-foreground shrink-0 text-xs">
                      {formatSize((item as FileNode).size)}
                    </span>
                  )}
              </button>
            );
          })
        )}
      </div>

      {/* Keyboard hints footer (desktop only) */}
      {!isMobile && (
        <div className="border-border text-muted-foreground flex items-center gap-4 border-t px-4 py-2 text-xs">
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">
              ↑↓
            </kbd>{" "}
            navigate
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">
              ⇥
            </kbd>{" "}
            complete
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">
              ↵
            </kbd>{" "}
            open / select
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">
              esc
            </kbd>{" "}
            close
          </span>
        </div>
      )}
    </div>
  );
}
