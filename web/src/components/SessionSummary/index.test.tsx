import type { ComponentProps } from "react";
import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { SessionSummary, countSessions } from "./index";
import type { SessionStatusInfo } from "@/types";

afterEach(cleanup);

const status = (over: Partial<SessionStatusInfo> = {}): SessionStatusInfo => ({
  sessionName: "s",
  status: "idle",
  providerType: "claude",
  unreadSince: null,
  userMarkedUnreadAt: null,
  ...over,
});

describe("countSessions", () => {
  it("counts running sessions as active", () => {
    const counts = countSessions([{ id: "a" }, { id: "b" }], {
      a: status({ status: "active" }),
      b: status({ status: "idle" }),
    });
    expect(counts).toEqual({ active: 1, unread: 0, total: 2 });
  });

  it("counts an idle session flagged unread", () => {
    const counts = countSessions([{ id: "a" }], {
      a: status({ unreadSince: "2026-01-01 00:00:00" }),
    });
    expect(counts).toEqual({ active: 0, unread: 1, total: 1 });
  });

  it("counts a manually-marked session as unread", () => {
    const counts = countSessions([{ id: "a" }], {
      a: status({ userMarkedUnreadAt: "2026-01-01 00:00:00" }),
    });
    expect(counts).toEqual({ active: 0, unread: 1, total: 1 });
  });

  it("lets active outrank unread, so no session is counted twice", () => {
    // Same precedence the node summary endpoint and the row's status dot use:
    // a running session reads as active even while its unread flag persists.
    const counts = countSessions([{ id: "a" }], {
      a: status({ status: "active", unreadSince: "2026-01-01 00:00:00" }),
    });
    expect(counts).toEqual({ active: 1, unread: 0, total: 1 });
  });

  it("treats a session with no status entry as neither active nor unread", () => {
    // Counting only tallies what it can see. Deciding whether an all-zero tally
    // means "caught up" or "not known yet" is SessionSummary's job, below.
    const counts = countSessions([{ id: "a" }], {});
    expect(counts).toEqual({ active: 0, unread: 0, total: 1 });
  });
});

