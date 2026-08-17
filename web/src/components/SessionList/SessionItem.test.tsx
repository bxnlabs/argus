import { describe, it, expect, vi, afterEach, beforeAll } from "vitest";
import { useState } from "react";
import {
  render,
  screen,
  fireEvent,
  cleanup,
  act,
} from "@testing-library/react";
import { SessionItem } from "./index";
import type { Session } from "@/types";
import type { BusyKind } from "@/hooks/useSessionMutationState";

afterEach(cleanup);

beforeAll(() => {
  // The real Radix dropdown needs jsdom-missing APIs to open.
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

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess-1",
    name: "My Session",
    slug: "my-session",
    tmux_name: "claude-sess-1",
    created_at: "2026-01-01 00:00:00",
    updated_at: "2026-01-01 00:00:00",
    working_directory: "/home/user/project",
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

type ItemProps = React.ComponentProps<typeof SessionItem>;

function itemProps(overrides: Partial<ItemProps> = {}): ItemProps {
  return {
    session: makeSession(),
    homeDir: "/home/user",
    isActive: false,
    statusValue: "idle",
    unreadSince: null,
    userMarkedUnreadAt: null,
    minuteTick: 0,
    isRenaming: false,
    renameValue: "",
    renameInputRef: () => {},
    onRenameValueChange: () => {},
    onConfirmRename: () => {},
    onCancelRename: () => {},
    onStartRename: () => {},
    onAttachSession: () => {},
    onDeleteSession: () => {},
    onCloneSession: () => {},
    onChangeProfile: () => {},
    onViewInfo: () => {},
    canChangeProfile: false,
    onTogglePin: () => {},
    onMarkRead: () => {},
    onMarkUnread: () => {},
    renamePendingRef: { current: false },
    ...overrides,
  };
}

function renderItem(busy?: BusyKind) {
  const onAttachSession = vi.fn();
  const { container } = render(
    <SessionItem {...itemProps({ onAttachSession, busy })} />,
  );
  // SessionItem's root element is the row div — not portaled, so container works.
  const row = container.firstElementChild as HTMLElement;
  return { onAttachSession, row, container };
}

describe("SessionItem busy state", () => {
  it("renders a spinner and the busy label while deleting", () => {
    const { container } = renderItem("deleting");
    expect(container.textContent).toContain("Deleting…");
    expect(container.querySelector(".animate-spin")).not.toBeNull();
  });

  it("marks the row aria-busy and dims it", () => {
    const { row } = renderItem("deleting");
    expect(row.getAttribute("aria-busy")).toBe("true");
    expect(row.className).toContain("opacity-60");
  });

  // The primary pointer guard, and the only one real pointer input ever meets:
  // it takes the whole row out of hit testing, actions trigger included. Asserted
  // on the class because jsdom does not do hit testing — dispatching a pointer
  // event at the trigger reaches its handler regardless of this, so no
  // event-based test in this suite can notice if it goes away.
  it("takes the busy row out of hit testing", () => {
    expect(renderItem("deleting").row.className).toContain("pointer-events-none");
    expect(renderItem(undefined).row.className).not.toContain(
      "pointer-events-none",
    );
  });

  // Kept mounted AND focusable so Radix's focus restore on menu close has a
  // live target — unmounting it, or disabling it, drops focus to <body>.
  it("keeps the actions menu trigger focusable but inert while busy", () => {
    renderItem("deleting");
    const trigger = screen.getByLabelText(
      "Session actions",
    ) as HTMLButtonElement;
    expect(trigger.getAttribute("aria-disabled")).toBe("true");
    expect(trigger.disabled).toBe(false);
    trigger.focus();
    expect(document.activeElement).toBe(trigger);
  });

  // The regression this guards: choosing a menu action makes the row busy in
  // the same commit that closes the menu, and Radix then restores focus to the
  // trigger. If the trigger is gone (or disabled) at that moment, focus falls
  // to <body> and the next Tab restarts from the top of the document.
  it("keeps focus on the trigger when a menu action makes the row busy", async () => {
    function Harness() {
      const [busy, setBusy] = useState<BusyKind | undefined>(undefined);
      return (
        <SessionItem
          {...itemProps({ busy, onCloneSession: () => setBusy("cloning") })}
        />
      );
    }
    render(<Harness />);
    const trigger = screen.getByLabelText("Session actions");
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter" });

    const clone = await screen.findByText("Clone");
    await act(async () => {
      fireEvent.click(clone);
      // Let Radix's close/focus-restore effects flush.
      await new Promise((resolve) => setTimeout(resolve, 0));
    });

    expect(trigger.getAttribute("aria-disabled")).toBe("true");
    expect(screen.queryByText("Clone")).toBeNull();
    expect(document.activeElement).toBe(trigger);
  });

  // Radix opens the menu asynchronously, and its open/close is a *toggle* —
  // so each gesture is fired and settled on its own, or a second gesture would
  // close a menu the first one wrongly opened and the test would pass blind.
  //
  // Both gestures are dispatched straight at the trigger, which is the whole
  // story for keyDown — the row's pointer-events-none does not apply to the
  // keyboard and the trigger stays focusable, so this is the real user path.
  // For pointerDown it is not: real pointer input never gets past the row, so
  // this covers only the synthesised/programmatic backstop. The guarantee that
  // matters for a mouse is asserted in "takes the busy row out of hit testing".
  it.each(["keyDown", "pointerDown"] as const)(
    "does not open the actions menu on a %s dispatched at the trigger while busy",
    async (gesture) => {
      renderItem("deleting");
      const trigger = screen.getByLabelText("Session actions");
      await act(async () => {
        if (gesture === "keyDown") {
          fireEvent.keyDown(trigger, { key: "Enter" });
        } else {
          fireEvent.pointerDown(trigger);
        }
        await new Promise((resolve) => setTimeout(resolve, 0));
      });
      expect(screen.queryByText("Clone")).toBeNull();
    },
  );

  // Only the keys Radix's trigger opens on may be cancelled. jsdom does not
  // move focus on Tab, so assert what the browser actually keys off —
  // cancelling Tab pins focus to a trigger we deliberately kept focusable.
  it.each([
    ["Enter", true],
    [" ", true],
    ["ArrowDown", true],
    ["Tab", false],
    ["Escape", false],
    ["ArrowUp", false],
  ] as const)("while busy, cancels %s: %s", (key, cancelled) => {
    renderItem("deleting");
    const trigger = screen.getByLabelText("Session actions");
    const event = new KeyboardEvent("keydown", {
      key,
      bubbles: true,
      cancelable: true,
    });
    trigger.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(cancelled);
  });

  it("ignores clicks while busy", () => {
    const { onAttachSession, row } = renderItem("deleting");
    fireEvent.click(row);
    expect(onAttachSession).not.toHaveBeenCalled();
  });

  it("behaves normally when not busy", () => {
    const { onAttachSession, row, container } = renderItem(undefined);
    expect(row.getAttribute("aria-busy")).toBeNull();
    expect(container.querySelector(".animate-spin")).toBeNull();
    const trigger = screen.getByLabelText("Session actions");
    expect(trigger.getAttribute("aria-disabled")).toBe("false");
    fireEvent.click(row);
    expect(onAttachSession).toHaveBeenCalledWith("sess-1");
  });
});

describe("SessionItem currency pill", () => {
  it("marks the active row with a white pill, and only the active row", () => {
    // White is the shared currency color across all three rails, leaving blue to
    // mean "unread".
    const { container } = render(<SessionItem {...itemProps({ isActive: true })} />);
    const pill = container.querySelector("[data-testid='session-pill']");
    expect(pill).not.toBeNull();
    expect(pill?.className).toContain("bg-white");

    cleanup();
    const { container: inactive } = render(
      <SessionItem {...itemProps({ isActive: false })} />,
    );
    expect(inactive.querySelector("[data-testid='session-pill']")).toBeNull();
  });
});
