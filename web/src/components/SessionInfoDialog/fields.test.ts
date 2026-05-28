import { describe, it, expect } from "vitest";
import { buildSessionInfoSections } from "./fields";
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

function findRow(sections: ReturnType<typeof buildSessionInfoSections>, label: string) {
  for (const s of sections) {
    const row = s.rows.find((r) => r.label === label);
    if (row) return row;
  }
  return undefined;
}

describe("buildSessionInfoSections", () => {
  it("renders header, provider, location, and timestamps sections", () => {
    const sections = buildSessionInfoSections(makeSession(), "idle", "/home/u");
    expect(sections.map((s) => s.title)).toEqual([null, "Provider", "Location", "Timestamps"]);
  });

  it("maps status, pinned, profile, and auto-approve to friendly values", () => {
    const sections = buildSessionInfoSections(
      makeSession({ pinned: true, profile: "default", auto_approve: true }),
      "active",
      "/home/u",
    );
    expect(findRow(sections, "Status")?.value).toBe("Active");
    expect(findRow(sections, "Pinned")?.value).toBe("Yes");
    expect(findRow(sections, "Profile")?.value).toBe("default");
    expect(findRow(sections, "Auto-approve")?.value).toBe("On");
  });

  it("falls back when optional fields are absent", () => {
    const sections = buildSessionInfoSections(makeSession(), undefined, "/home/u");
    expect(findRow(sections, "Status")?.value).toBe("Unknown");
    expect(findRow(sections, "Profile")?.value).toBe("None");
    expect(findRow(sections, "Model")).toBeUndefined();
    expect(findRow(sections, "Branch")).toBeUndefined();
    expect(findRow(sections, "Repo")).toBeUndefined();
  });

  it("includes model, repo, and branch when present", () => {
    const sections = buildSessionInfoSections(
      makeSession({
        model: "claude-opus-4-7",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "jeev/bxn-97",
      }),
      "idle",
      "/home/u",
    );
    expect(findRow(sections, "Model")?.value).toBe("claude-opus-4-7");
    expect(findRow(sections, "Repo")?.value).toBe("bxnlabs/argus");
    expect(findRow(sections, "Branch")?.value).toBe("jeev/bxn-97");
  });

  it("shows absolute timestamps", () => {
    const sections = buildSessionInfoSections(makeSession(), "idle", "/home/u");
    expect(findRow(sections, "Created")?.value).toContain("2026-05-20 14:32:05");
    expect(findRow(sections, "Updated")?.value).toContain("2026-05-28 09:15:00");
  });
});
