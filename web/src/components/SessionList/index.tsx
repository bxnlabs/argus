import { useState, useMemo, useRef, useCallback, useEffect, memo } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Plus, AlertCircle, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch, BrushCleaning, Star, Flag, MailOpen, Mail } from "lucide-react";
import { cn, formatRelativeTime, compressPath, parseRepoFromRemoteURL } from "@/lib/utils";
import type { Session, SessionStatusInfo } from "@/types";

function getStatusColor(status?: string) {
  switch (status) {
    case "active":
      return "bg-green-500";
    case "idle":
      return "bg-muted-foreground";
    case "dead":
      return "bg-red-500/50";
    default:
      return "bg-muted-foreground/40";
  }
}

function getStatusAnimation(status?: string) {
  switch (status) {
    case "active":
      return "animate-pulse-green";
    default:
      return "";
  }
}

function getStatusLabel(status?: string) {
  switch (status) {
    case "active":
      return "Active";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    default:
      return "";
  }
}

// Sort starred sessions to the top, then by updated_at descending within each
// group. Returns a new array (does not mutate the input).
export function sortSessions(sessions: Session[]): Session[] {
  return [...sessions].sort((a, b) => {
    if (a.starred !== b.starred) return a.starred ? -1 : 1;
    return new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
  });
}

// Decide which read/unread menu items to show. "Mark as read" appears only
// when the session is unread. "Mark as unread" appears only when the session
// is read AND is not the active session (the active session auto-acknowledges,
// so a manual unread there would immediately revert).
export function readMenuState(
  unreadSince: string | null | undefined,
  isActive: boolean,
): { showMarkRead: boolean; showMarkUnread: boolean } {
  const isUnread = !!unreadSince;
  return {
    showMarkRead: isUnread,
    showMarkUnread: !isUnread && !isActive,
  };
}

// ---------------------------------------------------------------------------
// SessionItem — memoized so it only re-renders when its own data changes.
// Receives `statusValue` (the enum string) instead of the full
// SessionStatusInfo object so that changes to `lastLine` (which updates on
// every status poll) don't force a re-render.
// ---------------------------------------------------------------------------

