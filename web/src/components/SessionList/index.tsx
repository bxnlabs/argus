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
import { Plus, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch, BrushCleaning, Settings2, Pin, MailOpen, Mail, Info, ChevronRight, Copy, Loader2 } from "lucide-react";
import { cn, formatRelativeTime, compressPath, parseRepoFromRemoteURL } from "@/lib/utils";
import { getStatusMeta } from "@/lib/sessionStatus";
import type { Session, SessionStatusInfo } from "@/types";
import { useProfilesQuery } from "@/data/sessions";
import type { BusyKind } from "@/hooks/useSessionMutationState";
import { useSessionMutationState } from "@/hooks/useSessionMutationState";
import {
  loadSectionCollapse,
  saveSectionCollapse,
  toggleSection,
  type SidebarSectionKey,
} from "@/lib/sidebarSections";

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

// resolveStatusDisplay computes the sidebar status dot + label. Active wins:
// while a session is actively running, surface the live status even if it's
// flagged unread. The unread marker persists in the data and re-surfaces as a
// blue "Unread" indicator once the session goes idle/dead. This keeps the
// indicator (live activity) independent from the read-menu (persistent flag).
export function resolveStatusDisplay(
  statusValue: string | undefined,
  unreadSince: string | null | undefined,
  userMarkedUnreadAt: string | null | undefined,
): { label: string; dotColor: string; animation: string } {
  const statusMeta = getStatusMeta(statusValue);
  const isUnread = !!unreadSince || !!userMarkedUnreadAt;
  if (isUnread && statusValue !== "active") {
    return { label: "Unread", dotColor: "bg-blue-500", animation: "" };
  }
  return { label: statusMeta.label, dotColor: statusMeta.color, animation: statusMeta.animation };
}

const BUSY_LABEL: Record<BusyKind, string> = {
  cloning: "Cloning…",
  deleting: "Deleting…",
  profile: "Updating profile…",
};

// Row display with in-flight mutations folded in. Busy outranks everything:
// a row being deleted shows that, not its last known status or unread marker.
export function resolveRowDisplay(
  busy: BusyKind | undefined,
  statusValue: string | undefined,
  unreadSince: string | null | undefined,
  userMarkedUnreadAt: string | null | undefined,
): { label: string; dotColor: string; animation: string; spinner: boolean } {
  if (busy) {
    return { label: BUSY_LABEL[busy], dotColor: "", animation: "", spinner: true };
  }
  return {
    ...resolveStatusDisplay(statusValue, unreadSince, userMarkedUnreadAt),
    spinner: false,
  };
}

// ---------------------------------------------------------------------------
// SessionListSkeleton
// ---------------------------------------------------------------------------

// Placeholder rows for a list that hasn't arrived yet. Mirrors a real row's
// structure — px-2 py-2 around a name line, a dot-and-status line, and the
// icon-and-label lines a worktree session carries — so a placeholder stands in
// at the real row height rather than a fraction of it. How many rows are coming
// isn't knowable, so the list still resizes when the real ones land.
//
// Widths are staggered so it reads as content taking shape. Same muted pulse the
// editor uses while a file loads (FileExplorer's EditorSkeleton).
const SKELETON_WIDTHS = [82, 54, 71, 44, 63];

// Repo and branch lines, scaled off the name width so each row keeps its own
// ragged silhouette.
const SKELETON_DETAIL_SCALES = [0.7, 0.85];

