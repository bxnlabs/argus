import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
} from "@testing-library/react";
import { ChangeProfileDialog } from "./index";
import type { Session } from "@/types";

// --- Mock data + viewport hooks ---
vi.mock("@/data/sessions", () => ({
  useProfilesQuery: () => ({ data: { profiles: ["default", "review"] } }),
}));
vi.mock("@/hooks/useViewport", () => ({
  useViewport: () => ({ isMobile: false, isDesktop: true, isHydrated: true }),
}));

// Radix Select renders internals that depend on these jsdom-missing APIs.
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

afterEach(cleanup);

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess-123",
    name: "My Session",
    tmux_name: "claude-sess-123",
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

function renderDialog(sessionOverrides: Partial<Session> = {}) {
  const onApply = vi.fn();
  const onClose = vi.fn();
  render(
    <ChangeProfileDialog
      session={makeSession(sessionOverrides)}
      onClose={onClose}
      onApply={onApply}
    />,
  );
  return { onApply, onClose };
}

// Open the Radix Select and pick an option by visible name.
async function selectProfile(name: string) {
  fireEvent.click(screen.getByRole("combobox"));
  fireEvent.click(await screen.findByRole("option", { name }));
}

describe("ChangeProfileDialog keyboard submit", () => {
  it("does not apply on Cmd+Enter when the selection is unchanged", async () => {
    const { onApply, onClose } = renderDialog({ profile: null });
    await screen.findByRole("dialog");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onApply).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("applies the changed profile and closes on Cmd+Enter", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onApply).toHaveBeenCalledWith("sess-1", "default");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("applies on Ctrl+Enter as well", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("review");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      ctrlKey: true,
    });
    expect(onApply).toHaveBeenCalledWith("sess-1", "review");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not apply a stale profile on Cmd+Enter while the dropdown is open", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");

    // Commit an initial change so `selected` differs from the session profile.
    await selectProfile("default");

    // Reopen the dropdown and keyboard-submit while a *different* option is
    // focused. The new "review" selection is only scheduled, not yet committed,
    // so pre-guard this bubbled to the dialog and applied the stale "default".
    // The containment guard must ignore this portaled keydown instead.
    fireEvent.click(screen.getByRole("combobox"));
    const option = await screen.findByRole("option", { name: "review" });
    option.focus(); // model the real keyboard path (option has focus)
    fireEvent.keyDown(option, { key: "Enter", metaKey: true });

    expect(onApply).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();

    // A follow-up Cmd+Enter from the (now-focused) trigger applies "review".
    const trigger = screen.getByRole("combobox");
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter", metaKey: true });
    expect(onApply).toHaveBeenCalledWith("sess-1", "review");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("is a no-op (does not open the dropdown) on unchanged Cmd+Enter from the focused trigger", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");

    // The Select trigger is focused on open. Radix opens the dropdown on any
    // Enter, so without the capture-phase stopPropagation guard this shortcut
    // would pop the dropdown open instead of being a true no-op.
    const trigger = screen.getByRole("combobox");
    trigger.focus();
    fireEvent.keyDown(trigger, { key: "Enter", metaKey: true });

    expect(screen.queryByRole("listbox")).toBeNull();
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(onApply).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("ignores plain Enter (no modifier)", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Enter" });
    expect(onApply).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("renders the Cmd/Ctrl+Enter hint on the Apply button (desktop)", async () => {
    renderDialog({ profile: null });
    await screen.findByRole("dialog");
    // The kbd hint is aria-hidden, so the button's accessible name stays "Apply".
    const apply = screen.getByRole("button", { name: "Apply" });
    expect(apply.textContent).toMatch(/↵/);
  });
});
