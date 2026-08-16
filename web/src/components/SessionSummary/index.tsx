import { cn } from "@/lib/utils";
import type { Session, SessionStatusInfo } from "@/types";

export interface SessionCounts {
  active: number;
  unread: number;
  total: number;
}

/**
 * Roll up the sidebar's sessions into the two counts worth stating at a glance.
 * Only ids are read, so callers can pass the session list as-is.
 *
 * Active outranks unread, matching both the node summary endpoint
 * (`internal/node/api/summary.go`) and the row's own status dot
 * (`resolveStatusDisplay`): a running session reads as active even while its
 * unread flag persists underneath. That keeps the two counts disjoint, so they
 * can be read as parts of the total rather than overlapping tallies.
 */
export function countSessions(
  sessions: Pick<Session, "id">[],
  sessionStatuses: Record<string, SessionStatusInfo>,
): SessionCounts {
  let active = 0;
  let unread = 0;
  for (const session of sessions) {
    const status = sessionStatuses[session.id];
    if (status?.status === "active") {
      active++;
    } else if (status && (status.unreadSince || status.userMarkedUnreadAt)) {
      unread++;
    }
  }
  return { active, unread, total: sessions.length };
}

function Segment({ count, label, dot }: { count: number; label: string; dot: string }) {
  return (
    <span className="flex items-center gap-1">
      <span aria-hidden="true" className={cn("h-1.5 w-1.5 rounded-full", dot)} />
      {count} {label}
    </span>
  );
}

/**
 * One-line rollup of the current node's sessions, sitting between the `argus`
 * wordmark and the session list it describes.
 *
 * Scope is deliberately the current node only: the line sits directly above a
 * list of exactly these sessions, so counting peers here would point at rows
 * nowhere in view. Cross-node unread is the switcher bell's job instead
 * ({@link UnreadBell}). Counts come from the live session data the sidebar
 * already holds rather than the polled node summary, so the line moves the
 * moment a session changes state.
 */
export function SessionSummary({
  sessions,
  sessionStatuses,
  sessionsLoaded,
}: {
  sessions: Pick<Session, "id">[];
  sessionStatuses: Record<string, SessionStatusInfo>;
  sessionsLoaded: boolean;
}) {
  const { active, unread, total } = countSessions(sessions, sessionStatuses);

  // Has any status landed for this node yet? Partial coverage deliberately
  // isn't checked: when a session is newer than the status map, the line counts
  // what it knows and lets the next poll correct it. Session create/clone/delete
  // refresh both queries together (`data/sessions/queries.ts`), so that lag is
  // measured in one round trip rather than a poll interval.
  const anyStatusKnown = Object.keys(sessionStatuses).length > 0;

  // The line always says something. Going blank mid-load reads as breakage
  // rather than as thinking, so each state below is the most specific true
  // statement available at that moment, and staleness is left for the poll to
  // fix. What it won't do is invent an all-clear: with no status data at all,
  // every session would tally as neither active nor unread, so it reports the
  // count it actually has instead of claiming you're caught up.
  //
  // Zero segments are dropped rather than printed as "0 active", so the line
  // stays scannable. Both empty states are real and distinct: nothing to do yet
  // vs. nothing left to do.
  let body;
  if (!sessionsLoaded) {
    body = "Loading…";
  } else if (total === 0) {
    body = "No sessions";
  } else if (!anyStatusKnown) {
    body = `${total} session${total === 1 ? "" : "s"}`;
  } else if (active === 0 && unread === 0) {
    body = "All caught up";
  } else {
    body = (
      <>
        {active > 0 && <Segment count={active} label="active" dot="bg-green-500" />}
        {active > 0 && unread > 0 && <span aria-hidden="true">{" · "}</span>}
        {unread > 0 && <Segment count={unread} label="unread" dot="bg-blue-500" />}
      </>
    );
  }

  return (
    <div
      data-testid="session-summary"
      className="text-muted-foreground flex h-5 items-center text-xs"
    >
      {body}
    </div>
  );
}