interface SessionItemProps {
  session: Session;
  homeDir: string;
  isActive: boolean;
  statusValue?: SessionStatusInfo["status"];
  unreadSince?: string | null;
  minuteTick: number;
  isRenaming: boolean;
  renameValue: string;
  renameInputRef: (el: HTMLInputElement | null) => void;
  onRenameValueChange: (value: string) => void;
  onConfirmRename: () => void;
  onCancelRename: () => void;
  onStartRename: (session: Session) => void;
  onAttachSession: (sessionId: string) => void;
  onDeleteSession: (sessionId: string, deleteBranch?: boolean) => void;
  onToggleStar: (sessionId: string, starred: boolean) => void;
  onToggleFlag: (sessionId: string, flagged: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;
  renamePendingRef: React.RefObject<boolean>;
}

const SessionItem = memo(function SessionItem({
  session,
  homeDir,
  isActive,
  statusValue,
  unreadSince,
  minuteTick: _minuteTick,
  isRenaming,
  renameValue,
  renameInputRef,
  onRenameValueChange,
  onConfirmRename,
  onCancelRename,
  onStartRename,
  onAttachSession,
  onDeleteSession,
  onToggleStar,
  onToggleFlag,
  onMarkRead,
  onMarkUnread,
  renamePendingRef,
}: SessionItemProps) {
  const repoPath = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;
  const { showMarkRead, showMarkUnread } = readMenuState(unreadSince, isActive);

  return (
    <div
      className={cn(
        "hover:bg-accent/50 has-[[data-state=open]]:bg-accent/50 group relative flex cursor-pointer items-center gap-1.5 rounded px-2 py-2",
        isActive && "bg-accent -ml-1.5 rounded-l-none pl-3.5"
      )}
      onClick={() => {
        if (!isRenaming) {
          onAttachSession(session.id);
        }
      }}
    >
      {/* Active session indicator pill — anchored to left border */}
      {isActive && (
        <span aria-hidden="true" className="bg-primary absolute left-0 top-0 h-full w-1 rounded-full" />
      )}
      {/* Session info — name, status, directory, branch */}
      <div className="min-w-0 flex-1">
        {isRenaming ? (
          <Input
            ref={renameInputRef}
            value={renameValue}
            onChange={(e) => onRenameValueChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                onConfirmRename();
              } else if (e.key === "Escape") {
                onCancelRename();
              }
            }}
            onBlur={onConfirmRename}
            className="h-6 text-sm"
            onClick={(e) => e.stopPropagation()}
          />
        ) : (
          <>
            <div className="flex min-w-0 items-center gap-1">
              <span className="truncate text-sm">
                {session.name || "Unnamed Session"}
              </span>
              {session.starred && (
                <Star className="h-3.5 w-3.5 flex-shrink-0 fill-amber-400 text-amber-400" />
              )}
              {session.flagged && (
                <Flag className="h-3.5 w-3.5 flex-shrink-0 text-orange-500" />
              )}
            </div>
            <div className="mt-0.5 flex items-center gap-1.5">
              <div
                className={cn(
                  "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                  unreadSince ? "bg-blue-500" : getStatusColor(statusValue),
                  !unreadSince && getStatusAnimation(statusValue)
                )}
              />
              <span className="text-muted-foreground text-xs">
                {(() => {
                  const label = unreadSince ? "Unread" : getStatusLabel(statusValue);
                  return label ? `${label} · ` : "";
                })()}
                {formatRelativeTime(session.updated_at)}
              </span>
            </div>
            {/* Line 3: Directory / Repo */}
            <span className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
              {session.git_parent_dir || session.git_remote_url || repoPath ? (
                <FolderGit2 className="h-3 w-3 flex-shrink-0" />
              ) : (
                <Folder className="h-3 w-3 flex-shrink-0" />
              )}
              <span className="truncate">
                {repoPath ??
                  compressPath(
                    session.git_parent_dir ?? session.working_directory,
                    homeDir,
                  )}
              </span>
            </span>
            {/* Line 4: Branch (worktree sessions only) */}
            {session.worktree_branch && (
              <span className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
                <GitBranch className="h-3 w-3 flex-shrink-0" />
                <span className="truncate">{session.worktree_branch}</span>
              </span>
            )}
          </>
        )}
      </div>

      {/* Actions menu — always visible on touch, hover on desktop */}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            onClick={(e) => e.stopPropagation()}
            className="text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5 opacity-100 md:opacity-0 md:group-hover:opacity-100 md:data-[state=open]:opacity-100"
            aria-label="Session actions"
          >
            <Ellipsis className="h-4 w-4" />
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          onCloseAutoFocus={(e) => {
            if (renamePendingRef.current) {
              e.preventDefault();
              renamePendingRef.current = false;
            }
          }}
        >
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onToggleStar(session.id, !session.starred);
            }}
          >
            <Star
              className={cn(
                "mr-2 h-3 w-3",
                session.starred && "fill-amber-400 text-amber-400"
              )}
            />
            {session.starred ? "Unstar" : "Star"}
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onToggleFlag(session.id, !session.flagged);
            }}
          >
            <Flag
              className={cn("mr-2 h-3 w-3", session.flagged && "text-orange-500")}
            />
            {session.flagged ? "Unflag" : "Flag"}
          </DropdownMenuItem>
          {showMarkRead && (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onMarkRead(session.id);
              }}
            >
              <MailOpen className="mr-2 h-3 w-3" />
              Mark as read
            </DropdownMenuItem>
          )}
          {showMarkUnread && (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onMarkUnread(session.id);
              }}
            >
              <Mail className="mr-2 h-3 w-3" />
              Mark as unread
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onStartRename(session);
            }}
          >
            <Pencil className="mr-2 h-3 w-3" />
            Rename
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onDeleteSession(session.id);
            }}
            className="text-red-500 focus:text-red-500"
          >
            <Trash2 className="mr-2 h-3 w-3" />
            Delete
          </DropdownMenuItem>
          {session.worktree_branch && (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onDeleteSession(session.id, true);
              }}
              className="text-red-500 focus:text-red-500"
            >
              <BrushCleaning className="mr-2 h-3 w-3" />
              Delete with branch
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
});

// ---------------------------------------------------------------------------
// SessionList
// ---------------------------------------------------------------------------

