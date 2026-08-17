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

  it("says nothing about currency when no session is attached", () => {
    renderSwitcher(undefined);
    expect(screen.queryByText("Current")).toBeNull();
  });

  it("keeps the keyboard cursor visible on the current session's row", () => {
    // Selection starts on index 0, which is also the current session here —
    // the exact overlap the old tint erased.
    renderSwitcher("a");

    const rowFor = (name: string) =>
      screen.getByText(name).closest("button")!.className.split(/\s+/);

    expect(rowFor("first")).toContain("bg-accent");

    // ...and it still moves off it. Two things this has to get right or it
    // asserts nothing: the key goes to the search input, since that's where the
    // handler lives (a keydown on window never reaches React's tree), and the
    // classes are compared as whole tokens, since an unselected row carries
    // `hover:bg-accent/50` and would satisfy a substring match for "bg-accent"
    // whether or not the cursor ever moved.
    fireEvent.keyDown(screen.getByPlaceholderText("Search sessions..."), {
      key: "ArrowDown",
    });

    expect(rowFor("first")).not.toContain("bg-accent");
    expect(rowFor("second")).toContain("bg-accent");
  });
});
