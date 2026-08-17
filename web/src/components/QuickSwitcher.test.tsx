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
// facts. `cn` runs tailwind-merge, so one background class can't carry both: a
// second background silently replaces the first.
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
    // be anywhere in it.
    renderSwitcher("b");
    expect(rowNames()).toEqual(["second", "first"]);
  });

  it("leaves the order alone when the current session is already first", () => {
    renderSwitcher("a");
    expect(rowNames()).toEqual(["first", "second"]);
  });

  it("doesn't resurrect the current session when a search excludes it", () => {
    // Hoisting applies among the matches only.
    renderSwitcher("b");
    fireEvent.change(screen.getByPlaceholderText("Search sessions..."), {
      target: { value: "first" },
    });
    expect(rowNames()).toEqual(["first"]);
  });

  it("opens the cursor past the session you're on, not on it", () => {
    // The top row is where you already are, so landing there would make Enter
    // mean "stay put".
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
    // Nothing to skip to, so the cursor falls back to row 0.
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
    // Cmd+K is bound with no gate on the list having loaded (App), so the dialog
    // can open during the first fetch — or right after a node switch, which
    // empties the cache for the new scope — and the cursor has to re-land once
    // the sessions show up.
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
    // Sessions can leave the list while the dialog is open, and the cursor
    // commonly starts at row 1, so a list dropping to one row would leave it
    // past the end: no row selected, and Enter does nothing.
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
    // The cursor can be arrowed onto the current row and must still show there.
    // Keys go to the search input, where the handler lives; classes are compared
    // as whole tokens, since an unselected row carries `hover:bg-accent/50` and
    // would satisfy a substring match for "bg-accent" either way.
    renderSwitcher("a");

    const rowFor = (name: string) =>
      screen.getByText(name).closest("button")!.className.split(/\s+/);

    expect(rowFor("second")).toContain("bg-accent");

    fireEvent.keyDown(screen.getByPlaceholderText("Search sessions..."), {
      key: "ArrowUp",
    });

    expect(rowFor("first")).toContain("bg-accent");
    expect(rowFor("second")).not.toContain("bg-accent");
  });
});
