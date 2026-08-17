import { describe, it, expect, afterEach, beforeAll, vi } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { QuickSwitcher } from "./QuickSwitcher";
import type { Session } from "@/types";

afterEach(cleanup);

beforeAll(() => {
  if (!("ResizeObserver" in globalThis)) {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

function makeSession(id: string, name: string): Session {
  return {
    id,
    name,
    slug: name,
    working_directory: "/home/u/repo",
    provider_type: "claude",
  } as Session;
}

const sessions = [makeSession("a", "first"), makeSession("b", "second")];

function rowNames() {
  return screen
    .getAllByRole("button")
    .map((b) => b.querySelector(".truncate.font-medium")?.textContent)
    .filter(Boolean);
}

function renderSwitcher(currentSessionId?: string) {
  return render(
    <QuickSwitcher
      sessions={sessions}
      homeDir="/home/u"
      open
      onOpenChange={() => {}}
      onSelectSession={() => {}}
      currentSessionId={currentSessionId}
    />,
  );
}

// The row a session sits on and the row the arrow keys sit on are different
// facts, and they used to be drawn with backgrounds that collided: `cn` runs
// tailwind-merge, so the current row's `bg-primary/10` replaced the cursor's
// `bg-accent` outright rather than layering with it. The cursor then had no
// visible effect on exactly one row — the one you're most likely to start on.
describe("QuickSwitcher current-session marker", () => {
  it("marks the current session in words, not with a background", () => {
    renderSwitcher("a");

    const row = screen.getByText("first").closest("button");
    expect(row).not.toBeNull();
    expect(row!.className).not.toMatch(/bg-primary/);
    expect(screen.getByText("Current")).toBeTruthy();
  });

  it("puts the session you're on at the top, whatever the list order says", () => {
    // `sessions` arrives in the sidebar's recency order, so the current one can
    // be anywhere in it. Its row is the only one whose position you can know
    // before looking.
    renderSwitcher("b");
    expect(rowNames()).toEqual(["second", "first"]);
  });

  it("leaves the order alone when the current session is already first", () => {
    renderSwitcher("a");
    expect(rowNames()).toEqual(["first", "second"]);
  });

  it("doesn't resurrect the current session when a search excludes it", () => {
    // Hoisting applies among the matches. A search the current session fails is
    // still a search it fails.
    renderSwitcher("b");
    fireEvent.change(screen.getByPlaceholderText("Search sessions..."), {
      target: { value: "first" },
    });
    expect(rowNames()).toEqual(["first"]);
  });

  it("opens the cursor past the session you're on, not on it", () => {
    // The top row is where you already are, so landing there would make Enter
    // mean "stay put" — the one thing nobody opens a switcher to do.
    renderSwitcher("b");

    const rows = screen.getAllByRole("button");
    const current = screen.getByText("second").closest("button")!;
    const next = screen.getByText("first").closest("button")!;

    expect(rows.indexOf(current)).toBeLessThan(rows.indexOf(next));
    expect(current.className.split(/\s+/)).not.toContain("bg-accent");
    expect(next.className.split(/\s+/)).toContain("bg-accent");
  });

  it("selects the next session on Enter without touching the arrows", () => {
    const onSelectSession = vi.fn();
    render(
      <QuickSwitcher
        sessions={sessions}
        homeDir="/home/u"
        open
        onOpenChange={() => {}}
        onSelectSession={onSelectSession}
        currentSessionId="b"
      />,
    );

    fireEvent.keyDown(screen.getByPlaceholderText("Search sessions..."), {
      key: "Enter",
    });

    expect(onSelectSession).toHaveBeenCalledWith("a");
  });

  it("still has a selected row when the current session is the only one", () => {
    // Nothing to skip to, so the cursor stays put rather than pointing past the
    // end of the list.
    render(
      <QuickSwitcher
        sessions={[makeSession("a", "only")]}
        homeDir="/home/u"
        open
        onOpenChange={() => {}}
        onSelectSession={() => {}}
        currentSessionId="a"
      />,
    );

    expect(
      screen.getByText("only").closest("button")!.className.split(/\s+/),
    ).toContain("bg-accent");
  });

  it("lands the cursor once the list arrives after the dialog opened", () => {
    // Cmd+K is bound with no gate on the list having loaded (App), and the
    // dialog stays mounted, so opening it during the first fetch — or right
    // after a node switch, which empties the cache for the new scope — places
    // the cursor against an empty list.
    //
    // Reading the landing spot out of a ref made that permanent: the ref
    // updated when the sessions arrived, but nothing depended on it, so no
    // effect re-ran. The cursor stayed on row 0, the current session was
    // hoisted underneath it, and Enter went back to where you already were.
    const onSelectSession = vi.fn();
    const props = {
      homeDir: "/home/u",
      open: true,
      onOpenChange: () => {},
      onSelectSession,
      currentSessionId: "b",
    };
    const { rerender } = render(<QuickSwitcher sessions={[]} {...props} />);
    rerender(<QuickSwitcher sessions={sessions} {...props} />);

    const rowFor = (name: string) =>
      screen.getByText(name).closest("button")!.className.split(/\s+/);

    expect(rowNames()).toEqual(["second", "first"]);
    expect(rowFor("second")).not.toContain("bg-accent");
    expect(rowFor("first")).toContain("bg-accent");

    fireEvent.keyDown(screen.getByPlaceholderText("Search sessions..."), {
      key: "Enter",
    });
    expect(onSelectSession).toHaveBeenCalledWith("a");
  });

  it("keeps a usable cursor when the list shrinks under it", () => {
    // Sessions can leave the list while the dialog is open — deleted from
    // another client, or on the node itself. The cursor now commonly starts at
    // row 1, so a list dropping to a single row leaves it pointing past the
    // end: nothing takes the wash, and Enter selects nothing at all.
    const onSelectSession = vi.fn();
    const props = {
      homeDir: "/home/u",
      open: true,
      onOpenChange: () => {},
      onSelectSession,
      currentSessionId: "b",
    };
    const { rerender } = render(<QuickSwitcher sessions={sessions} {...props} />);
    rerender(
      <QuickSwitcher sessions={[makeSession("b", "second")]} {...props} />,
    );

    expect(
      screen.getByText("second").closest("button")!.className.split(/\s+/),
    ).toContain("bg-accent");

    fireEvent.keyDown(screen.getByPlaceholderText("Search sessions..."), {
      key: "Enter",
    });
    expect(onSelectSession).toHaveBeenCalledWith("b");
  });

  it("says nothing about currency when no session is attached", () => {
    renderSwitcher(undefined);
    expect(screen.queryByText("Current")).toBeNull();
  });

  it("keeps the keyboard cursor visible on the current session's row", () => {
    // The cursor no longer starts on the current row, but it can still be moved
    // onto it — and that overlap is what the old `bg-primary/10` tint erased,
    // since tailwind-merge dropped the cursor's own background outright.
    //
    // Two things this has to get right or it asserts nothing: the key goes to
    // the search input, since that's where the handler lives (a keydown on
    // window never reaches React's tree), and the classes are compared as whole
    // tokens, since an unselected row carries `hover:bg-accent/50` and would
    // satisfy a substring match for "bg-accent" either way.
    renderSwitcher("a");

    const rowFor = (name: string) =>
      screen.getByText(name).closest("button")!.className.split(/\s+/);

    // Opens on the non-current row...
    expect(rowFor("second")).toContain("bg-accent");

    // ...and arrowing back onto the current one still shows the cursor there.
    fireEvent.keyDown(screen.getByPlaceholderText("Search sessions..."), {
      key: "ArrowUp",
    });

    expect(rowFor("first")).toContain("bg-accent");
    expect(rowFor("second")).not.toContain("bg-accent");
  });
});
