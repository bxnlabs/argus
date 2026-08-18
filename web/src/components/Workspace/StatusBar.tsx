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
import { cn, compressPath } from "@/lib/utils";
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

// The branch's cap is CSS, not a cut string. It still bounds the cell's flex
// basis, so one long branch can't inflate the identity half and crush the rest
// of it — uncapped, a 59-character branch left the repo cell 6% of its text at
// a 1280px bar — but it leaves the whole branch in the DOM. Cutting the string
// dropped the tail at every width, not only where the bar was short of room,
// and with git unavailable the cell is an inert span whose tooltip opens on
// hover alone: the tail then existed nowhere a keyboard or a screen reader
// could reach it.
//
// A backstop, not a routine trim. `ch` is the width of a zero and this font's
// average glyph is narrower, so 35ch measures 282px here rather than the 213px
// a 35-character cut produced — it starts biting around 59 characters. That
// slack is deliberate: at 213px the cap clipped the branch to 55% at a 1280px
// bar while the path kept 77%, inverting the ranking the shrink factors below
// exist to set.
const BRANCH_MAX_WIDTH = "max-w-[35ch]";

// The word labels and the node name are the first thing to go when the bar
// narrows: they carry the least per pixel, and the counts are what should yield
// to the attached session's identity rather than the other way round. Keyed off
// the bar's own width (`@container` below), not the window's — opening the
// sidebar takes ~340px out of the bar without touching the viewport.
//
// Dropping them is the *only* way the counts yield: the group is `shrink-0`
// (see below). So the breakpoint is not a refinement on top of flex shrink, it
// is the whole mechanism, and it wants to be early rather than late. Collapsing
// sooner than strictly necessary costs three words that the `sr-only` line and
// each count's tooltip still carry; collapsing later starves the identity,
// which is the half nothing else on screen repeats.
//
// No fixed threshold is right for every session: the identity's natural width
// is a name plus a profile plus a branch plus a path, so a session with long
// ones wants the labels gone long before a session with short ones does. What
// makes the choice tractable is that the two ways of being wrong don't cost the
// same. Collapsing earlier than a short session needed costs three words that
// the `sr-only` line and each count's tooltip still carry, and its identity
// stays whole either way. Collapsing later than a long session needed costs
// that identity — and costs it at a cliff, because the labels are 219px that
// appear all at once.
//
// Measured, both directions: a short session (`api`, `main`, `~/code/api`)
// keeps 100% of its identity at every width down to 680px whether the labels
// are there or not, so an early collapse costs it nothing but the words. A
// worktree session with all four fields long is the one that pays, and it pays
// at a step: with the labels gone it keeps everything, and the pixel they
// return on it loses ~100px of text at once.
//
// So the threshold goes where the labelled bar actually fits, and the width it
// has to fit is the widest the labelled group can get — not the width of a
// comfortable example. That group is bounded but not small: the node cell
// contributes up to its 16ch cap, and the numbers up to their digit count.
// Measured against the long identity, the narrowest bar at which nothing
// truncates is 1460px with an 11-character node and two-digit counts, 1524px
// with the node at its cap, and 1552px with three-digit counts as well. A
// threshold below the last of those doesn't remove the cliff, it just picks
// which configuration falls off it — 90rem was under even the first case.
//
// Hence 98rem. Deliberately past the measured worst case rather than at it,
// because the error is asymmetric and the slack is cheap: the only thing an
// over-wide threshold costs is three words on bars between there and where
// they'd have fit, and those words are still in the `sr-only` line and in every
// count's tooltip. What it buys is that no bounded configuration reads worse
// than it does one pixel narrower.
//
// It does not make that true for an unbounded one. The session name and the
// profile have no cap, so a long enough pair still truncates above any fixed
// threshold. That degrades by ellipsis rather than by a step, which is the part
// that holds regardless of content — but a bar that has to survive arbitrary
// identity strings wants a measured collapse, not a number in `rem`.
const WIDE = "hidden @min-[98rem]:inline";
const WIDE_CELL = "hidden @min-[98rem]:flex";

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
 * Whether the path already names this repo.
 *
 * It usually does — a session sitting in the repo itself reads
 * `~/Workspace/repos/bxnlabs/argus`, and a second `bxnlabs/argus` beside it buys
 * nothing. A worktree's path points away from the repo, and there the repo cell
 * is the only thing on the bar that says which project this is.
 *
 * Compared segment-wise against what is DISPLAYED rather than the absolute path:
 * the path shown is compressed, so a repo name buried in an elided middle isn't
 * readable and shouldn't count — and a plain substring test would find `argus`
 * inside `.argus` and strike out a segment that was carrying its own weight.
 *
 * The branch deliberately doesn't count, though it names a segment often enough
 * to look like it should. A branch namespace is arbitrary: `argus/main` in
 * `bxnlabs/argus` would suppress the cell while naming neither the owner nor,
 * for a repo whose basename another owner also uses, the project. It would not
 * pay for that in the case it was meant for either — the case is a worktree,
 * whose path doesn't name the repo, and whose branch here is `user/slug` and
 * doesn't name it either.
 */
