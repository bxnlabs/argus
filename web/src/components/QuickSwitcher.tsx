import { useState, useEffect, useCallback, useRef } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn, formatRelativeTime, compressPath } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Terminal, Clock, X } from "lucide-react";
import type { Session } from "@/types";
import { useViewport } from "@/hooks/useViewport";

interface QuickSwitcherProps {
  sessions: Session[];
  homeDir: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelectSession: (sessionId: string) => void;
  currentSessionId?: string;
}

/**
 * Quick session switcher with search
 * Triggered by Cmd+K or button tap
 */
export function QuickSwitcher({
  sessions,
  homeDir,
  open,
  onOpenChange,
  onSelectSession,
  currentSessionId,
}: QuickSwitcherProps) {
  const [query, setQuery] = useState("");
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);
  const { isMobile } = useViewport();

  // Filter sessions based on search query
  const matchingSessions = sessions.filter((session) => {
    if (!query) return true;
    const q = query.toLowerCase();
    return (
      session.name?.toLowerCase().includes(q) ||
      session.working_directory?.toLowerCase().includes(q) ||
      session.git_parent_dir?.toLowerCase().includes(q) ||
      session.provider_type?.toLowerCase().includes(q)
    );
  });

  // The session you're on sits at the top, whatever the sidebar's recency order
  // puts there. It's the one row whose position you can predict before the list
  // renders, which is what makes it a landmark — the rest of the list reads as
  // "everything else" relative to it, and switching away and back doesn't move
  // it. Hoisted among the matches rather than pinned outright, so a search it
  // doesn't match still excludes it.
  const currentIndex = currentSessionId
    ? matchingSessions.findIndex((s) => s.id === currentSessionId)
    : -1;
  const filteredSessions =
    currentIndex > 0
      ? [
          matchingSessions[currentIndex],
          ...matchingSessions.slice(0, currentIndex),
          ...matchingSessions.slice(currentIndex + 1),
        ]
      : matchingSessions;

  // Reset state when dialog opens
  useEffect(() => {
    if (open) {
      setQuery("");
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open]);

  // Reset selected index when filtered results change
  useEffect(() => {
    setSelectedIndex(0);
  }, [query]);

  // Auto-scroll selected item into view
  useEffect(() => {
    selectedRef.current?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [selectedIndex]);

  // Navigation handlers
  const navigateUp = useCallback(() => {
    setSelectedIndex((prev) => Math.max(prev - 1, 0));
  }, []);

  const navigateDown = useCallback(() => {
    setSelectedIndex((prev) =>
      Math.min(prev + 1, Math.max(filteredSessions.length - 1, 0))
    );
  }, [filteredSessions.length]);

  const selectCurrent = useCallback(() => {
    if (filteredSessions[selectedIndex]) {
      onSelectSession(filteredSessions[selectedIndex].id);
      onOpenChange(false);
    }
  }, [filteredSessions, selectedIndex, onSelectSession, onOpenChange]);

  const close = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);

  // Handle keyboard navigation
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
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
          selectCurrent();
          break;
        case "Escape":
          e.preventDefault();
          close();
          break;
      }
    },
    [navigateDown, navigateUp, selectCurrent, close]
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          "gap-0 overflow-hidden p-0",
          isMobile
            ? "top-[env(safe-area-inset-top)] left-0 right-0 h-[calc(var(--app-height)_-_env(safe-area-inset-top))] max-w-none translate-x-0 translate-y-0 rounded-none border-0"
            : "top-[50%] left-[50%] translate-x-[-50%] translate-y-[-50%] sm:max-w-md"
        )}
        showCloseButton={false}
      >
        <div className={cn("flex min-h-0 min-w-0 flex-col", isMobile && "h-full")}>
          <DialogHeader className="sr-only">
            <DialogTitle>Switch Session</DialogTitle>
          </DialogHeader>

          {/* Search Input */}
          <div className="border-border flex items-center gap-2 border-b p-3">
            <Input
              ref={inputRef}
              placeholder="Search sessions..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              className="h-10"
            />
            <button
              onClick={() => onOpenChange(false)}
              className="text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          {/* Results */}
          <div className={cn(
            "overflow-y-auto overscroll-contain py-2",
            isMobile ? "min-h-0 flex-1" : "h-[300px]"
          )}>
            {filteredSessions.length === 0 ? (
              <div className="text-muted-foreground px-4 py-8 text-center text-sm">
                No sessions found
              </div>
            ) : (
              filteredSessions.map((session, index) => {
                const isCurrent = session.id === currentSessionId;
                return (
                  <button
                    ref={index === selectedIndex ? selectedRef : undefined}
                    key={session.id}
                    onClick={() => {
                      onSelectSession(session.id);
                      onOpenChange(false);
                    }}
                    // The row wash is the keyboard cursor and nothing else. It
                    // used to double as a current-session marker in
                    // `bg-primary/10`, which cost twice: blue is unread's color
                    // elsewhere in the app, and since `cn` runs tailwind-merge
                    // the later background silently dropped `bg-accent` — so
                    // arrowing onto the current row changed nothing on screen
                    // and the cursor vanished on exactly one row. The "Current"
                    // chip carries that meaning now, where it can't collide.
                    className={cn(
                      "flex w-full items-center gap-3 px-4 py-2.5 text-left transition-colors",
                      index === selectedIndex
                        ? "bg-accent"
                        : "hover:bg-accent/50",
                    )}
                  >
                    {/* Icon */}
                    <div className="bg-emerald-500/20 text-emerald-400 flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md">
                      <Terminal className="h-4 w-4" />
                    </div>

                    {/* Content */}
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="truncate font-medium">
                          {session.name || "Unnamed Session"}
                        </span>
                        {/* White for the same reason the node panel's check is
                            (MobileNodePanel): "you're on this one" reads white
                            app-wide, leaving blue to mean unread. Says it in
                            words rather than a check because this row sits in a
                            list you're picking *from* — a bare tick reads as
                            "chosen" as easily as "current". Outlined chip at
                            text-[10px] is the house style (ProviderBadge). */}
                        {isCurrent && (
                          <Badge
                            variant="outline"
                            className="flex-shrink-0 border-current px-1 py-0 text-[10px] font-medium text-white"
                          >
                            Current
                          </Badge>
                        )}
                      </div>
                      <div className="text-muted-foreground flex items-center gap-2 text-xs">
                        <span className="truncate">
                          {compressPath(
                            session.git_parent_dir ?? session.working_directory ?? "~",
                            homeDir,
                          )}
                        </span>
                        <span>·</span>
                        <span className="capitalize">
                          {session.provider_type}
                        </span>
                      </div>
                    </div>

                    {/* Time */}
                    <div className="text-muted-foreground flex flex-shrink-0 items-center gap-1 text-xs">
                      <Clock className="h-3 w-3" />
                      <span>{formatRelativeTime(session.updated_at)}</span>
                    </div>
                  </button>
                );
              })
            )}
          </div>

          {/* Footer: keyboard hints (desktop only) */}
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
      </DialogContent>
    </Dialog>
  );
}
