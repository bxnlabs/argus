import {
  Folder,
  FolderGit2,
  GitBranch,
  Hash,
  Layers,
  Settings2,
  Unplug,
} from "lucide-react";
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

// The word labels and the node name are the first thing to go when the bar
// narrows: they carry the least per pixel, and the counts are what should yield
// to the attached session's identity rather than the other way round. Keyed off
// the bar's own width (`@container` below), not the window's — opening the
// sidebar takes ~340px out of the bar without touching the viewport.
const WIDE = "hidden @5xl:inline";
const WIDE_CELL = "hidden @5xl:flex";

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

/**
 * Whether something else on the bar already names this repo.
 *
 * The path usually does — a session sitting in the repo itself reads
 * `~/Workspace/repos/bxnlabs/argus`, and a second `bxnlabs/argus` beside it buys
 * nothing. A worktree's path points away from the repo, and there the repo cell
 * is the only thing on the bar that says which project this is.
 *
 * Compared segment-wise against what is DISPLAYED rather than the absolute path:
 * the path shown is compressed, so a repo name buried in an elided middle isn't
 * readable and shouldn't count — and a plain substring test would find `argus`
 * inside `.argus` and strike out a segment that was carrying its own weight.
 */
export function repoNamedElsewhere(
  repo: string,
  dirDisplay: string,
  branch: string | null,
): boolean {
  const name = repo.split("/").pop();
  if (!name) return false;
  const segments = [...dirDisplay.split("/"), ...(branch?.split("/") ?? [])];
  return segments.includes(name);
}