export function repoNamedElsewhere(repo: string, dirDisplay: string): boolean {
  const name = repo.split("/").pop();
  if (!name) return false;
  return dirDisplay.split("/").includes(name);
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
  //
  // Known gap, accepted: an inert cell is a span, so it takes no focus and its
  // tooltip opens on hover only. It is a gap about legibility, not about data —
  // every inert cell keeps its whole value in the DOM, so a screen reader reads
  // what the ellipsis hides. What a sighted keyboard user can't recover is the
  // hidden tail, and the profile is the sharp case: inert, and on the same
  // shrink factor as the path, it measured 14 of 176px at a 680px bar.
  //
  // Not fixed by making these focusable. That buys the tail back at the price
  // of a tab stop per inert cell, and the cells that do something — the branch,
  // the repo, the path, the id, detach — are already buttons and already in the
  // tab order. So the new stops would all be on cells that go nowhere, which is
  // a poor trade in a bar this size. Revisit if an inert cell becomes
  // actionable, or for a keyboard-first user, for whom it is the wrong call.
  if (!tooltip || disabled) return item;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{item}</TooltipTrigger>
      <TooltipContent side="top">{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function Dot({
  color,
  animation,
  /**
   * Squares the mark off. Below the breakpoint the counts lose their words and
   * active and unread come down to two marks of the same size, so a reader who
   * can't separate green from blue has nothing left to go on — position won't
   * serve either, since a zero count is dropped rather than dimmed. Shape is
   * the one cue that costs no width, which is what the collapse was for.
   */
  square,
}: {
  color: string;
  animation?: string;
  square?: boolean;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "h-1.5 w-1.5 shrink-0",
        // Circles stay with liveness — the attached session's status dot is one
        // — and the square belongs to the unread marker alone.
        square ? "rounded-[1px]" : "rounded-full",
        color,
        animation,
      )}
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
    location?.repo && !repoNamedElsewhere(location.repo, dirDisplay)
      ? location.repo
      : null;

  const statusMeta = getStatusMeta(
    session ? sessionStatuses[session.id]?.status : undefined,
  );
  // getStatusMeta leaves the label empty for a status it doesn't know, which
  // includes the gap before the first status poll lands — the list and the poll
  // arrive independently, as summarizeSessions above depends on. Sighted, that
  // gap reads as the dot's muted "unknown" colour; without the fallback it
  // reads as nothing at all, because the dot is aria-hidden and the cell would
  // then announce a bare session name. The one thing this cell exists to say is
  // what the session is doing, so it says that it doesn't know.
  const statusLabel = statusMeta.label || "Status unknown";

  // Each count's own phrase, reused as its tooltip. Below the breakpoint the
  // words are gone and a count is a mark and a number, so the phrase is the
  // only place the full reading survives for a pointer. It backs up the mark's
  // shape rather than carrying the distinction alone — a tooltip needs a
  // pointer, and these cells take no focus.
  const totalLabel = `${total} ${total === 1 ? "session" : "sessions"}`;
  const activeLabel = `${active} active`;
  const unreadLabel = `${unread} unread`;

  // Spoken as one phrase, and with the zeros the bar hides: a screen reader
  // gets the whole picture in a single stop rather than a run of bare numbers.
  const countsLabel = [
    nodeName ? `${nodeName}:` : null,
    `${totalLabel},`,
    `${activeLabel},`,
    unreadLabel,
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
      {/* The counts in one phrase, with the zeros the bar hides. `sr-only` is
          absolutely positioned, so it sits outside the flex flow and costs the
          bar no width. Not a live region: these numbers move every time any
          agent anywhere wakes up, and narrating the whole fleet at a
          screen-reader user all day is worse than saying nothing. */}
      <span className="sr-only">{countsLabel}</span>

      {/* Fleet counts. They yield to the identity — they are an aggregate that
          changes slowly, against the one line on screen that says what you are
          looking at — but they yield by dropping their labels at the breakpoint
          above, not by shrinking.

          `shrink-0` is the point. Letting these shrink does not rank them below
          the identity: two flex siblings that both shrink give up the same
          *proportion* of their width, so the counts merely shrank alongside the
          identity rather than ahead of it, and the identity, being the wider of
          the two, lost more pixels doing it. Ranking them by an unequal shrink
          factor instead is worse still — the cells in here have no `truncate`
          to absorb it (a count is an icon and a number, and half a number is
          not a reading), so a shrunk group overlaps each cell's text with its
          neighbour's. The identity half shrinks well precisely because every
          cell in it truncates. These have two useful widths and nothing in
          between, so they are pinned to whichever one the bar is wide enough
          for.

          What that buys, stated no wider than it was measured: the optional
          labels collapse before the identity truncates, for identities up to
          the longest one measured against. A wider identity than that — enough
          long fields at once, or counts in the thousands — still truncates,
          because a threshold in `rem` cannot see the strings. It truncates
          rather than overlapping, which is the part that holds regardless.

          `h-full` on the group is what lets the cells inside it fill the bar: a
          cell's own `h-full` resolves against its parent, so a content-height
          group would leave half-height hit targets and hover boxes to match.

          Hidden from assistive tech, because the line above already says all of
          it. Naming the group instead left the cells in the tree underneath the
          name, where a reader that walks into the group commonly reaches them
          again: the summary, and then the run of bare numbers the summary was
          added to replace. Nothing in here takes focus, so hiding it strands no
          control. */}
      <div
        data-testid="status-counts"
        aria-hidden="true"
        className="flex h-full shrink-0 items-center gap-0.5 pr-1"
      >
        {nodeName && (
          <>
            {/* The counts are scoped to the active node, so on a bar that can
                show two nodes' worth of sessions over its lifetime, "7 sessions"
                alone is ambiguous. */}
            <StatusItem
              testId="status-node"
              className={WIDE_CELL}
              tooltip={nodeName}
            >
              {/* Capped for the same reason the branch is, and more sharply
                  now that the group is `shrink-0`: this is the one string in
                  here that isn't a small number, so it is the only way the
                  counts can inflate, and nothing gives way when they do. A
                  discovered node's name is derived from its hostname (see
                  deriveNodeName, which takes the first DNS label), but a manual
                  one is whatever the user typed (see registry.ManualNode) and
                  has no length anyone promised, which is the case the cap is
                  for. The tooltip holds whatever it takes off. */}
              <span className="max-w-[16ch] truncate">{nodeName}</span>
            </StatusItem>
            <Separator className={WIDE} />
          </>
        )}

        {/* Read-only. Clicking a count should narrow the session list to exactly
            that count, which needs filters the quick switcher doesn't have yet;
            opening it unfiltered would answer a question nobody asked. */}
        <StatusItem testId="status-count-total" tooltip={totalLabel}>
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
            <StatusItem testId="status-count-active" tooltip={activeLabel}>
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
            <StatusItem testId="status-count-unread" tooltip={unreadLabel}>
              {/* Blue means unread everywhere else in the app (session rows, node
                  rail badges), so it means unread here too — and the square is
                  what carries that meaning when the blue doesn't land. */}
              <Dot color="bg-blue-500" square />
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
              tooltip={`${statusLabel} · ${session.name}`}
            >
              <Dot color={statusMeta.color} animation={statusMeta.animation} />
              <span className="sr-only">{statusLabel}</span>
              <span className="truncate">{session.name}</span>
            </StatusItem>

            {session.profile && (
              <>
                <Separator />
                <StatusItem
                  testId="status-profile"
                  className="shrink-[8]"
                  tooltip={session.profile}
                >
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
                  <span className={cn("truncate", BRANCH_MAX_WIDTH)}>
                    {session.worktree_branch}
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
