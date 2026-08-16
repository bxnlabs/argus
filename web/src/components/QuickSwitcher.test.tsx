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

  it("says nothing about currency when no session is attached", () => {
    renderSwitcher(undefined);
    expect(screen.queryByText("Current")).toBeNull();
  });

  it("keeps the keyboard cursor visible on the current session's row", () => {
    // Selection starts on index 0, which is also the current session here —
    // the exact overlap the old tint erased.
    renderSwitcher("a");

    const first = screen.getByText("first").closest("button")!;
    expect(first.className).toMatch(/bg-accent/);

    // ...and it still moves off it.
    fireEvent.keyDown(window, { key: "ArrowDown" });
    const second = screen.getByText("second").closest("button")!;
    expect(second.className).toMatch(/bg-accent/);
  });
});
