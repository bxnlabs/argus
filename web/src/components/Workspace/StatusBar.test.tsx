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
    expect(repoNamedElsewhere("bxnlabs/argus", "~/repos/bxnlabs/argus")).toBe(
      true,
    );
  });

  it("does not match a segment that merely contains the name", () => {
    // The regression this guards: `~/.argus/...` is where every worktree lives,
    // so a substring test would hide the repo cell for every worktree session —
    // the one case where the path can't say which repo it belongs to.
    expect(
      repoNamedElsewhere("bxnlabs/argus", "~/.argus/projects/x/worktrees/a"),
    ).toBe(false);
  });

  it("does not match inside an elided path", () => {
    // compressPath drops segments; a name the user can't read isn't naming it.
    expect(
      repoNamedElsewhere("bxnlabs/argus", "~/.../worktrees/jeev--foo"),
    ).toBe(false);
  });
});

function renderBar(props: Partial<React.ComponentProps<typeof StatusBar>> = {}) {
  return render(
    // No delay, so a tooltip opens on the pointer event rather than after a
    // timer the tests would have to fake.
    <TooltipProvider delayDuration={0}>
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

/**
 * Open a cell's tooltip and return its text.
 *
 * Radix opens on `pointermove`, which jsdom dispatches happily — no layout
 * needed, since the tooltip's content is real DOM whether or not anything is
 * positioned. That makes the value recoverable in a test, which matters because
 * the tooltip is the only place a shrunk cell's text survives.
 */
async function tooltipText(testId: string) {
  fireEvent.pointerMove(screen.getByTestId(testId), { pointerType: "mouse" });
  const tip = await screen.findByRole("tooltip");
  return tip.textContent ?? "";
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
});

describe("StatusBar counts", () => {
  const busy = {
    sessions: [session({ id: "a" }), session({ id: "b" })],
    sessionStatuses: {
      a: status({ sessionName: "a", status: "active" }),
      b: status({ sessionName: "b", unreadSince: NOW }),
    },
  };

  it("renders the derived counts as bare numbers", () => {
    // Marks and numbers, no words: the words cost a container query and a
    // threshold that couldn't be derived, to say on a wide monitor what the
    // icons already say. What they said survives in the tooltips and the
    // sr-only line below.
    renderBar(busy);
    expect(text("status-count-total")).toBe("2");
    expect(text("status-count-active")).toBe("1");
    expect(text("status-count-unread")).toBe("1");
  });

  it("drops a count at zero but always keeps the total", () => {
    // "0 unread" held 78px of a bar whose identity half was ellipsing four
    // fields at once. The total is the anchor and stays even at zero, so an
    // empty node reads as empty rather than broken.
    renderBar();
    expect(text("status-count-total")).toBe("0");
    expect(screen.queryByTestId("status-count-active")).toBeNull();
    expect(screen.queryByTestId("status-count-unread")).toBeNull();
  });

  it("says 1 session, not 1 sessions, where it still says it", () => {
    // Only the tooltip and the spoken line carry the noun now, so that is
    // where the plural has to be right.
    renderBar({ sessions: [session({ id: "a" })] });
    expect(text("status-bar")).toContain("1 session,");
    expect(text("status-bar")).not.toContain("1 sessions");
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
    // The counts are node-scoped, so "2 sessions" alone is ambiguous on a bar
    // that shows two nodes' worth over its lifetime — and the bar is the only
    // thing on screen that answers it in the default layout, since the rail
    // that also names the active node renders only with the sidebar open (see
    // DesktopView) and the sidebar starts closed.
    renderBar({ ...busy, nodeName: "prime" });
    expect(text("status-node")).toContain("prime");
  });

  it("omits the node segment until the nodes load", () => {
    renderBar(busy);
    expect(screen.queryByTestId("status-node")).toBeNull();
  });

  it("separates active from unread by shape, not only by hue", () => {
    // Both counts are a mark and a number, and a dropped zero means position
    // says nothing either. Colour alone would leave anyone who can't split
    // green from blue with two identical marks. Shape is the cue that costs no
    // width, which is what carries the distinction now the words are gone.
    renderBar(busy);
    const mark = (id: string) =>
      screen.getByTestId(id).querySelector("span[aria-hidden]")?.className ?? "";
    expect(mark("status-count-active")).toContain("rounded-full");
    expect(mark("status-count-unread")).not.toContain("rounded-full");
    // The attached session's status mark is a circle too: circles are liveness.
    renderBar({ ...busy, session: session({ id: "a" }) });
    expect(mark("status-session")).toContain("rounded-full");
  });

  it("hangs a tooltip on every count, and on the node", async () => {
    // A count is a mark and a number, so the phrase is the only place the full
    // reading survives for a pointer. It backs up the mark's shape rather than
    // carrying the distinction on its own — a tooltip needs a pointer, and
    // these cells take no focus. The node's is what its cap takes off.
    renderBar({ ...busy, nodeName: "prime" });
    expect(await tooltipText("status-node")).toBe("prime");
    cleanup();

    renderBar({ ...busy, nodeName: "prime" });
    expect(await tooltipText("status-count-total")).toBe("2 sessions");
    cleanup();

    renderBar({ ...busy, nodeName: "prime" });
    expect(await tooltipText("status-count-active")).toBe("1 active");
    cleanup();

    renderBar({ ...busy, nodeName: "prime" });
    expect(await tooltipText("status-count-unread")).toBe("1 unread");
  });

  it("speaks the counts as one phrase, zeros included", () => {
    // Unlabelled, these announce as a run of bare numbers — "7", "sessions",
    // "2", "active" — with nothing saying whose sessions they are. The zeros
    // the bar drops visually stay in the spoken form: hiding them saves pixels,
    // which is not a problem a screen reader has.
    renderBar({ ...busy, nodeName: "prime" });
    const spoken = screen
      .getByTestId("status-bar")
      .querySelector(".sr-only")?.textContent;
    expect(spoken).toBe("prime: 2 sessions, 1 active, 1 unread");
  });

  it("keeps the visual counts out of the accessibility tree", () => {
    // Naming the group left its cells in the tree underneath the name, where a
    // reader walking into the group reaches the bare numbers the summary was
    // added to replace. jsdom computes no accessibility tree, so this asserts
    // the markup that produces the outcome, not the announcement itself.
    //
    // Hiding the cluster is safe only while nothing inside it can take focus —
    // aria-hidden over a focusable node strands it for assistive tech.
    renderBar({ ...busy, nodeName: "prime" });
    const counts = screen.getByTestId("status-counts");
    expect(counts.getAttribute("aria-hidden")).toBe("true");
    expect(
      counts.querySelector(
        "button, a[href], input, select, textarea, [tabindex]",
      ),
    ).toBeNull();
  });

  it("is pinned to its content width rather than shrinking", () => {
    // Not a layout assertion — jsdom does no layout — but the class this names
    // is the whole ranking mechanism, so it is worth pinning.
    //
    // The counts were briefly made shrinkable to stop them crowding out the
    // identity. That doesn't rank them: two flex siblings that both shrink give
    // up the same proportion of their width, so measured at a 1184px bar the
    // counts and the identity had both shrunk to 82% of their basis — the
    // counts holding 284px of labels while the repo cell was down to 27px of an
    // 81px name. Ranking by unequal shrink factors is worse: these cells carry
    // no `truncate`, so at a factor of 8 the labels ran into their neighbours
    // and rendered as "7 sessio●2 activ■3 unre". Those labels are gone now, and
    // what is left holds its width at every bar width instead.
    renderBar({ ...busy, session: session({ id: "a" }) });
    expect(classes("status-counts")).toContain("shrink-0");
  });

  it("keeps its width out of the bar's hands, and bounded", () => {
    // Not "one width" — the node name, the digit count and a count crossing
    // zero all change it. The property is that none of those is the BAR's
    // width: nothing in this group answers to a resize, which is why there is
    // no container query here any more and no threshold to keep true.
    //
    // Bounded is the other half. `shrink-0` means nothing in this group gives
    // way, so any uncapped string reintroduced here would take its width
    // straight out of the identity. That is what the cap is for, and why it
    // isn't optional. Guarded as markup, since jsdom does no layout.
    const nodeName = "a-very-long-manually-named-node-indeed";
    renderBar({ ...busy, nodeName });
    expect(classes("status-counts")).toContain("shrink-0");
    expect(classes("status-bar")).not.toContain("@container");
    // Whole value kept, width bounded — the cap trims pixels, not the string.
    const span = screen.getByTestId("status-node").querySelector("span");
    expect(span?.textContent).toBe(nodeName);
    expect(span?.className).toContain("max-w-");
    expect(span?.className).toContain("truncate");
    // And nothing else in here is a string at all.
    expect(text("status-count-total")).toBe("2");
    expect(text("status-count-active")).toBe("1");
    expect(text("status-count-unread")).toBe("1");
  });

  it("keeps the node visible rather than gating it on width again", () => {
    // The cell is the only place the active node's name is plainly legible in
    // the default layout, so it must not go back behind a responsive variant.
    // Presence in the DOM won't catch that: jsdom reports textContent for a
    // `hidden` node just the same, which is exactly how a `hidden @xl:flex`
    // would slip back in unnoticed.
    renderBar({ ...busy, nodeName: "prime" });
    expect(classes("status-node")).not.toContain("hidden");
    expect(classes("status-node")).not.toMatch(/@\w*\[?[\w.]*\]?:/);
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

  it("says so when the session's status hasn't arrived yet", () => {
    // The list and the status poll arrive independently, so a freshly created
    // session is listed before it has a status. Sighted that reads as the dot's
    // muted colour; the dot is aria-hidden, so without a label the cell would
    // announce a bare name and say nothing about the one thing it is for.
    renderBar({ session: attached, sessionStatuses: {} });
    expect(text("status-session")).toContain("Status unknown");
  });

  it("keeps the repo cell when only the branch echoes its name", () => {
    // A branch namespace is arbitrary and doesn't name a repo: `argus/main`
    // identifies neither the owner nor, where two owners share a basename, the
    // project. The path is the only thing that counts as naming it, and a
    // worktree's path points away from the repo — which is exactly when this
    // cell is the only thing on the bar saying which project this is.
    renderBar({
      session: session({
        id: "sess_wt2",
        working_directory: "/home/jeevb/.argus/projects/x/worktrees/jeev--foo",
        git_parent_dir: "/home/jeevb/Workspace/repos/bxnlabs/argus",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "argus/main",
      }),
    });
    expect(text("status-repo")).toContain("bxnlabs/argus");
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

  it("caps a long branch rather than pushing the bar wide", () => {
    // Bounded in CSS, so the cell can't inflate the identity half — uncapped, a
    // 59-character branch left the repo 6% of its text at a 1280px bar — while
    // the branch itself stays whole in the DOM. It used to be cut to 35
    // characters in JS, which dropped the tail at every width and left it only
    // in a tooltip that opens on hover, unreachable when the cell goes inert.
    const branch = "feat/some-really-long-branch-name-that-keeps-going";
    renderBar({ session: session({ id: "a", worktree_branch: branch }) });
    const span = screen
      .getByTestId("status-branch")
      .querySelector(".truncate");
    expect(span?.textContent).toBe(branch);
    expect(span?.className).toContain("max-w-");
    expect(span?.className).toContain("truncate");
  });

  it("keeps the whole branch readable when git is unavailable", () => {
    // The cell is an inert span here, and its tooltip needs a pointer. If the
    // value were cut to fit, the tail would exist nowhere else — so it isn't.
    const branch = "feat/some-really-long-branch-name-that-keeps-going";
    renderBar({
      session: session({ id: "a", worktree_branch: branch }),
      onOpenGit: undefined,
    });
    expect(screen.getByTestId("status-branch").tagName).not.toBe("BUTTON");
    expect(text("status-branch")).toContain(branch);
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

  it("opens a tooltip on the inert cells too", async () => {
    // Every cell here can be ellipsed down to its icon — the repo cell measured
    // 27px of an 81px name at a 680px bar, and the profile 14 of 176px — and
    // the tooltip is then the only place the value survives. Inert cells used
    // to drop theirs on the floor: StatusItem returned the span before it
    // reached the wrapper, so the value was recoverable nowhere.
    renderBar({ session: attached, onOpenGit: undefined });
    expect(screen.getByTestId("status-branch").tagName).not.toBe("BUTTON");
    expect(await tooltipText("status-branch")).toBe("feat/status-bar");
    cleanup();

    renderBar({ session: attached, onOpenGit: undefined });
    expect(await tooltipText("status-profile")).toBe("work");
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

  it("orders the identity cells' shrink factors, branch lowest", () => {
    // Reads the factors off the classes; the ellipsing itself needs a layout.
    // Flex shrink is proportional, so equal factors clipped the branch — the
    // field that gets read — at the same rate as the path and the repo. At
    // 1024px that showed 91px of a 185px branch. Lower factor, later ellipsis:
    // measured at a 560px bar the branch still held 135 of 170px while the repo
    // and the path were down to nothing.
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
