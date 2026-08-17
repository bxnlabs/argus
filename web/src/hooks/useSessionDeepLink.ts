import { useEffect, useState } from "react";
import type { Session } from "@/types";

const DEEP_LINK_EVENT = "argus:session-deep-link";

/**
 * Announce that `?session=` has just been written programmatically.
 *
 * `history.replaceState` does not re-render React and does not fire `popstate`,
 * so a hook that only samples the URL during render would not see the write
 * until something unrelated happened to re-render. That is the case when the
 * writer is a *dead* workspace closure handing work to a live one on the same
 * node: `setActiveNode` no-ops, nothing re-renders, and the attach waits on
 * whatever incidental dependency churns next. This makes the wake-up explicit.
 */
export function notifySessionDeepLink(): void {
  window.dispatchEvent(new Event(DEEP_LINK_EVENT));
}

/**
 * Consumes the `?session=` deep link: attaches the session it names, once that
 * session actually exists in the list, and only then clears the param.
 *
 * The param is the one-shot token — there is no "already handled" flag — and it
 * survives a miss on purpose. A request routinely arrives before the list
 * catches up: `sessionsLoaded` only means the list query holds data, which is
 * already true for a cached list with a refetch in flight, so a create or clone
 * handed over from another node lands while that list still predates it.
 * Consuming the param on that first miss would drop the request for good;
 * leaving it lets the render that finally sees the session finish the job.
 *
 * A link naming a session that never appears simply leaves the param in place,
 * which costs a `find` per session update and nothing else.
 */
export function useSessionDeepLink({
  sessionsLoaded,
  sessions,
  onAttach,
}: {
  sessionsLoaded: boolean;
  sessions: Session[];
  onAttach: (session: Session) => void;
}): void {
  // Re-samples the URL on an explicit notify. Not folded into the effect below
  // as a listener that attaches directly: the param may name a session that is
  // not in the list yet, so the retry has to go through a render yielding the
  // current `sessions`, not through the event's own closure.
  const [notified, setNotified] = useState(0);
  useEffect(() => {
    const wake = () => setNotified((n) => n + 1);
    window.addEventListener(DEEP_LINK_EVENT, wake);
    return () => window.removeEventListener(DEEP_LINK_EVENT, wake);
  }, []);

  useEffect(() => {
    if (!sessionsLoaded) return;

    const sessionId = new URLSearchParams(window.location.search).get(
      "session",
    );
    if (!sessionId) return;

    const session = sessions.find((s) => s.id === sessionId);
    if (!session) return;

    onAttach(session);

    // Cleared only now, so this cannot re-trigger on a later render or on
    // refresh — and so a second deep link to the same session still lands.
    const url = new URL(window.location.href);
    url.searchParams.delete("session");
    window.history.replaceState({}, "", url.pathname + url.search + url.hash);
  }, [sessionsLoaded, sessions, onAttach, notified]);
}
