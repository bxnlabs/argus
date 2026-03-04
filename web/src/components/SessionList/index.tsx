import { useState, useMemo, useRef, useCallback } from "react";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Plus, AlertCircle, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch } from "lucide-react";
import { cn, formatRelativeTime, compressPath } from "@/lib/utils";
import type { Session, SessionStatusInfo } from "@/types";

function getStatusColor(status?: string) {
  switch (status) {
    case "running":
      return "bg-green-500";
    case "waiting":
      return "bg-yellow-500";
    case "idle":
      return "bg-muted-foreground";
    case "error":
      return "bg-red-500";
    case "dead":
      return "bg-red-500/50";
    default:
      return "bg-muted-foreground/40";
  }
}

function getStatusAnimation(status?: string) {
  switch (status) {
    case "running":
      return "animate-pulse-green";
    case "waiting":
      return "animate-pulse-yellow";
    default:
      return "";
  }
}

function getStatusLabel(status?: string) {
  switch (status) {
    case "running":
      return "Running";
    case "waiting":
      return "Needs input";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    case "error":
      return "Error";
    default:
      return "";
  }
}

interface SessionListProps {
  sessions: Session[];
  homeDir: string;
  activeSessionId?: string;
  sessionStatuses?: Record<string, SessionStatusInfo>;
  isLoading?: boolean;
  isError?: boolean;
  errorMessage?: string;
  onAttachSession: (sessionId: string) => void;
  onDeleteSession: (sessionId: string) => void;
  onRenameSession: (sessionId: string, newName: string) => void;
  onNewSession: () => void;
  onRetry?: () => void;
}

export function SessionList({
  sessions,
  homeDir,
  activeSessionId,
  sessionStatuses,
  isLoading,
  isError,
  errorMessage,
  onAttachSession,
  onDeleteSession,
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

  // Sessions sorted by updated_at descending
  const sortedSessions = useMemo(
    () =>
      [...sessions].sort(
        (a, b) =>
          new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()
      ),
    [sessions]
  );

  const handleStartRename = (session: Session) => {
    renamePendingRef.current = true;
    setRenamingSessionId(session.id);
    setRenameValue(session.name || "");
  };

  const handleConfirmRename = () => {
    if (renamingSessionId && renameValue.trim()) {
      onRenameSession(renamingSessionId, renameValue.trim());
    }
    setRenamingSessionId(null);
    setRenameValue("");
  };

  const handleCancelRename = () => {
    setRenamingSessionId(null);
    setRenameValue("");
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

          {/* Flat session list */}
          {!isLoading &&
            !isError &&
            sortedSessions.map((session) => {
              const status = sessionStatuses?.[session.id];
              const isActive = session.id === activeSessionId;
              const isRenaming = renamingSessionId === session.id;

              return (
                <div
                  key={session.id}
                  className={cn(
                    "hover:bg-accent/50 has-[[data-state=open]]:bg-accent/50 group relative flex cursor-pointer items-center gap-1.5 rounded px-2 py-2",
                    isActive && "bg-accent -mr-1.5 rounded-r-none pr-3.5"
                  )}
                  onClick={() => {
                    if (!isRenaming) {
                      onAttachSession(session.id);
                    }
                  }}
                >
                  {/* Active session indicator pill — anchored to right border */}
                  {isActive && (
                    <div className="bg-primary absolute right-0 top-1/2 h-4 w-1 -translate-y-1/2 rounded-full" />
                  )}
                  {/* Session info — name, status, directory, branch */}
                  <div className="min-w-0 flex-1">
                    {isRenaming ? (
                      <Input
                        ref={renameInputRef}
                        value={renameValue}
                        onChange={(e) => setRenameValue(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") {
                            e.preventDefault();
                            handleConfirmRename();
                          } else if (e.key === "Escape") {
                            handleCancelRename();
                          }
                        }}
                        onBlur={handleConfirmRename}
                        className="h-6 text-sm"
                        onClick={(e) => e.stopPropagation()}
                      />
                    ) : (
                      <>
                        <span className="block truncate text-sm">
                          {session.name || "Unnamed Session"}
                        </span>
                        <div className="mt-0.5 flex items-center gap-1.5">
                          <div
                            className={cn(
                              "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                              getStatusColor(status?.status),
                              getStatusAnimation(status?.status)
                            )}
                          />
                          <span className="text-muted-foreground text-xs">
                            {(() => {
                              const label = getStatusLabel(status?.status);
                              return label ? `${label} · ` : "";
                            })()}
                            {formatRelativeTime(session.updated_at)}
                          </span>
                        </div>
                        {/* Line 3: Directory */}
                        <span className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
                          {session.git_parent_dir ? (
                            <FolderGit2 className="h-3 w-3 flex-shrink-0" />
                          ) : (
                            <Folder className="h-3 w-3 flex-shrink-0" />
                          )}
                          <span className="truncate">
                            {compressPath(
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
                          handleStartRename(session);
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
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              );
            })}
        </div>
      </ScrollArea>
    </div>
  );
}
