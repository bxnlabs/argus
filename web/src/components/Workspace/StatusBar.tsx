import { Folder, FolderGit2, GitBranch, Settings2, Unplug } from "lucide-react";
import { toast } from "sonner";
import { cn, compressPath, truncateRight } from "@/lib/utils";
import { isMac } from "@/lib/device";
import { copyToClipboard } from "@/lib/clipboard";
import { getStatusMeta } from "@/lib/sessionStatus";
import { getSessionLocation } from "@/components/SessionInfoDialog/fields";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { Session, SessionStatusInfo } from "@/types";

// Widths inherited from the tmux status bar this replaced, which had the same
// job of keeping a long branch or a deep path from crowding out everything else.
const MAX_DIR_WIDTH = 50;
const MAX_BRANCH_WIDTH = 35;

export interface SessionCounts {
  total: number;
  active: number;
  unread: number;
}

/**
 * Fold the session list into the counts the status bar reports.
 *
 * Deliberately mirrors the server's rule in internal/node/api/summary.go: a
 * session that is active counts as active even when it also carries an unread
 * marker, so this bar and the node rail's attention badge can never disagree
 * about the same session. Sessions whose status hasn't landed yet count as
 * neither — the list and the status poll arrive independently.
 */
export function summarizeSessions(
  sessions: Session[],
  sessionStatuses: Record<string, SessionStatusInfo>,
): SessionCounts {
  let active = 0;
  let unread = 0;
  for (const s of sessions) {
    const info = sessionStatuses[s.id];
    if (info?.status === "active") {
      active++;
    } else if (info?.unreadSince || info?.userMarkedUnreadAt) {
      unread++;
    }
  }
  return { total: sessions.length, active, unread };
}

