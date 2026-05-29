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
import { Plus, AlertCircle, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch, BrushCleaning, Settings2, Pin, MailOpen, Mail, Info } from "lucide-react";
import { cn, formatRelativeTime, compressPath, parseRepoFromRemoteURL } from "@/lib/utils";
import {
  getStatusColor,
  getStatusAnimation,
  getStatusLabel,
} from "@/lib/sessionStatus";
import type { Session, SessionStatusInfo } from "@/types";
import { useProfilesQuery } from "@/data/sessions";
import { ProviderBadge } from "@/components/ProviderBadge";
import { Badge } from "@/components/ui/badge";

// Split sessions into pinned and the rest, each ordered by updated_at
// descending. Returns new arrays (does not mutate the input).
export function partitionSessions(sessions: Session[]): {
  pinned: Session[];
  rest: Session[];
} {
  const byUpdatedDesc = (a: Session, b: Session) =>
    new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime();
  return {
    pinned: sessions.filter((s) => s.pinned).sort(byUpdatedDesc),
    rest: sessions.filter((s) => !s.pinned).sort(byUpdatedDesc),
  };
}

// Decide which read/unread menu items to show. A session is "unread" when
// either the automatic unread_since or the manual user_marked_unread_at is set.
// "Mark as read" shows when unread; "Mark as unread" shows when read. The
// manual marker survives auto-acknowledge, so the active session is no longer
// special-cased.
export function readMenuState(
  unreadSince: string | null | undefined,
  userMarkedUnreadAt: string | null | undefined,
): { showMarkRead: boolean; showMarkUnread: boolean } {
  const isUnread = !!unreadSince || !!userMarkedUnreadAt;
  return { showMarkRead: isUnread, showMarkUnread: !isUnread };
}

// ---------------------------------------------------------------------------
// SectionHeader
// ---------------------------------------------------------------------------

function SectionHeader({ children }: { children: React.ReactNode }) {
  return (
    <div className="text-muted-foreground px-2 pt-2 pb-1 text-xs font-bold">
      {children}
    </div>
  );
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
  userMarkedUnreadAt?: string | null;
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
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
  canChangeProfile: boolean;
  onTogglePin: (sessionId: string, pinned: boolean) => void;
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
  userMarkedUnreadAt,
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
  onChangeProfile,
  onViewInfo,
  canChangeProfile,
  onTogglePin,
  onMarkRead,
  onMarkUnread,
  renamePendingRef,
}: SessionItemProps) {
  const repoPath = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;
  const isUnread = !!unreadSince || !!userMarkedUnreadAt;
  const { showMarkRead, showMarkUnread } = readMenuState(unreadSince, userMarkedUnreadAt);

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
            {/* Provider + auto-approve (YOLO) chips */}
            <div className="flex items-center gap-1.5">
              <ProviderBadge type={session.provider_type} />
              {session.auto_approve && (
                <Badge
                  variant="outline"
                  className="border-current px-1 py-0 text-[10px] font-medium text-yellow-500"
                >
                  YOLO
                </Badge>
              )}
            </div>
            <div className="mt-0.5 truncate text-sm">
              {session.name || "Unnamed Session"}
            </div>
            <div className="mt-0.5 flex items-center gap-1.5">
              <div
                className={cn(
                  "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                  isUnread ? "bg-blue-500" : getStatusColor(statusValue),
                  !isUnread && getStatusAnimation(statusValue)
                )}
              />
              <span className="text-muted-foreground min-w-0 truncate text-xs">
                {(() => {
                  const label = isUnread ? "Unread" : getStatusLabel(statusValue);
                  return label ? `${label} · ` : "";
                })()}
                {formatRelativeTime(session.updated_at)}
                {session.profile ? ` · ${session.profile}` : ""}
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
              onTogglePin(session.id, !session.pinned);
            }}
          >
            <Pin
              className={cn("mr-2 h-3 w-3", session.pinned && "fill-current")}
            />
            {session.pinned ? "Unpin" : "Pin"}
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
          {canChangeProfile && (
            <DropdownMenuItem
              onClick={(e) => {
                e.stopPropagation();
                onChangeProfile(session);
              }}
            >
              <Settings2 className="mr-2 h-3 w-3" />
              Change profile
            </DropdownMenuItem>
          )}
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onViewInfo(session);
            }}
          >
            <Info className="mr-2 h-3 w-3" />
            Info
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
  onTogglePin: (sessionId: string, pinned: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;
  onRenameSession: (sessionId: string, newName: string) => void;
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
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
  onTogglePin,
  onMarkRead,
  onMarkUnread,
  onRenameSession,
  onChangeProfile,
  onViewInfo,
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

  // Pinned sessions in their own group; the rest below. Each updated_at DESC.
  const { pinned, rest } = useMemo(() => partitionSessions(sessions), [sessions]);

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

  const { data: profilesData } = useProfilesQuery();
  const hasProfiles = (profilesData?.profiles?.length ?? 0) > 0;

  const renderItem = (session: Session) => {
    const isRenaming = renamingSessionId === session.id;
    return (
      <SessionItem
        key={session.id}
        session={session}
        homeDir={homeDir}
        isActive={session.id === activeSessionId}
        statusValue={sessionStatuses?.[session.id]?.status}
        unreadSince={sessionStatuses?.[session.id]?.unreadSince}
        userMarkedUnreadAt={sessionStatuses?.[session.id]?.userMarkedUnreadAt}
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
        onChangeProfile={onChangeProfile}
        onViewInfo={onViewInfo}
        canChangeProfile={hasProfiles || session.profile !== null}
        onTogglePin={onTogglePin}
        onMarkRead={onMarkRead}
        onMarkUnread={onMarkUnread}
        renamePendingRef={renamePendingRef}
      />
    );
  };

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

          {/* Pinned section — only when at least one session is pinned */}
          {!isLoading && !isError && pinned.length > 0 && (
            <>
              <SectionHeader>Pinned</SectionHeader>
              {pinned.map(renderItem)}
            </>
          )}

          {/* Recents section — the rest */}
          {!isLoading && !isError && rest.length > 0 && (
            <>
              <SectionHeader>Recents</SectionHeader>
              {rest.map(renderItem)}
            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
});