describe("SessionSummary", () => {
  it("reports both counts for the sessions listed below it", () => {
    render(
      <SessionSummary
        sessions={[{ id: "a" }, { id: "b" }, { id: "c" }]}
        sessionStatuses={{
          a: status({ status: "active" }),
          b: status({ status: "active" }),
          c: status({ unreadSince: "2026-01-01 00:00:00" }),
        }}
        sessionsLoaded
      />,
    );
    // No spaces around the separator: it's a flex item, so any it carried would
    // be trimmed away. The gap asserted below is what actually spaces the line.
    expect(screen.getByTestId("session-summary").textContent).toBe("2 active·1 unread");
  });

  it("spaces the segments with a gap rather than whitespace flex would trim", () => {
    // jsdom has no layout, so this can't be caught by reading the text: a " · "
    // separator reports its spaces in textContent while rendering as a bare
    // glyph wedged between the two segments. Assert the mechanism instead.
    render(
      <SessionSummary
        sessions={[{ id: "a" }, { id: "b" }]}
        sessionStatuses={{
          a: status({ status: "active" }),
          b: status({ unreadSince: "2026-01-01 00:00:00" }),
        }}
        sessionsLoaded
      />,
    );
    const line = screen.getByTestId("session-summary");
    expect(line.className).toMatch(/\bgap-\d/);
    expect(line.textContent).not.toMatch(/\s·|·\s/);
  });

  it("drops a segment that would read zero", () => {
    render(
      <SessionSummary
        sessions={[{ id: "a" }]}
        sessionStatuses={{ a: status({ unreadSince: "2026-01-01 00:00:00" }) }}
        sessionsLoaded
      />,
    );
    expect(screen.getByTestId("session-summary").textContent).toBe("1 unread");
  });

  it("says you're caught up when sessions exist but none want you", () => {
    render(
      <SessionSummary
        sessions={[{ id: "a" }]}
        sessionStatuses={{ a: status() }}
        sessionsLoaded
      />,
    );
    expect(screen.getByTestId("session-summary").textContent).toBe("All caught up");
  });

  it("says there are no sessions rather than claiming you're caught up", () => {
    render(<SessionSummary sessions={[]} sessionStatuses={{}} sessionsLoaded />);
    expect(screen.getByTestId("session-summary").textContent).toBe("No sessions");
  });

  it("reports the count it has rather than an all-clear it hasn't earned", () => {
    // The window between the sessions fetch landing and the status fetch (which
    // is only enabled once sessions exist) settling. Every session tallies as
    // neither active nor unread here, which is the absence of an answer — so the
    // line states what it does know instead of saying "All caught up".
    render(<SessionSummary sessions={[{ id: "a" }]} sessionStatuses={{}} sessionsLoaded />);
    expect(screen.getByTestId("session-summary").textContent).toBe("1 session");
  });

  it("counts what it knows when the status map lags the session list", () => {
    // A session newer than the status map. Rather than withhold the line, it
    // counts the covered sessions and lets the next poll correct it — which the
    // create/clone/delete status invalidation keeps to about a round trip.
    render(
      <SessionSummary
        sessions={[{ id: "a" }, { id: "b" }]}
        sessionStatuses={{ a: status({ status: "active" }) }}
        sessionsLoaded
      />,
    );
    expect(screen.getByTestId("session-summary").textContent).toBe("1 active");
  });

  it("stays silent rather than claiming there are no sessions", () => {
    // Not "Loading…": the placeholder rows below already say that, and a node
    // that never answers would sit on the word forever. The node's presence dot
    // is what distinguishes "still connecting" from "gave up".
    render(<SessionSummary sessions={[]} sessionStatuses={{}} sessionsLoaded={false} />);
    expect(screen.getByTestId("session-summary").textContent).toBe("");
  });

  it("keeps its slot open while it has nothing to say", () => {
    // The height is what stops the header from jumping when the line finally
    // speaks, so silence has to be an empty line rather than no line.
    render(<SessionSummary sessions={[]} sessionStatuses={{}} sessionsLoaded={false} />);
    expect(screen.getByTestId("session-summary").className).toMatch(/\bh-\d/);
  });

  it("never renders an empty line once the list has landed", () => {
    // The line holds a fixed-height slot in the sidebar header; going blank
    // with an answer in hand reads as breakage rather than as work in progress.
    const states: ComponentProps<typeof SessionSummary>[] = [
      { sessions: [], sessionStatuses: {}, sessionsLoaded: true },
      { sessions: [{ id: "a" }], sessionStatuses: {}, sessionsLoaded: true },
      { sessions: [{ id: "a" }], sessionStatuses: { a: status() }, sessionsLoaded: true },
      {
        sessions: [{ id: "a" }, { id: "b" }],
        sessionStatuses: { a: status({ status: "active" }) },
        sessionsLoaded: true,
      },
    ];
    for (const props of states) {
      const { unmount } = render(<SessionSummary {...props} />);
      expect(screen.getByTestId("session-summary").textContent).not.toBe("");
      unmount();
    }
  });

  it("colors the dots to match the session rows they summarize", () => {
    const { container } = render(
      <SessionSummary
        sessions={[{ id: "a" }, { id: "b" }]}
        sessionStatuses={{
          a: status({ status: "active" }),
          b: status({ unreadSince: "2026-01-01 00:00:00" }),
        }}
        sessionsLoaded
      />,
    );
    // Same tokens the rows use: green for a running session (getStatusMeta),
    // blue for unread (resolveStatusDisplay).
    expect(container.querySelector(".bg-green-500")).not.toBeNull();
    expect(container.querySelector(".bg-blue-500")).not.toBeNull();
  });
});