export function SessionListSkeleton() {
  return (
    <div role="status" aria-label="Loading sessions" data-testid="session-list-skeleton">
      {SKELETON_WIDTHS.map((w, i) => (
        <div key={i} className="px-2 py-2">
          {/* Name — text-sm's 20px line box */}
          <div className="flex h-5 items-center">
            <div
              className="bg-muted h-3 animate-pulse rounded"
              style={{ width: `${w}%` }}
            />
          </div>
          {/* Status — dot and label, on text-xs's 16px line box */}
          <div className="mt-0.5 flex h-4 items-center gap-1.5">
            <div className="bg-muted h-1.5 w-1.5 flex-shrink-0 animate-pulse rounded-full" />
            <div
              className="bg-muted h-2 animate-pulse rounded"
              style={{ width: `${Math.round(w * 0.55)}%` }}
            />
          </div>
          {SKELETON_DETAIL_SCALES.map((scale, j) => (
            <div key={j} className="mt-0.5 flex h-4 items-center gap-1">
              <div className="bg-muted h-3 w-3 flex-shrink-0 animate-pulse rounded-sm" />
              <div
                className="bg-muted h-2 animate-pulse rounded"
                style={{ width: `${Math.round(w * scale)}%` }}
              />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

// ---------------------------------------------------------------------------
// SectionHeader
// ---------------------------------------------------------------------------

export function SectionHeader({
  children,
  collapsed,
  onToggle,
}: {
  children: React.ReactNode;
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={!collapsed}
      className="text-muted-foreground hover:text-foreground flex w-full items-center px-2 pt-2 pb-1 text-xs font-bold transition-colors"
    >
      <span>{children}</span>
      <ChevronRight
        aria-hidden="true"
        className={cn(
          "ml-auto h-3.5 w-3.5 transition-transform",
          !collapsed && "rotate-90",
        )}
      />
    </button>
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
  onCloneSession: (sessionId: string) => void;
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
  canChangeProfile: boolean;
  onTogglePin: (sessionId: string, pinned: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;
  renamePendingRef: React.RefObject<boolean>;
  busy?: BusyKind;
}

export const SessionItem = memo(function SessionItem({
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
  onCloneSession,
  onChangeProfile,
  onViewInfo,
  canChangeProfile,
  onTogglePin,
  onMarkRead,
  onMarkUnread,
  renamePendingRef,
  busy,
}: SessionItemProps) {
  const repoPath = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;
  const { showMarkRead, showMarkUnread } = readMenuState(unreadSince, userMarkedUnreadAt);
  const { label: statusLabel, dotColor, animation, spinner } = resolveRowDisplay(
    busy,
    statusValue,
    unreadSince,
    userMarkedUnreadAt,
  );

  return (
    <div
      aria-busy={busy ? true : undefined}
      className={cn(
        "hover:bg-accent/50 has-[[data-state=open]]:bg-accent/50 group relative flex cursor-pointer items-center gap-1.5 rounded px-2 py-2",
        isActive && "bg-accent -ml-1.5 rounded-l-none pl-3.5",
        busy && "pointer-events-none opacity-60",
      )}
      onClick={() => {
        if (busy) return;
        if (!isRenaming) {
          onAttachSession(session.id);
        }
      }}
    >
      {/* Active session indicator pill — anchored to left border. White is the
          shared currency color across the session list, node rail, and view-mode
          rail, leaving blue to mean "unread". */}
      {isActive && (
        <span
          aria-hidden="true"
          data-testid="session-pill"
          className="absolute left-0 top-0 h-full w-1 rounded-full bg-white"
        />
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
            <div className="truncate text-sm">
              {session.name || "Unnamed Session"}
            </div>
            <div className="mt-0.5 flex items-center gap-1.5">
              {spinner ? (
                <Loader2
                  aria-hidden="true"
                  className="text-muted-foreground h-3 w-3 flex-shrink-0 animate-spin"
                />
              ) : (
                <div
                  className={cn(
                    "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                    dotColor,
                    animation
                  )}
                />
              )}
              <span className="text-muted-foreground min-w-0 truncate text-xs">
                {statusLabel ? `${statusLabel} · ` : ""}
                {formatRelativeTime(session.updated_at)}
              </span>
            </div>
            {/* Profile (when set) — its own line, right after status */}
            {session.profile && (
              <span className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
                <Settings2 className="h-3 w-3 flex-shrink-0" />
                <span className="truncate">{session.profile}</span>
              </span>
            )}
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

      {/* Actions menu — always visible on touch, hover on desktop. While busy
          it stays mounted and focusable, just inert: choosing Clone/Delete
          closes the menu in the same commit that makes the row busy, and Radix
          restores focus to the trigger right after. Unmounting it — or setting
          the `disabled` attribute, which makes it un-focusable — drops focus to
          <body>, so the next Tab restarts from the top of the document.
          aria-disabled keeps it a focus target; preventDefault on the opening
          gestures stops Radix's composed handlers from opening the menu.

          The two gesture guards are not equals. onKeyDown is the load-bearing
          one: the row's `pointer-events-none` does not apply to the keyboard,
          and the trigger is deliberately still focusable. onPointerDown is a
          backstop — real pointer input never reaches it through that row, so it
          only catches synthesised/programmatic events, which is also why its
          test has to dispatch the event directly. Removing it is safe; removing
          `pointer-events-none` in favour of it is not. */}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <button
            onClick={(e) => e.stopPropagation()}
            aria-disabled={busy !== undefined}
            onPointerDown={(e) => {
              if (busy) e.preventDefault();
            }}
            onKeyDown={(e) => {
              // Exactly the keys Radix's trigger opens on. Anything wider
              // swallows Tab and traps focus on a control we deliberately kept
              // focusable — the same failure this whole branch exists to avoid.
              if (busy && ["Enter", " ", "ArrowDown"].includes(e.key)) {
                e.preventDefault();
              }
            }}
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
              onCloneSession(session.id);
            }}
          >
            <Copy className="mr-2 h-3 w-3" />
            Clone
          </DropdownMenuItem>
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
  onAttachSession: (sessionId: string) => void;
  onDeleteSession: (sessionId: string, deleteBranch?: boolean) => void;
  onCloneSession: (sessionId: string) => void;
  onTogglePin: (sessionId: string, pinned: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;
  onRenameSession: (sessionId: string, newName: string) => void;
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
  onNewSession: () => void;
}

export const SessionList = memo(function SessionList({
  sessions,
  homeDir,
  activeSessionId,
  sessionStatuses,
  isLoading,
  onAttachSession,
  onDeleteSession,
  onCloneSession,
  onTogglePin,
  onMarkRead,
  onMarkUnread,
  onRenameSession,
  onChangeProfile,
  onViewInfo,
  onNewSession,
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

  // Collapsed/expanded state for the Pinned and Recents sections, hydrated
  // from localStorage on mount and persisted whenever it changes.
  const [sectionCollapse, setSectionCollapse] = useState(loadSectionCollapse);
  useEffect(() => {
    saveSectionCollapse(sectionCollapse);
  }, [sectionCollapse]);
  const handleToggleSection = useCallback((key: SidebarSectionKey) => {
    setSectionCollapse((prev) => toggleSection(prev, key));
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
  const { busySessions } = useSessionMutationState();

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
        onCloneSession={onCloneSession}
        onChangeProfile={onChangeProfile}
        onViewInfo={onViewInfo}
        canChangeProfile={hasProfiles || session.profile !== null}
        onTogglePin={onTogglePin}
        onMarkRead={onMarkRead}
        onMarkUnread={onMarkUnread}
        renamePendingRef={renamePendingRef}
        busy={busySessions[session.id]}
      />
    );
  };

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Session list */}
      <ScrollArea className="w-full flex-1">
        <div className="max-w-full space-y-0.5 px-1.5 py-1">
          {/* Waiting on the list, including when the last fetch failed: the
              query keeps polling, so the placeholder rows stay up and the
              failure is announced by a toast. */}
          {isLoading && <SessionListSkeleton />}

          {/* Empty state — only once a list has arrived and is empty */}
          {!isLoading && sessions.length === 0 && (
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
          {!isLoading && pinned.length > 0 && (
            <>
              <SectionHeader
                collapsed={sectionCollapse.pinned}
                onToggle={() => handleToggleSection("pinned")}
              >
                Pinned
              </SectionHeader>
              {!sectionCollapse.pinned && pinned.map(renderItem)}
            </>
          )}

          {/* Recents section — the rest */}
          {!isLoading && rest.length > 0 && (
            <>
              <SectionHeader
                collapsed={sectionCollapse.recents}
                onToggle={() => handleToggleSection("recents")}
              >
                Recents
              </SectionHeader>
              {!sectionCollapse.recents && rest.map(renderItem)}
            </>
          )}
        </div>
      </ScrollArea>
    </div>
  );
});
