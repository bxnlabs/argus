import { useState, useEffect, useCallback, useRef } from "react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { X, Loader2, GitBranch } from "lucide-react";
import { useGitHubReposQuery } from "@/data/github";
import { useViewport } from "@/hooks/useViewport";

interface RepoBrowserProps {
  open: boolean;
  onSelect: (repo: string) => void;
  onClose: () => void;
  placeholder?: string;
  initialQuery?: string;
}

export function RepoBrowser({
  open,
  onSelect,
  onClose,
  placeholder = "Search repos or enter a URL...",
  initialQuery = "",
}: RepoBrowserProps) {
  const [query, setQuery] = useState(initialQuery);
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { isMobile } = useViewport();

  // Debounce query for API call
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (!query.trim()) {
      setDebouncedQuery("");
      return;
    }
    debounceRef.current = setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 300);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [query]);

  const { data, isLoading } = useGitHubReposQuery(debouncedQuery, {
    enabled: open,
  });
  const repos = data?.repos ?? [];

  // Reset selection when results change
  useEffect(() => {
    setSelectedIndex(null);
  }, [repos.length, debouncedQuery]);

  // Focus input and reset state when opened
  useEffect(() => {
    if (open) {
      setQuery(initialQuery);
      setDebouncedQuery("");
      setSelectedIndex(null);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open, initialQuery]);

  // Scroll selected item into view
  useEffect(() => {
    if (selectedIndex !== null) {
      selectedRef.current?.scrollIntoView({ block: "nearest" });
    }
  }, [selectedIndex]);

  const navigateUp = useCallback(() => {
    setSelectedIndex((prev) => {
      if (prev === null) return null;
      return prev === 0 ? null : prev - 1;
    });
  }, []);

  const navigateDown = useCallback(() => {
    if (repos.length === 0) return;
    setSelectedIndex((prev) =>
      prev === null ? 0 : Math.min(prev + 1, repos.length - 1),
    );
  }, [repos.length]);

  const handleSelect = useCallback(
    (repo: string) => {
      onSelect(repo);
    },
    [onSelect],
  );

  const handleEnter = useCallback(() => {
    if (selectedIndex !== null && repos[selectedIndex]) {
      handleSelect(repos[selectedIndex]);
    } else if (query.trim()) {
      // Free-text fallback: submit typed text as custom repo
      handleSelect(query.trim());
    }
  }, [selectedIndex, repos, query, handleSelect]);

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
    [navigateDown, navigateUp, handleEnter, onClose],
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
        isMobile && "h-full",
      )}
    >
      {/* Search Input Row */}
      <div className="border-border flex items-center gap-2 border-b p-3">
        <Input
          ref={inputRef}
          placeholder={placeholder}
          value={query}
          onChange={handleQueryChange}
          onKeyDown={handleKeyDown}
          className="h-10"
        />
        <button
          onClick={onClose}
          className="text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5"
          aria-label="Close"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {/* Results */}
      <div
        className={cn(
          "overflow-y-auto overscroll-contain py-2",
          isMobile ? "min-h-0 flex-1" : "h-[300px]",
        )}
      >
        {isLoading && repos.length === 0 ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
          </div>
        ) : repos.length === 0 && !query.trim() ? (
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            {data
              ? "No repositories found. Type to search or enter a repo URL."
              : "Type to search or enter a repo URL"}
          </div>
        ) : repos.length === 0 && query.trim() ? (
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            No matching repos. Press <kbd className="bg-muted rounded px-1.5 py-0.5">↵</kbd> to use &ldquo;{query.trim()}&rdquo; as-is.
          </div>
        ) : (
          repos.map((repo, index) => (
            <button
              ref={index === selectedIndex ? selectedRef : undefined}
              key={repo}
              onClick={() => handleSelect(repo)}
              className={cn(
                "flex w-full items-center gap-3 px-4 py-1.5 text-left transition-colors",
                index === selectedIndex
                  ? "bg-accent"
                  : "hover:bg-accent/50",
              )}
            >
              <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md bg-purple-500/20 text-purple-400">
                <GitBranch className="h-3.5 w-3.5" />
              </div>
              <span className="min-w-0 flex-1 truncate font-medium">
                {repo}
              </span>
            </button>
          ))
        )}
      </div>

      {/* Keyboard hints footer (desktop only) */}
      {!isMobile && (
        <div className="border-border text-muted-foreground flex items-center gap-4 border-t px-4 py-2 text-xs">
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">↑↓</kbd>{" "}
            navigate
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">↵</kbd>{" "}
            select
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">esc</kbd>{" "}
            close
          </span>
        </div>
      )}
    </div>
  );
}
