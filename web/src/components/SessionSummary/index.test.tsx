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
      />,
    );
    expect(screen.getByTestId("session-summary").textContent).toBe("2 active · 1 unread");
  });

  it("drops a segment that would read zero", () => {
    render(
      <SessionSummary
        sessions={[{ id: "a" }]}
        sessionStatuses={{ a: status({ unreadSince: "2026-01-01 00:00:00" }) }}
      />,
    );
    expect(screen.getByTestId("session-summary").textContent).toBe("1 unread");
  });

  it("says you're caught up when sessions exist but none want you", () => {
    render(
      <SessionSummary sessions={[{ id: "a" }]} sessionStatuses={{ a: status() }} />,
    );
    expect(screen.getByTestId("session-summary").textContent).toBe("All caught up");
  });

  it("says there are no sessions rather than claiming you're caught up", () => {
    render(<SessionSummary sessions={[]} sessionStatuses={{}} />);
    expect(screen.getByTestId("session-summary").textContent).toBe("No sessions");
  });

  it("colors the dots to match the session rows they summarize", () => {
    const { container } = render(
      <SessionSummary
        sessions={[{ id: "a" }, { id: "b" }]}
        sessionStatuses={{
          a: status({ status: "active" }),
          b: status({ unreadSince: "2026-01-01 00:00:00" }),
        }}
      />,
    );
    // Same tokens the rows use: green for a running session (getStatusMeta),
    // blue for unread (resolveStatusDisplay).
    expect(container.querySelector(".bg-green-500")).not.toBeNull();
    expect(container.querySelector(".bg-blue-500")).not.toBeNull();
  });
});
