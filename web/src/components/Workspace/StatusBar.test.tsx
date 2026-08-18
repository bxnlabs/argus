import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@/components/ui/tooltip";
import { StatusBar, summarizeSessions, repoNamedElsewhere } from "./StatusBar";
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

describe("repoNamedElsewhere", () => {
  it("matches a whole path segment", () => {
    expect(repoNamedElsewhere("bxnlabs/argus", "~/repos/bxnlabs/argus", null)).toBe(
      true,
    );
  });

  it("does not match a segment that merely contains the name", () => {
    // The regression this guards: `~/.argus/...` is where every worktree lives,
    // so a substring test would hide the repo cell for every worktree session —
    // the one case where the path can't say which repo it belongs to.
    expect(
      repoNamedElsewhere("bxnlabs/argus", "~/.argus/projects/x/worktrees/a", null),
    ).toBe(false);
  });

  it("counts the branch as naming the repo", () => {
    expect(repoNamedElsewhere("bxnlabs/argus", "~/code/checkout", "argus/main")).toBe(
      true,
    );
  });

  it("does not match inside an elided path", () => {
    // compressPath drops segments; a name the user can't read isn't naming it.
    expect(
      repoNamedElsewhere("bxnlabs/argus", "~/.../worktrees/jeev--foo", null),
    ).toBe(false);
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
const classes = (testId: string) => screen.getByTestId(testId).className;

describe("StatusBar detach", () => {
  // The bar's cells, in visual order. The three layout groups carry testids as
  // stable handles for the layout guards further down, but they are containers
  // rather than cells, so they don't belong in this ordering.
  const GROUPS = new Set([
    "status-counts",
    "status-identity",
    "status-actions",
  ]);
  const testIds = () =>
    [...screen.getByTestId("status-bar").querySelectorAll("[data-testid]")]
      .map((el) => el.getAttribute("data-testid"))
      .filter((id) => id !== null && !GROUPS.has(id));

  it("trails the bar, beside the session it acts on", () => {
    // It used to lead: ~700px from the identity it detaches, and grouped by
    // proximity with fleet counts it has nothing to do with.
    renderBar({ onDetach: vi.fn(), session: session({ id: "a" }) });
    const ids = testIds();
    expect(ids[ids.length - 1]).toBe("status-detach");
  });

  it("detaches the active session", () => {
    const onDetach = vi.fn();
    renderBar({ onDetach });
    fireEvent.click(screen.getByTestId("status-detach"));
    expect(onDetach).toHaveBeenCalledTimes(1);
  });

  it("goes disabled with nothing attached, keeping its place", () => {
    // Dropping the control would reshuffle the end of the bar the moment a tab
    // detaches. A disabled button holds the position it will be clickable in
    // and, unlike a dimmed span, says so to assistive tech.
    renderBar({ onDetach: undefined });
    const detach = screen.getByTestId("status-detach");
    expect(detach.tagName).toBe("BUTTON");
    expect((detach as HTMLButtonElement).disabled).toBe(true);
    const ids = testIds();
    expect(ids[ids.length - 1]).toBe("status-detach");
  });

  it("does not fire when disabled", () => {
    const onDetach = vi.fn();
    renderBar({ onDetach: undefined });
    fireEvent.click(screen.getByTestId("status-detach"));
    expect(onDetach).not.toHaveBeenCalled();
  });
});

describe("StatusBar counts", () => {
  const busy = {
    sessions: [session({ id: "a" }), session({ id: "b" })],
    sessionStatuses: {
      a: status({ sessionName: "a", status: "active" }),
      b: status({ sessionName: "b", unreadSince: NOW }),
    },
  };

  it("renders the derived counts", () => {
    renderBar(busy);
    expect(text("status-count-total")).toContain("2 sessions");
    expect(text("status-count-active")).toContain("1 active");
    expect(text("status-count-unread")).toContain("1 unread");
  });

  it("drops a count at zero but always keeps the total", () => {
    // "0 unread" held 78px of a bar whose identity half was ellipsing four
    // fields at once. The total is the anchor and stays even at zero, so an
    // empty node reads as empty rather than broken.
    renderBar();
    expect(text("status-count-total")).toContain("0 sessions");
    expect(screen.queryByTestId("status-count-active")).toBeNull();
    expect(screen.queryByTestId("status-count-unread")).toBeNull();
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
    renderBar(busy);
    for (const id of [
      "status-count-total",
      "status-count-active",
      "status-count-unread",
    ]) {
      expect(screen.getByTestId(id).tagName).not.toBe("BUTTON");
    }
  });

  it("names the node the counts belong to", () => {
    renderBar({ ...busy, nodeName: "prime" });
    expect(text("status-node")).toContain("prime");
  });

  it("omits the node segment until the nodes load", () => {
    renderBar(busy);
    expect(screen.queryByTestId("status-node")).toBeNull();
  });

  it("speaks the counts as one named phrase, zeros included", () => {
    // Unlabelled, these announce as a run of bare numbers — "7", "sessions",
    // "2", "active" — with nothing saying whose sessions they are. The zeros
    // the bar drops visually stay in the spoken form: hiding them saves pixels,
    // which is not a problem a screen reader has.
    renderBar({ ...busy, nodeName: "prime" });
    const group = screen.getByRole("group", {
      name: "prime: 2 sessions, 1 active, 1 unread",
    });
    expect(group).toBe(screen.getByTestId("status-counts"));
  });

  it("yields width before the identity does", () => {
    // The counts used to be `shrink-0`, so every pixel the bar lost came out of
    // the identity half: measured at 1024px, the counts held 272px of a 680px
    // bar — 40%, one field of it reading "0 unread" — while the branch, the repo
    // and the path were each clipped to about half their text.
    renderBar({ ...busy, session: session({ id: "a" }) });
    expect(classes("status-counts")).not.toContain("shrink-0");
    expect(classes("status-counts")).toContain("min-w-0");
  });

  it("drops its word labels before the bar overflows", () => {
    // Container queries, so this tracks the BAR's width rather than the
    // window's — opening the sidebar takes ~340px out of the bar without
    // touching the viewport. jsdom does no layout, so this asserts the
    // container and the variant that make the collapse possible.
    renderBar({ ...busy, nodeName: "prime" });
    expect(classes("status-bar")).toContain("@container");
    expect(classes("status-node")).toContain("@5xl");
    expect(
      screen
        .getByTestId("status-count-total")
        .querySelector(".tabular-nums span")?.className,
    ).toContain("@5xl");
  });
});

describe("StatusBar active session segments", () => {
  const attached = session({
    id: "sess_m2abc12_xyz789",
    name: "status-bar",
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
    expect(screen.queryByTestId("status-identity")).toBeNull();
    expect(screen.queryByTestId("status-session")).toBeNull();
    expect(screen.queryByTestId("status-session-id")).toBeNull();
    expect(screen.queryByTestId("status-branch")).toBeNull();
    expect(screen.queryByTestId("status-dir")).toBeNull();
  });

  it("leads with the attached session's name", () => {
    // The identity half used to open on the session id — 160px of the string
    // least likely to be read, in the position read first.
    renderBar({ session: attached });
    expect(text("status-session")).toContain("status-bar");
  });

  it("says the attached session's own status, not just the fleet's", () => {
    // The dots on the left count the fleet. Until now nothing on the bar said
    // what the session you are actually attached to is doing.
    renderBar({
      session: attached,
      sessionStatuses: {
        [attached.id]: status({ sessionName: "a", status: "active" }),
      },
    });
    expect(text("status-session")).toContain("Active");
    expect(
      screen.getByTestId("status-session").querySelector("span[aria-hidden]")
        ?.className,
    ).toContain("bg-green-500");
  });

  it("renders profile, branch, and a tilde-contracted directory", () => {
    renderBar({ session: attached });
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

  it("omits the repo segment when the branch already names it", () => {
    renderBar({
      session: session({
        id: "a",
        working_directory: "/home/jeevb/code/checkout",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "argus/main",
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
    expect(screen.queryByTestId("status-session")).toBeTruthy();
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

  it("keeps the session id as an icon that copies it", () => {
    // The id is a copy target, not a readout: it spent 160px at the head of the
    // identity half being the least-read string on the bar.
    renderBar({ session: attached });
    expect(text("status-session-id")).toBe("");
    expect(screen.getByTestId("status-session-id")).toHaveProperty(
      "ariaLabel",
      "Copy session ID",
    );
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

  it("hangs a tooltip on the inert cells too", () => {
    // Every cell here can be ellipsed down to its icon — the repo cell measured
    // 27px of an 81px name at a 680px bar — and the tooltip is then the only
    // place the value survives. Inert cells used to drop theirs on the floor.
    renderBar({ session: attached, onOpenGit: undefined });
    for (const id of ["status-session", "status-branch"]) {
      expect(screen.getByTestId(id)).toHaveProperty("dataset.state", "closed");
    }
  });

  it("leaves the branch inert when git is unavailable", () => {
    // The git panel is only reachable when the session sits in a repo; a button
    // that silently does nothing is worse than plain text.
    renderBar({ session: attached, onOpenGit: undefined });
    expect(screen.getByTestId("status-branch").tagName).not.toBe("BUTTON");
  });

  // jsdom does no layout, so these assert the classes that make truncation
  // possible rather than a measured width. They guard a bug that shipped: a
  // flex child defaults to min-width:auto, so without `min-w-0` the cells stay
  // as wide as their text and the identity half runs off the end of the bar
  // instead of ellipsing — measured at 329px of overflow in a 900px column,
  // leaving 12px of the 339px path cell inside it.
  it("lets the identity cells shrink so their text can truncate", () => {
    renderBar({ session: attached });
    for (const id of ["status-session", "status-branch", "status-dir"]) {
      expect(classes(id)).toContain("min-w-0");
    }
    expect(classes("status-identity")).toContain("min-w-0");
    expect(classes("status-identity")).toContain("overflow-hidden");
  });

  it("ranks the identity cells so the branch is the last to ellipse", () => {
    // Flex shrink is proportional, so equal factors clipped the branch — the
    // field that gets read — at the same rate as the path and the repo. At
    // 1024px that showed 91px of a 185px branch. Lower factor, later ellipsis.
    renderBar({ session: worktree });
    const factor = (id: string) =>
      Number(/shrink-\[(\d+)\]/.exec(classes(id))?.[1]);
    expect(factor("status-branch")).toBeLessThan(factor("status-session"));
    expect(factor("status-session")).toBeLessThan(factor("status-dir"));
    expect(factor("status-dir")).toBeLessThan(factor("status-repo"));
  });

  it("stretches every group to the bar's height", () => {
    // Each cell sizes itself with `h-full`, which resolves against its group,
    // not the bar. A content-height group therefore silently halves the cells:
    // measured at 12px for detach and 14px for identity inside a 24px bar,
    // shrinking the detach hit target and its hover box to match.
    renderBar({ session: attached });
    for (const id of ["status-counts", "status-identity", "status-actions"]) {
      expect(classes(id)).toContain("h-full");
    }
  });

  it("rules off the session half from the fleet half", () => {
    // The boundary used to be a fourth `·`, identical to the separators inside
    // each group: at 1280px the gap across it measured 2px, so seven cells read
    // as one run rather than two groups.
    renderBar({ session: attached });
    const divider = screen.getByTestId("status-identity").parentElement;
    expect(divider?.className).toContain("border-l");
  });
});
