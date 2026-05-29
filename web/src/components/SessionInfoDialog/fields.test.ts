import { describe, it, expect } from "vitest";
import { buildSessionInfoModel } from "./fields";
import type { Session } from "@/types";

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "id",
    name: "name",
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

describe("buildSessionInfoModel", () => {
  it("uses working_directory as the directory and omits worktreeDir for plain sessions", () => {
    const m = buildSessionInfoModel(makeSession(), "idle", "/home/u");
    expect(m.location.directory.copy).toBe("/home/u/work");
    expect(m.location.directory.display).toBe("~/work");
    expect(m.location.worktreeDir).toBeNull();
  });

  it("uses git_parent_dir as directory and working_directory as worktreeDir for worktree sessions", () => {
    const m = buildSessionInfoModel(
      makeSession({
        git_parent_dir: "/home/u/work",
        working_directory: "/home/u/.wt/bxn-104",
        worktree_branch: "jeev/bxn-104",
      }),
      "active",
      "/home/u",
    );
    expect(m.location.directory.copy).toBe("/home/u/work");
    expect(m.location.directory.display).toBe("~/work");
    expect(m.location.worktreeDir?.copy).toBe("/home/u/.wt/bxn-104");
    expect(m.location.worktreeDir?.display).toBe("~/.wt/bxn-104");
    expect(m.location.branch).toBe("jeev/bxn-104");
  });

  it("omits repo, branch, and model when absent and passes status/profile through", () => {
    const m = buildSessionInfoModel(makeSession(), undefined, "/home/u");
    expect(m.location.repo).toBeNull();
    expect(m.location.branch).toBeNull();
    expect(m.details.model).toBeNull();
    expect(m.status).toBeUndefined();
    expect(m.profile).toBeNull();
  });

  it("includes model, repo, branch, profile, and pinned when present", () => {
    const m = buildSessionInfoModel(
      makeSession({
        model: "claude-opus-4-7",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "jeev/bxn-97",
        profile: "default",
        pinned: true,
      }),
      "idle",
      "/home/u",
    );
    expect(m.details.model).toBe("claude-opus-4-7");
    expect(m.location.repo).toBe("bxnlabs/argus");
    expect(m.location.branch).toBe("jeev/bxn-97");
    expect(m.profile).toBe("default");
    expect(m.pinned).toBe(true);
  });

  it("exposes absolute timestamps and a relative updated time", () => {
    const m = buildSessionInfoModel(makeSession(), "idle", "/home/u");
    expect(m.createdAbsolute).toBe("2026-05-20 14:32:05");
    expect(m.updatedAbsolute).toBe("2026-05-28 09:15:00");
    expect(typeof m.updatedRelative).toBe("string");
  });
});
