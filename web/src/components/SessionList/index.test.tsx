import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import {
  partitionSessions,
  readMenuState,
  resolveStatusDisplay,
  resolveRowDisplay,
  SessionListSkeleton,
} from "./index";
import type { Session } from "@/types";

afterEach(cleanup);

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "id",
    name: "name",
    slug: "name",
    tmux_name: "claude-id",
    created_at: "2026-01-01 00:00:00",
    updated_at: "2026-01-01 00:00:00",
    working_directory: "~",
    worktree_branch: null,
    git_parent_dir: null,
    git_remote_url: null,
    provider_session_id: null,
    model: null,
    system_prompt: null,
    provider_type: "claude",
    auto_approve: false,
    profile: null,
    pinned: false,
    ...overrides,
  };
}

describe("partitionSessions", () => {
  it("splits pinned from the rest", () => {
    const a = makeSession({ id: "a", pinned: false });
    const b = makeSession({ id: "b", pinned: true });
    const { pinned, rest } = partitionSessions([a, b]);
    expect(pinned.map((s) => s.id)).toEqual(["b"]);
    expect(rest.map((s) => s.id)).toEqual(["a"]);
  });

  it("orders each group by updated_at descending", () => {
    const pOld = makeSession({ id: "p-old", pinned: true, updated_at: "2026-01-01 00:00:00" });
    const pNew = makeSession({ id: "p-new", pinned: true, updated_at: "2026-01-04 00:00:00" });
    const rOld = makeSession({ id: "r-old", updated_at: "2026-01-02 00:00:00" });
    const rNew = makeSession({ id: "r-new", updated_at: "2026-01-09 00:00:00" });
    const { pinned, rest } = partitionSessions([pOld, rOld, pNew, rNew]);
    expect(pinned.map((s) => s.id)).toEqual(["p-new", "p-old"]);
    expect(rest.map((s) => s.id)).toEqual(["r-new", "r-old"]);
  });

  it("does not mutate the input array", () => {
    const input = [makeSession({ id: "a" }), makeSession({ id: "b", pinned: true })];
    const copy = [...input];
    partitionSessions(input);
    expect(input).toEqual(copy);
  });
});

describe("readMenuState", () => {
  it("shows 'Mark as read' when either unread signal is set", () => {
    expect(readMenuState("2026-01-01 00:00:00", null).showMarkRead).toBe(true);
    expect(readMenuState(null, "2026-01-01 00:00:00").showMarkRead).toBe(true);
    expect(readMenuState(null, null).showMarkRead).toBe(false);
    expect(readMenuState(undefined, undefined).showMarkRead).toBe(false);
  });

  it("shows 'Mark as unread' only when neither signal is set", () => {
    expect(readMenuState(null, null).showMarkUnread).toBe(true);
    expect(readMenuState("2026-01-01 00:00:00", null).showMarkUnread).toBe(false);
    expect(readMenuState(null, "2026-01-01 00:00:00").showMarkUnread).toBe(false);
  });
});

describe("resolveStatusDisplay", () => {
  it("shows the live Active status even when the session is marked unread", () => {
    // A running session flagged unread must still surface as Active — the unread
    // marker persists in the data but is masked while activity is live.
    const display = resolveStatusDisplay("active", null, "2026-01-01 00:00:00");
    expect(display.label).toBe("Active");
    expect(display.dotColor).toBe("bg-green-500");
    expect(display.animation).toBe("animate-pulse-green");
  });

  it("masks the automatic unread_since while active too", () => {
    const display = resolveStatusDisplay("active", "2026-01-01 00:00:00", null);
    expect(display.label).toBe("Active");
    expect(display.dotColor).toBe("bg-green-500");
  });

  it("shows Unread when idle and marked unread", () => {
    const display = resolveStatusDisplay("idle", null, "2026-01-01 00:00:00");
    expect(display.label).toBe("Unread");
    expect(display.dotColor).toBe("bg-blue-500");
    expect(display.animation).toBe("");
  });

  it("shows Unread when dead and marked unread", () => {
    const display = resolveStatusDisplay("dead", "2026-01-01 00:00:00", null);
    expect(display.label).toBe("Unread");
    expect(display.dotColor).toBe("bg-blue-500");
  });

  it("falls back to the plain status meta when not unread", () => {
    expect(resolveStatusDisplay("idle", null, null).label).toBe("Idle");
    expect(resolveStatusDisplay("active", null, null).dotColor).toBe("bg-green-500");
  });
});

describe("resolveRowDisplay", () => {
  it("shows a spinner and the busy label for each busy kind", () => {
    expect(resolveRowDisplay("deleting", "idle", null, null)).toEqual({
      label: "Deleting…",
      dotColor: "",
      animation: "",
      spinner: true,
    });
    expect(resolveRowDisplay("cloning", "idle", null, null).label).toBe("Cloning…");
    expect(resolveRowDisplay("profile", "idle", null, null).label).toBe(
      "Updating profile…",
    );
  });

  // Busy is the most urgent thing happening to the row, so it outranks both
  // the live status and the unread marker.
  it("takes precedence over an active session", () => {
    const active = resolveRowDisplay(undefined, "active", null, null);
    const busy = resolveRowDisplay("deleting", "active", null, null);
    expect(active.spinner).toBe(false);
    expect(busy.spinner).toBe(true);
    expect(busy.label).toBe("Deleting…");
  });

  it("takes precedence over an unread session", () => {
    expect(resolveRowDisplay(undefined, "idle", "2026-01-01", null).label).toBe(
      "Unread",
    );
    expect(resolveRowDisplay("deleting", "idle", "2026-01-01", null).label).toBe(
      "Deleting…",
    );
  });

  it("matches resolveStatusDisplay when not busy", () => {
    for (const status of ["active", "idle", "waiting", undefined]) {
      expect(resolveRowDisplay(undefined, status, null, null)).toEqual({
        ...resolveStatusDisplay(status, null, null),
        spinner: false,
      });
    }
  });
});

describe("SessionListSkeleton", () => {
  it("renders pulsing placeholder rows, announced as loading", () => {
    const { container } = render(<SessionListSkeleton />);
    const root = screen.getByTestId("session-list-skeleton");
    expect(root.getAttribute("role")).toBe("status");
    expect(root.getAttribute("aria-label")).toBe("Loading sessions");
    // Every bar pulses — a static grey block reads as broken layout, not as work
    // in progress.
    const bars = container.querySelectorAll(".bg-muted");
    expect(bars.length).toBeGreaterThan(0);
    for (const bar of bars) {
      expect(bar.className).toContain("animate-pulse");
    }
  });
});
