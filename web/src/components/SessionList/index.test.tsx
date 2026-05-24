import { describe, it, expect } from "vitest";
import { partitionSessions, readMenuState } from "./index";
import type { Session } from "@/types";

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "id",
    name: "name",
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
