import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { StatusBar, summarizeSessions } from "./StatusBar";
import type { Session, SessionStatusInfo } from "@/types";

const copyToClipboard = vi.fn(async (_text: string) => true);
vi.mock("@/lib/clipboard", () => ({
  copyToClipboard: (text: string) => copyToClipboard(text),
}));
vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

afterEach(cleanup);
beforeEach(() => copyToClipboard.mockClear());

const NOW = "2026-08-17 12:00:00";

function session(overrides: Partial<Session> & { id: string }): Session {
  return {
    name: overrides.id,
    slug: overrides.id,
    tmux_name: `argus-${overrides.id}`,
    created_at: NOW,
    updated_at: NOW,
    working_directory: "/home/jeevb/project",
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

function status(
  overrides: Partial<SessionStatusInfo> & { sessionName: string },
): SessionStatusInfo {
  return { status: "idle", providerType: "claude", ...overrides };
}

describe("summarizeSessions", () => {
  it("counts totals, active, and unread", () => {
    const sessions = [session({ id: "a" }), session({ id: "b" }), session({ id: "c" })];
    const statuses = {
      a: status({ sessionName: "a", status: "active" }),
      b: status({ sessionName: "b", unreadSince: NOW }),
      c: status({ sessionName: "c" }),
    };
    expect(summarizeSessions(sessions, statuses)).toEqual({
      total: 3,
      active: 1,
      unread: 1,
    });
  });

  it("counts an active session as active even when it is flagged unread", () => {
    // Mirrors the server's summary.go rule, so this bar can never disagree with
    // the node rail's attention badge about the same session.
    const statuses = {
      a: status({ sessionName: "a", status: "active", unreadSince: NOW }),
    };
    expect(summarizeSessions([session({ id: "a" })], statuses)).toEqual({
      total: 1,
      active: 1,
      unread: 0,
    });
  });

  it("counts a manually-marked-unread idle session as unread", () => {
    const statuses = { a: status({ sessionName: "a", userMarkedUnreadAt: NOW }) };
    expect(summarizeSessions([session({ id: "a" })], statuses).unread).toBe(1);
  });

  it("counts a dead unread session as unread", () => {
    const statuses = {
      a: status({ sessionName: "a", status: "dead", unreadSince: NOW }),
    };
    expect(summarizeSessions([session({ id: "a" })], statuses).unread).toBe(1);
  });

  it("tolerates a session with no status entry yet", () => {
    // Statuses arrive on their own poll, so a freshly-created session shows up
    // in the list before it has one.
    expect(summarizeSessions([session({ id: "a" })], {})).toEqual({
      total: 1,
      active: 0,
      unread: 0,
    });
  });
});

function renderBar(props: Partial<React.ComponentProps<typeof StatusBar>> = {}) {
  return render(
    <TooltipProvider>
      <StatusBar
        sessions={[]}
        sessionStatuses={{}}
        session={null}
        homeDir="/home/jeevb"
        {...props}
      />
    </TooltipProvider>,
  );
}

const text = (testId: string) => screen.getByTestId(testId).textContent ?? "";

describe("StatusBar detach", () => {
  const testIds = () =>
    [...screen.getByTestId("status-bar").querySelectorAll("[data-testid]")].map(
      (el) => el.getAttribute("data-testid"),
    );

  it("leads the bar", () => {
    renderBar({ onDetach: vi.fn() });
    expect(testIds()[0]).toBe("status-detach");
  });

  it("detaches the active session", () => {
    const onDetach = vi.fn();
    renderBar({ onDetach });
    fireEvent.click(screen.getByTestId("status-detach"));
    expect(onDetach).toHaveBeenCalledTimes(1);
  });

  it("goes inert with nothing attached, keeping its place", () => {
    // Dropping the control would shift every count left the moment a tab
    // detaches; an inert one holds the position it will be clickable in.
    renderBar({ onDetach: undefined });
    expect(screen.getByTestId("status-detach").tagName).not.toBe("BUTTON");
    expect(testIds()[0]).toBe("status-detach");
  });
});

describe("StatusBar counts", () => {
  it("always renders all three counts, zeros included", () => {
    // A count that vanishes at zero leaves you unsure whether it is zero or
    // broken, and shifts the layout every time a session wakes up.
    renderBar();
    expect(text("status-count-total")).toContain("0 sessions");
    expect(text("status-count-active")).toContain("0 active");
    expect(text("status-count-unread")).toContain("0 unread");
  });

  it("renders the derived counts", () => {
    renderBar({
      sessions: [session({ id: "a" }), session({ id: "b" })],
      sessionStatuses: {
        a: status({ sessionName: "a", status: "active" }),
        b: status({ sessionName: "b", unreadSince: NOW }),
      },
    });
    expect(text("status-count-total")).toContain("2 sessions");
    expect(text("status-count-active")).toContain("1 active");
    expect(text("status-count-unread")).toContain("1 unread");
  });

  it("says 1 session, not 1 sessions", () => {
    renderBar({ sessions: [session({ id: "a" })] });
    expect(text("status-count-total")).toContain("1 session");
    expect(text("status-count-total")).not.toContain("1 sessions");
  });

  it("leaves the counts inert", () => {
    // Clicking a count should narrow the list to that count, which needs
    // filters the quick switcher doesn't have. Until it does, the counts are
    // read-only rather than offering a click that lands somewhere else.
    renderBar({ sessions: [session({ id: "a" })] });
    for (const id of [
      "status-count-total",
      "status-count-active",
      "status-count-unread",
    ]) {
      expect(screen.getByTestId(id).tagName).not.toBe("BUTTON");
    }
  });
});

describe("StatusBar active session segments", () => {
  const attached = session({
    id: "sess_m2abc12_xyz789",
    profile: "work",
    worktree_branch: "feat/status-bar",
    working_directory: "/home/jeevb/Workspace/repos/bxnlabs/argus",
  });

  // A worktree session: its working directory sits away from the repo it
  // belongs to, which is the only case where the two paths differ.
  const worktree = session({
    id: "sess_wt",
    working_directory: "/home/jeevb/.argus/projects/x/worktrees/jeev--foo",
    git_parent_dir: "/home/jeevb/Workspace/repos/bxnlabs/argus",
    git_remote_url: "git@github.com:bxnlabs/argus.git",
    worktree_branch: "jeev/foo",
  });

  it("renders nothing about a session when none is attached", () => {
    renderBar({ session: null });
    expect(screen.queryByTestId("status-session-id")).toBeNull();
    expect(screen.queryByTestId("status-branch")).toBeNull();
    expect(screen.queryByTestId("status-dir")).toBeNull();
  });

  it("renders id, profile, branch, and a tilde-contracted directory", () => {
    renderBar({ session: attached });
    expect(text("status-session-id")).toContain("sess_m2abc12_xyz789");
    expect(text("status-profile")).toContain("work");
    expect(text("status-branch")).toContain("feat/status-bar");
    expect(text("status-dir")).toContain("~/");
    expect(text("status-dir")).toContain("argus");
  });

  it("shows the session's own directory, not the repo root", () => {
    // What you see is what a click copies. The repo root is reachable from the
    // repo segment; showing it here would name a directory the session isn't in.
    renderBar({ session: worktree });
    expect(text("status-dir")).toContain("jeev--foo");
    expect(text("status-dir")).not.toContain("Workspace/repos");
  });

  it("copies the worktree path it displays", async () => {
    renderBar({ session: worktree });
    fireEvent.click(screen.getByTestId("status-dir"));
    await waitFor(() =>
      expect(copyToClipboard).toHaveBeenCalledWith(
        "/home/jeevb/.argus/projects/x/worktrees/jeev--foo",
      ),
    );
  });

  it("names the repo when the path no longer does", () => {
    renderBar({ session: worktree });
    expect(text("status-repo")).toContain("bxnlabs/argus");
  });

  it("omits the repo segment when the path already names it", () => {
    // A session sitting in the repo itself needs no second copy of the name —
    // the path is already `~/Workspace/repos/bxnlabs/argus`.
    renderBar({
      session: session({
        id: "a",
        working_directory: "/home/jeevb/Workspace/repos/bxnlabs/argus",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
      }),
    });
    expect(screen.queryByTestId("status-repo")).toBeNull();
  });

  it("omits the repo segment when there is no remote to name it", () => {
    renderBar({
      session: session({
        id: "a",
        working_directory: "/home/jeevb/.argus/projects/x/worktrees/jeev--foo",
        git_parent_dir: "/home/jeevb/Workspace/repos/bxnlabs/argus",
      }),
    });
    expect(screen.queryByTestId("status-repo")).toBeNull();
    expect(text("status-dir")).toContain("jeev--foo");
  });

  it("copies the repo slug", async () => {
    renderBar({ session: worktree });
    fireEvent.click(screen.getByTestId("status-repo"));
    await waitFor(() =>
      expect(copyToClipboard).toHaveBeenCalledWith("bxnlabs/argus"),
    );
  });

  it("omits the profile and branch segments when unset", () => {
    renderBar({ session: session({ id: "a" }) });
    expect(screen.queryByTestId("status-profile")).toBeNull();
    expect(screen.queryByTestId("status-branch")).toBeNull();
    expect(screen.queryByTestId("status-session-id")).toBeTruthy();
  });

  it("truncates a long branch rather than pushing the bar wide", () => {
    renderBar({
      session: session({
        id: "a",
        worktree_branch: "feat/some-really-long-branch-name-that-keeps-going",
      }),
    });
    expect(text("status-branch")).toContain("…");
  });

  it("copies the session id", async () => {
    renderBar({ session: attached });
    fireEvent.click(screen.getByTestId("status-session-id"));
    await waitFor(() =>
      expect(copyToClipboard).toHaveBeenCalledWith("sess_m2abc12_xyz789"),
    );
  });

  it("copies the full absolute directory, not the compressed display", async () => {
    renderBar({ session: attached });
    fireEvent.click(screen.getByTestId("status-dir"));
    await waitFor(() =>
      expect(copyToClipboard).toHaveBeenCalledWith(
        "/home/jeevb/Workspace/repos/bxnlabs/argus",
      ),
    );
  });

  it("opens the git panel from the branch", () => {
    const onOpenGit = vi.fn();
    renderBar({ session: attached, onOpenGit });
    fireEvent.click(screen.getByTestId("status-branch"));
    expect(onOpenGit).toHaveBeenCalledTimes(1);
  });

  it("leaves the branch inert when git is unavailable", () => {
    // The git panel is only reachable when the session sits in a repo; a button
    // that silently does nothing is worse than plain text.
    renderBar({ session: attached, onOpenGit: undefined });
    expect(screen.getByTestId("status-branch").tagName).not.toBe("BUTTON");
  });
});
