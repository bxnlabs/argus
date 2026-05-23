import { describe, it, expect } from "vitest";
import { sortSessions } from "./index";
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
    flagged: false,
    starred: false,
    ...overrides,
  };
}

describe("sortSessions", () => {
  it("places starred sessions before unstarred ones", () => {
    const a = makeSession({ id: "a", starred: false, updated_at: "2026-01-03 00:00:00" });
    const b = makeSession({ id: "b", starred: true, updated_at: "2026-01-01 00:00:00" });
    const sorted = sortSessions([a, b]);
    expect(sorted.map((s) => s.id)).toEqual(["b", "a"]);
  });

  it("orders by updated_at descending within the same starred group", () => {
    const older = makeSession({ id: "older", updated_at: "2026-01-01 00:00:00" });
    const newer = makeSession({ id: "newer", updated_at: "2026-01-05 00:00:00" });
    const sorted = sortSessions([older, newer]);
    expect(sorted.map((s) => s.id)).toEqual(["newer", "older"]);
  });

  it("orders starred group by updated_at, then unstarred group by updated_at", () => {
    const starredOld = makeSession({ id: "s-old", starred: true, updated_at: "2026-01-01 00:00:00" });
    const starredNew = makeSession({ id: "s-new", starred: true, updated_at: "2026-01-04 00:00:00" });
    const plainOld = makeSession({ id: "p-old", starred: false, updated_at: "2026-01-02 00:00:00" });
    const plainNew = makeSession({ id: "p-new", starred: false, updated_at: "2026-01-09 00:00:00" });
    const sorted = sortSessions([starredOld, plainOld, starredNew, plainNew]);
    expect(sorted.map((s) => s.id)).toEqual(["s-new", "s-old", "p-new", "p-old"]);
  });

  it("does not mutate the input array", () => {
    const input = [makeSession({ id: "a" }), makeSession({ id: "b", starred: true })];
    const copy = [...input];
    sortSessions(input);
    expect(input).toEqual(copy);
  });
});