// One cell of the bar. Renders as a button when it does something and as plain
// text when it doesn't, so nothing offers a click that goes nowhere.
function StatusItem({
  testId,
  onClick,
  tooltip,
  ariaLabel,
  children,
}: {
  testId: string;
  onClick?: () => void;
  tooltip?: string;
  /** Required for icon-only items, which carry no text to name them. */
  ariaLabel?: string;
  children: React.ReactNode;
}) {
  // `min-w-0` is load-bearing: a flex child defaults to min-width:auto, which
  // pins it to its content and leaves the `truncate` on the text below with no
  // width to truncate against. Without it the identity cells push past the bar
  // instead of shrinking, and the last of them — the path — lands outside it.
  const className = cn(
    "flex h-full min-w-0 items-center gap-1.5 whitespace-nowrap rounded-sm px-2",
    onClick
      ? "hover:bg-accent/60 hover:text-foreground transition-colors"
      : ariaLabel && "opacity-40",
  );

  if (!onClick) {
    return (
      <span data-testid={testId} aria-label={ariaLabel} className={className}>
        {children}
      </span>
    );
  }

  const button = (
    <button
      type="button"
      data-testid={testId}
      onClick={onClick}
      aria-label={ariaLabel}
      className={className}
    >
      {children}
    </button>
  );

  if (!tooltip) return button;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side="top">{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function Dot({ color }: { color: string }) {
  return (
    <span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full", color)} />
  );
}

function Separator() {
  return (
    <span aria-hidden="true" className="text-muted-foreground/30 select-none">
      ·
    </span>
  );
}

interface StatusBarProps {
  sessions: Session[];
  sessionStatuses: Record<string, SessionStatusInfo>;
  /** The session the active tab is attached to, or null when detached. */
  session: Session | null;
  homeDir: string;
  /** Undefined when the session isn't in a git repo — the branch goes inert. */
  onOpenGit?: () => void;
  /** Undefined when no session is attached — the detach control goes inert. */
  onDetach?: () => void;
}

/**
 * The workspace's bottom bar: fleet counts on the left, the attached session's
 * identity on the right.
 *
 * The right half carries what tmux's status-right used to, in the same order —
 * id, profile, branch, directory — because that bar is now off (see
 * session.NewSession) and this is where the information went. Desktop only; the
 * caller gates it.
 */
export function StatusBar({
  sessions,
  sessionStatuses,
  session,
  homeDir,
  onOpenGit,
  onDetach,
}: StatusBarProps) {
  const { total, active, unread } = summarizeSessions(sessions, sessionStatuses);

  // Reuses the session-info dialog's rule for the repo/worktree split, so the
  // two never drift. `worktreeDir` is set only when the session's working
  // directory sits away from the repo root — the one case where the two differ.
  const location = session ? getSessionLocation(session, homeDir) : null;
  // Always the directory the session is actually in, so what the bar shows and
  // what a click copies are the same path.
  const dir = location ? (location.worktreeDir ?? location.directory).copy : "";
  // The repo name earns a segment only when the path stops carrying it. For a
  // session sitting in the repo itself the path already reads
  // `~/Workspace/repos/bxnlabs/argus`, and repeating it buys nothing.
  const repo = location?.worktreeDir ? location.repo : null;

  const copy = async (value: string, label: string) => {
    if (await copyToClipboard(value)) {
      toast.success(`Copied ${label}`);
    } else {
      toast.error("Copy failed");
    }
  };

  return (
    <div
      data-testid="status-bar"
      className="border-border bg-muted text-muted-foreground flex h-6 shrink-0 select-none items-center gap-0.5 border-t px-1 text-xs"
    >
      {/* The counts hold their width: they are short and the first thing the eye
          lands on, so the identity half yields instead. `h-full` on both groups
          is what lets the cells inside them fill the bar — a cell's own `h-full`
          resolves against its parent, so a content-height group would leave the
          detach control a half-height hit target and a hover box to match. */}
      <div
        data-testid="status-counts"
        className="flex h-full shrink-0 items-center gap-0.5"
      >
        {/* Detach leads the bar — it acts on the attached session, whose identity
            the bar's right half spells out. Icon-only, so it needs an aria-label;
            the shortcut lives in the tooltip, as it did on the tab bar. */}
        <StatusItem
          testId="status-detach"
          onClick={onDetach}
          ariaLabel="Detach session"
          tooltip={`Detach session (${isMac() ? "⌘ ;" : "Ctrl ;"} d)`}
        >
          <Unplug className="h-3 w-3 shrink-0" />
        </StatusItem>
        <Separator />

        {/* Read-only. Clicking a count should narrow the session list to exactly
            that count, which needs filters the quick switcher doesn't have yet;
            opening it unfiltered would answer a question nobody asked. */}
        <StatusItem testId="status-count-total">
          {total} {total === 1 ? "session" : "sessions"}
        </StatusItem>
        <Separator />
        <StatusItem testId="status-count-active">
          <Dot color={getStatusMeta("active").color} />
          {active} active
        </StatusItem>
        <Separator />
        <StatusItem testId="status-count-unread">
          {/* Blue means unread everywhere else in the app (session rows, node
              rail badges), so it means unread here too. */}
          <Dot color="bg-blue-500" />
          {unread} unread
        </StatusItem>
      </div>

      {session && (
        <div
          data-testid="status-identity"
          className="ml-auto flex h-full min-w-0 items-center gap-0.5 overflow-hidden"
        >
          <StatusItem
            testId="status-session-id"
            onClick={() => copy(session.id, "session ID")}
            tooltip="Copy session ID"
          >
            <span className="truncate font-mono">{session.id}</span>
          </StatusItem>

          {session.profile && (
            <>
              <Separator />
              <StatusItem testId="status-profile">
                <Settings2 className="h-3 w-3 shrink-0" />
                <span className="truncate">{session.profile}</span>
              </StatusItem>
            </>
          )}

          {session.worktree_branch && (
            <>
              <Separator />
              <StatusItem
                testId="status-branch"
                onClick={onOpenGit}
                tooltip="Open git panel"
              >
                <GitBranch className="h-3 w-3 shrink-0" />
                <span className="truncate">
                  {truncateRight(session.worktree_branch, MAX_BRANCH_WIDTH)}
                </span>
              </StatusItem>
            </>
          )}

          {repo && (
            <>
              <Separator />
              <StatusItem
                testId="status-repo"
                onClick={() => copy(repo, "repo")}
                tooltip="Copy repo"
              >
                <FolderGit2 className="h-3 w-3 shrink-0" />
                <span className="truncate">{repo}</span>
              </StatusItem>
            </>
          )}

          <Separator />
          <StatusItem
            testId="status-dir"
            // The compressed display elides path segments, so copy the real one.
            onClick={() => copy(dir, "path")}
            tooltip={dir}
          >
            <Folder className="h-3 w-3 shrink-0" />
            <span className="truncate">
              {compressPath(dir, homeDir, MAX_DIR_WIDTH)}
            </span>
          </StatusItem>
        </div>
      )}
    </div>
  );
}