interface SessionListProps {
  sessions: Session[];
  homeDir: string;
  activeSessionId?: string;
  sessionStatuses?: Record<string, SessionStatusInfo>;
  isLoading?: boolean;
  isError?: boolean;
  errorMessage?: string;
  onAttachSession: (sessionId: string) => void;
  onDeleteSession: (sessionId: string, deleteBranch?: boolean) => void;
  onToggleStar: (sessionId: string, starred: boolean) => void;
  onToggleFlag: (sessionId: string, flagged: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;
  onRenameSession: (sessionId: string, newName: string) => void;
  onNewSession: () => void;
  onRetry?: () => void;
}

export const SessionList = memo(function SessionList({
  sessions,
  homeDir,
  activeSessionId,
  sessionStatuses,
  isLoading,
  isError,
  errorMessage,
  onAttachSession,
  onDeleteSession,
  onToggleStar,
  onToggleFlag,
  onMarkRead,
  onMarkUnread,
  onRenameSession,
  onNewSession,
  onRetry,
}: SessionListProps) {
  const [renamingSessionId, setRenamingSessionId] = useState<string | null>(
    null
  );
  const [renameValue, setRenameValue] = useState("");
  const renamePendingRef = useRef(false);
  const renameRafRef = useRef<number>(undefined);
  const renameInputRef = useCallback((el: HTMLInputElement | null) => {
    if (el) {
      // Use rAF to focus after Radix's close sequence completes
      renameRafRef.current = requestAnimationFrame(() => {
        el.focus();
        el.select();
      });
    } else if (renameRafRef.current) {
      cancelAnimationFrame(renameRafRef.current);
    }
  }, []);

  // Tick counter that increments every 60s so memoized SessionItems
  // re-render their relative timestamps (e.g. "2m ago" → "3m ago").
  const [minuteTick, setMinuteTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setMinuteTick((t) => t + 1), 60_000);
    return () => clearInterval(id);
  }, []);

  // Starred sessions first, then by updated_at descending within each group.
  const sortedSessions = useMemo(() => sortSessions(sessions), [sessions]);

  const handleStartRename = useCallback((session: Session) => {
    renamePendingRef.current = true;
    setRenamingSessionId(session.id);
    setRenameValue(session.name || "");
  }, []);

  const handleConfirmRename = useCallback(() => {
    if (renamingSessionId) {
      const trimmed = renameValue.trim();
      if (trimmed) {
        onRenameSession(renamingSessionId, trimmed);
      }
    }
    setRenamingSessionId(null);
    setRenameValue("");
  }, [renamingSessionId, renameValue, onRenameSession]);

  const handleCancelRename = useCallback(() => {
    setRenamingSessionId(null);
    setRenameValue("");
  }, []);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Session list */}
      <ScrollArea className="w-full flex-1">
        <div className="max-w-full space-y-0.5 px-1.5 py-1">
          {/* Loading state */}
          {isLoading && (
            <div className="flex flex-col items-center justify-center px-4 py-12">
              <div className="text-muted-foreground text-sm">
                Loading sessions...
              </div>
            </div>
          )}

          {/* Error state */}
          {isError && !isLoading && (
            <div className="flex flex-col items-center justify-center px-4 py-12">
              <AlertCircle className="text-destructive/50 mb-3 h-10 w-10" />
              <p className="text-destructive mb-2 text-sm">
                Failed to load sessions
              </p>
              <p className="text-muted-foreground mb-4 text-xs">
                {errorMessage || "Unknown error"}
              </p>
              {onRetry && (
                <Button variant="outline" onClick={onRetry} className="gap-2">
                  Retry
                </Button>
              )}
            </div>
          )}

          {/* Empty state */}
          {!isLoading && !isError && sessions.length === 0 && (
            <div className="flex flex-col items-center justify-center px-4 py-12">
              <p className="text-muted-foreground mb-4 text-center text-sm">
                No sessions yet. Create one to get started.
              </p>
              <Button onClick={onNewSession} className="gap-2">
                <Plus className="h-4 w-4" />
                New Session
              </Button>
            </div>
          )}

          {/* Flat session list */}
          {!isLoading &&
            !isError &&
            sortedSessions.map((session) => {
              const isRenaming = renamingSessionId === session.id;

              return (
                <SessionItem
                  key={session.id}
                  session={session}
                  homeDir={homeDir}
                  isActive={session.id === activeSessionId}
                  statusValue={sessionStatuses?.[session.id]?.status}
                  unreadSince={sessionStatuses?.[session.id]?.unreadSince}
                  minuteTick={minuteTick}
                  isRenaming={isRenaming}
                  renameValue={isRenaming ? renameValue : ""}
                  renameInputRef={renameInputRef}
                  onRenameValueChange={setRenameValue}
                  onConfirmRename={handleConfirmRename}
                  onCancelRename={handleCancelRename}
                  onStartRename={handleStartRename}
                  onAttachSession={onAttachSession}
                  onDeleteSession={onDeleteSession}
                  onToggleStar={onToggleStar}
                  onToggleFlag={onToggleFlag}
                  onMarkRead={onMarkRead}
                  onMarkUnread={onMarkUnread}
                  renamePendingRef={renamePendingRef}
                />
              );
            })}
        </div>
      </ScrollArea>
    </div>
  );
});
