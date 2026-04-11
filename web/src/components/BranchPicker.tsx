import { useState, useEffect, useCallback, useRef } from "react";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { X, Loader2, GitBranch } from "lucide-react";
import { useBranchesQuery } from "@/data/git/queries";
import { useViewport } from "@/hooks/useViewport";

interface BranchPickerProps {
  open: boolean;
  source: string;
  onSelect: (branch: string) => void;
  onClose: () => void;
  placeholder?: string;
  initialQuery?: string;
}

export function BranchPicker({
  open,
  source,
  onSelect,
  onClose,
  placeholder = "Search branches or type a name...",
  initialQuery = "",
}: BranchPickerProps) {
  const [query, setQuery] = useState(initialQuery);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);
  const { isMobile } = useViewport();

  const { data, isLoading } = useBranchesQuery(source, { enabled: open });
  const allBranches = data ?? [];

  const filtered = query.trim()
    ? allBranches.filter((b) =>
        b.toLowerCase().includes(query.trim().toLowerCase()),
      )
    : allBranches;

  useEffect(() => {
    setSelectedIndex(null);
  }, [filtered.length, query]);

  useEffect(() => {
    if (open) {
      setQuery(initialQuery);
      setSelectedIndex(null);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open, initialQuery]);

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
    if (filtered.length === 0) return;
    setSelectedIndex((prev) =>
      prev === null ? 0 : Math.min(prev + 1, filtered.length - 1),
    );
  }, [filtered.length]);

  const handleSelect = useCallback(
    (branch: string) => {
      onSelect(branch);
    },
    [onSelect],
  );

  const handleEnter = useCallback(() => {
    if (selectedIndex !== null && filtered[selectedIndex]) {
      handleSelect(filtered[selectedIndex]);
    } else if (query.trim()) {
      handleSelect(query.trim());
    }
  }, [selectedIndex, filtered, query, handleSelect]);

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
        isMobile && "flex-1 min-h-0",
      )}
    >
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

      <div
        className={cn(
          "overflow-y-auto overscroll-contain py-2",
          isMobile ? "min-h-0 flex-1" : "h-[300px]",
        )}
      >
        {isLoading && filtered.length === 0 ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="text-muted-foreground h-5 w-5 animate-spin" />
          </div>
        ) : filtered.length === 0 && !query.trim() ? (
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            {data ? "No branches found." : "Loading branches..."}
          </div>
        ) : filtered.length === 0 && query.trim() ? (
          <div className="text-muted-foreground px-4 py-8 text-center text-sm">
            No matching branches.
            <br />
            Press <kbd className="bg-muted rounded px-1.5 py-0.5">↵</kbd> to
            use &ldquo;{query.trim()}&rdquo; as a new branch.
          </div>
        ) : (
          filtered.map((branch, index) => (
            <button
              ref={index === selectedIndex ? selectedRef : undefined}
              key={branch}
              onClick={() => handleSelect(branch)}
              className={cn(
                "flex w-full items-center gap-3 px-4 py-1.5 text-left transition-colors",
                index === selectedIndex ? "bg-accent" : "hover:bg-accent/50",
              )}
            >
              <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md bg-green-500/20 text-green-400">
                <GitBranch className="h-3.5 w-3.5" />
              </div>
              <span className="min-w-0 flex-1 truncate font-medium">
                {branch}
              </span>
            </button>
          ))
        )}
      </div>

      {!isMobile && (
        <div className="border-border text-muted-foreground flex items-center gap-4 border-t px-4 py-2 text-xs">
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">↑↓</kbd> navigate
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">↵</kbd> select
          </span>
          <span>
            <kbd className="bg-muted rounded px-1.5 py-0.5">esc</kbd> close
          </span>
        </div>
      )}
    </div>
  );
}
