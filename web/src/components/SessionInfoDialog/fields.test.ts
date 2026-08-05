import { describe, it, expect } from "vitest";
import { getSessionLocation } from "./fields";
import type { Session } from "@/types";

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "id",
    name: "name",
    slug: "name",
    tmux_name: "claude-id",
    created_at: "2026-05-20 14:32:05",
    updated_at: "2026-05-28 09:15:00",
    working_directory: "/home/u/work",
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

describe("getSessionLocation", () => {
  it("uses working_directory as the directory and omits worktreeDir for plain sessions", () => {
    const loc = getSessionLocation(makeSession(), "/home/u");
    expect(loc.directory.copy).toBe("/home/u/work");
    expect(loc.directory.display).toBe("~/work");
    expect(loc.worktreeDir).toBeNull();
  });

  it("uses git_parent_dir as directory and working_directory as worktreeDir for worktree sessions", () => {
    const loc = getSessionLocation(
      makeSession({
        git_parent_dir: "/home/u/work",
        working_directory: "/home/u/.wt/bxn-104",
        worktree_branch: "jeev/bxn-104",
      }),
      "/home/u",
    );
    expect(loc.directory.copy).toBe("/home/u/work");
    expect(loc.directory.display).toBe("~/work");
    expect(loc.worktreeDir?.copy).toBe("/home/u/.wt/bxn-104");
    expect(loc.worktreeDir?.display).toBe("~/.wt/bxn-104");
    expect(loc.branch).toBe("jeev/bxn-104");
  });

  it("omits worktreeDir when git_parent_dir equals working_directory (plain git repo)", () => {
    const loc = getSessionLocation(
      makeSession({
        git_parent_dir: "/home/u/work",
        working_directory: "/home/u/work",
      }),
      "/home/u",
    );
    expect(loc.directory.copy).toBe("/home/u/work");
    expect(loc.directory.display).toBe("~/work");
    expect(loc.worktreeDir).toBeNull();
  });

  it("omits repo and branch when absent", () => {
    const loc = getSessionLocation(makeSession(), "/home/u");
    expect(loc.repo).toBeNull();
    expect(loc.branch).toBeNull();
  });

  it("includes repo and branch when present", () => {
    const loc = getSessionLocation(
      makeSession({
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "jeev/bxn-97",
      }),
      "/home/u",
    );
    expect(loc.repo).toBe("bxnlabs/argus");
    expect(loc.branch).toBe("jeev/bxn-97");
  });
});