// One cell of the bar. Renders as a button when it does something and as plain
// text when it doesn't, so nothing offers a click that goes nowhere. A cell that
// is disabled rather than merely inert stays a button: it has an action, it just
// can't take it right now, and only a real `disabled` button says that to
// assistive tech instead of dimming itself and hoping.
function StatusItem({
  testId,
  onClick,
  disabled,
  tooltip,
  ariaLabel,
  className,
  children,
}: {
  testId: string;
  onClick?: () => void;
  disabled?: boolean;
  tooltip?: string;
  /** Required for icon-only items, which carry no text to name them. */
  ariaLabel?: string;
  className?: string;
  children: React.ReactNode;
}) {
  // `min-w-0` is load-bearing: a flex child defaults to min-width:auto, which
  // pins it to its content and leaves the `truncate` on the text below with no
  // width to truncate against. Without it the identity cells push past the bar
  // instead of shrinking, and the last of them — the path — lands outside it.
  const classes = cn(
    "flex h-full min-w-0 items-center gap-1.5 whitespace-nowrap rounded-sm px-2",
    onClick &&
      !disabled &&
      "hover:bg-accent/60 hover:text-foreground transition-colors",
    disabled && "opacity-50",
    className,
  );

  const item =
    !onClick && !disabled ? (
      <span data-testid={testId} aria-label={ariaLabel} className={classes}>
        {children}
      </span>
    ) : (
      <button
        type="button"
        data-testid={testId}
        onClick={onClick}
        disabled={disabled}
        aria-label={ariaLabel}
        className={classes}
      >
        {children}
      </button>
    );

  // Inert cells get tooltips too: every cell here can be ellipsed down to its
  // icon, and the tooltip is then the only place the value survives.
  //
  // A disabled button is the exception — it takes no pointer events, so Radix
  // never sees the hover that would open one. Nothing to hang it on.
  if (!tooltip || disabled) return item;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{item}</TooltipTrigger>
      <TooltipContent side="top">{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function Dot({ color, animation }: { color: string; animation?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn("h-1.5 w-1.5 shrink-0 rounded-full", color, animation)}
    />
  );
}

function Separator({ className }: { className?: string }) {
  return (
    <span
      aria-hidden="true"
      className={cn("text-muted-foreground/30 select-none", className)}
    >
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
  /** Names the node the counts are counting; null before the nodes load. */
  nodeName?: string | null;
  /** Undefined when the session isn't in a git repo — the branch goes inert. */
  onOpenGit?: () => void;
  /** Undefined when no session is attached — the detach control goes disabled. */
  onDetach?: () => void;
}

/**
 * The workspace's bottom bar: the active node's session counts on the left, the
 * attached session's identity on the right, and the controls that act on that
 * session at the trailing edge beside it.
 *
 * The right half carries what tmux's status-right used to — that bar is now off
 * (see session.NewSession) and this is where the information went — but led by
 * the session's own status and name rather than its id, because that is the
 * question a status bar is asked most often and the id is a copy target, not a
 * readout. Desktop only; the caller gates it.
 */
export function StatusBar({
  sessions,
  sessionStatuses,
  session,
  homeDir,
  nodeName,
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
  const dirDisplay = compressPath(dir, homeDir, MAX_DIR_WIDTH);
  const repo =
    location?.repo &&
    !repoNamedElsewhere(location.repo, dirDisplay, location.branch)
      ? location.repo
      : null;

  const statusMeta = getStatusMeta(
    session ? sessionStatuses[session.id]?.status : undefined,
  );

  // Spoken as one phrase, and with the zeros the bar hides: a screen reader
  // gets the whole picture in a single stop rather than a run of bare numbers.
  const countsLabel = [
    nodeName ? `${nodeName}:` : null,
    `${total} ${total === 1 ? "session" : "sessions"},`,
    `${active} active,`,
    `${unread} unread`,
  ]
    .filter(Boolean)
    .join(" ");

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
      className="@container border-border bg-muted text-muted-foreground flex h-6 shrink-0 select-none items-center gap-0.5 border-t px-1 text-xs"
    >
      {/* Fleet counts. They shrink before the identity does — they are an
          aggregate that changes slowly, against the one line on screen that says
          what you are looking at. `h-full` on the group is what lets the cells
          inside it fill the bar: a cell's own `h-full` resolves against its
          parent, so a content-height group would leave half-height hit targets
          and hover boxes to match.

          `role="group"` rather than `role="status"`: these numbers move every
          time any agent anywhere wakes up, and a live region would narrate the
          whole fleet at a screen-reader user all day. The name is what was
          missing, not the announcement. */}
      <div
        data-testid="status-counts"
        role="group"
        aria-label={countsLabel}
        className="flex h-full min-w-0 shrink items-center gap-0.5 overflow-hidden pr-1"
      >
        {nodeName && (
          <>
            {/* The counts are scoped to the active node, so on a bar that can
                show two nodes' worth of sessions over its lifetime, "7 sessions"
                alone is ambiguous. */}
            <StatusItem testId="status-node" className={WIDE_CELL}>
              <span className="truncate">{nodeName}</span>
            </StatusItem>
            <Separator className={WIDE} />
          </>
        )}

        {/* Read-only. Clicking a count should narrow the session list to exactly
            that count, which needs filters the quick switcher doesn't have yet;
            opening it unfiltered would answer a question nobody asked. */}
        <StatusItem testId="status-count-total">
          {/* Earns the icon because the word is the part that drops on a narrow
              bar, and a bare "7" names nothing. */}
          <Layers className="h-3 w-3 shrink-0" />
          <span className="tabular-nums">
            {total}
            <span className={WIDE}> {total === 1 ? "session" : "sessions"}</span>
          </span>
        </StatusItem>

        {/* Zero states are dropped, not dimmed: "0 unread" held 78px of a bar
            whose identity half was ellipsing four fields at once. The counts sit
            left of an `ml-auto` group, so a count appearing or leaving moves
            nothing but itself. */}
        {active > 0 && (
          <>
            <Separator />
            <StatusItem testId="status-count-active">
              <Dot color={getStatusMeta("active").color} />
              <span className="tabular-nums">
                {active}
                <span className={WIDE}> active</span>
              </span>
            </StatusItem>
          </>
        )}
        {unread > 0 && (
          <>
            <Separator />
            <StatusItem testId="status-count-unread">
              {/* Blue means unread everywhere else in the app (session rows, node
                  rail badges), so it means unread here too. */}
              <Dot color="bg-blue-500" />
              <span className="tabular-nums">
                {unread}
                <span className={WIDE}> unread</span>
              </span>
            </StatusItem>
          </>
        )}
      </div>

      {/* Everything session-scoped, behind a rule. The boundary between the
          fleet and the attached session used to be a fourth `·`, identical to
          the ones dividing cells within each group — at 1280px the gap across it
          measured 2px, so seven cells read as one run. */}
      <div className="border-border/60 ml-auto flex h-full min-w-0 items-center gap-0.5 border-l pl-1.5">
        {session && (
          <div
            data-testid="status-identity"
            className="flex h-full min-w-0 items-center gap-0.5 overflow-hidden"
          >
            {/* Leads with the state of the session you are attached to — the
                question a status bar exists to answer, and the one thing the
                fleet dots could not say. */}
            <StatusItem
              testId="status-session"
              className="shrink-[2]"
              tooltip={
                statusMeta.label
                  ? `${statusMeta.label} · ${session.name}`
                  : session.name
              }
            >
              <Dot color={statusMeta.color} animation={statusMeta.animation} />
              {statusMeta.label && (
                <span className="sr-only">{statusMeta.label}</span>
              )}
              <span className="truncate">{session.name}</span>
            </StatusItem>

            {session.profile && (
              <>
                <Separator />
                <StatusItem testId="status-profile" className="shrink-[8]">
                  <Settings2 className="h-3 w-3 shrink-0" />
                  <span className="truncate">{session.profile}</span>
                </StatusItem>
              </>
            )}

            {session.worktree_branch && (
              <>
                <Separator />
                {/* Lowest shrink factor on the bar: of everything here, the
                    branch is what gets read, and it was ellipsing at the same
                    rate as the id and the path. */}
                <StatusItem
                  testId="status-branch"
                  className="shrink-[1]"
                  onClick={onOpenGit}
                  tooltip={
                    onOpenGit
                      ? `Open git panel (${session.worktree_branch})`
                      : session.worktree_branch
                  }
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
                  className="shrink-[16]"
                  onClick={() => copy(repo, "repo")}
                  tooltip={`Copy repo (${repo})`}
                >
                  <FolderGit2 className="h-3 w-3 shrink-0" />
                  <span className="truncate">{repo}</span>
                </StatusItem>
              </>
            )}

            <Separator />
            {/* Yields early and degrades to its icon: the display is already
                middle-elided by compressPath, and a path that is elided twice
                has stopped being readable — what survives is the tooltip and
                the click, both of which carry the whole path. */}
            <StatusItem
              testId="status-dir"
              className="shrink-[8]"
              // The compressed display elides path segments, so copy the real one.
              onClick={() => copy(dir, "path")}
              tooltip={dir}
            >
              <Folder className="h-3 w-3 shrink-0" />
              <span className="truncate">{dirDisplay}</span>
            </StatusItem>
          </div>
        )}

        {/* The two things you can do to the attached session, at the end of the
            line that describes it. Detach used to lead the bar, ~700px from the
            session it detaches and glued by proximity to counts it has nothing
            to do with. */}
        <div
          data-testid="status-actions"
          className="flex h-full shrink-0 items-center gap-0.5"
        >
          {session && (
            // Icon-only: the id is a copy target, not a readout. It spent 160px
            // at the head of the identity half being the least-read string on
            // the bar; the tooltip still shows it in full.
            <StatusItem
              testId="status-session-id"
              onClick={() => copy(session.id, "session ID")}
              ariaLabel="Copy session ID"
              tooltip={`Copy session ID (${session.id})`}
            >
              <Hash className="h-3 w-3 shrink-0" />
            </StatusItem>
          )}
          {/* Disabled rather than dropped when nothing is attached, so the end
              of the bar doesn't reshuffle each time a tab detaches. */}
          <StatusItem
            testId="status-detach"
            onClick={onDetach}
            disabled={!onDetach}
            ariaLabel="Detach session"
            tooltip={`Detach session (${isMac() ? "⌘ ;" : "Ctrl ;"} d)`}
          >
            <Unplug className="h-3 w-3 shrink-0" />
          </StatusItem>
        </div>
      </div>
    </div>
  );
}
