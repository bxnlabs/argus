import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
  act,
} from "@testing-library/react";
import { ChangeProfileDialog } from "./index";
import type { Session } from "@/types";

// --- Mock data + viewport hooks ---
vi.mock("@/data/sessions", () => ({
  useProfilesQuery: () => ({
    data: {
      profiles: [
        { name: "default", type: "host" },
        { name: "review", type: "host" },
        { name: "sandbox", type: "docker" },
      ],
    },
  }),
}));
vi.mock("@/hooks/useViewport", () => ({
  useViewport: () => ({ isMobile: false, isDesktop: true, isHydrated: true }),
}));

const mocks = vi.hoisted(() => ({ busySessions: {} as Record<string, string> }));
vi.mock("@/hooks/useSessionMutationState", () => ({
  useSessionMutationState: () => ({
    isCreating: false,
    busySessions: mocks.busySessions,
  }),
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
afterEach(() => {
  mocks.busySessions = {};
});

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

  it("applies the changed profile on Cmd+Enter", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onApply).toHaveBeenCalledWith("sess-1", "default");
    // The dialog does not close itself — App closes it in its success branch.
    expect(onClose).not.toHaveBeenCalled();
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
    expect(onClose).not.toHaveBeenCalled();
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
    expect(onClose).not.toHaveBeenCalled();
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

  it("renders the dockerized badge for a dockerized profile", async () => {
    renderDialog({ profile: null });
    await screen.findByRole("dialog");
    // Open the dropdown so the profile options (and their badges) render.
    fireEvent.click(screen.getByRole("combobox"));
    await screen.findByRole("option", { name: /sandbox/ });
    expect(screen.getByLabelText("dockerized")).toBeTruthy();
  });

  it("renders the Cmd/Ctrl+Enter hint on the Apply button (desktop)", async () => {
    renderDialog({ profile: null });
    await screen.findByRole("dialog");
    // The kbd hint is aria-hidden, so the button's accessible name stays "Apply".
    const apply = screen.getByRole("button", { name: "Apply" });
    expect(apply.textContent).toMatch(/↵/);
  });
});

describe("ChangeProfileDialog busy state", () => {
  it("does not close itself on apply — App closes it on success", async () => {
    const onApply = vi.fn();
    const onClose = vi.fn();
    render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={onClose}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    // Select a different profile so Apply is genuinely enabled, then submit.
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /apply/i }));
    expect(onApply).toHaveBeenCalledWith("sess-1", "default");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("shows a spinner and 'Applying…' while the profile change is in flight", () => {
    const session = makeSession();
    mocks.busySessions = { [session.id]: "profile" };
    render(
      <ChangeProfileDialog session={session} onClose={() => {}} onApply={() => {}} />,
    );
    const apply = screen.getByRole("button", {
      name: /applying/i,
    }) as HTMLButtonElement;
    expect(apply.disabled).toBe(true);
    expect(apply.textContent).toContain("Applying…");
    expect(apply.querySelector(".animate-spin")).not.toBeNull();
  });

  it("keeps Cancel live while applying", () => {
    const session = makeSession();
    mocks.busySessions = { [session.id]: "profile" };
    const onClose = vi.fn();
    render(
      <ChangeProfileDialog session={session} onClose={onClose} onApply={() => {}} />,
    );
    const cancel = screen.getByRole("button", {
      name: /cancel/i,
    }) as HTMLButtonElement;
    expect(cancel.disabled).toBe(false);
    fireEvent.click(cancel);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  // An apply still in flight, as a real one is for most of its life.
  function pendingApply() {
    return vi.fn(() => new Promise<void>(() => {}));
  }

  // The guard cannot lean on `isApplying` alone: TanStack recomputes its
  // snapshot on each cache event but schedules React's re-read, so `isApplying`
  // is still false for anything dispatched in the same tick as the first apply.
  // Holding the mock false reproduces that window. Duplicates matter here
  // because a profile change restarts the session — twice means two restarts,
  // and the second request races the id change.
  it("applies once for repeat applies inside the isApplying gap", async () => {
    const onApply = pendingApply();
    render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");

    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));

    expect(onApply).toHaveBeenCalledTimes(1);
  });

  // The whole lock lifecycle in one test, so it fails on both sides: on the
  // unlocked original (the mid-flight retry gets through) and on an
  // overcorrection that strands the lock (the post-settle retry does not).
  //
  // The mid-flight retry waits out a macrotask first, which is the window that
  // actually matters: TanStack schedules its notify with setTimeout(0)
  // (`systemSetTimeoutZero`), not a microtask, so `isApplying` lags the
  // dispatch by a whole task rather than a tick. The release has to hold
  // across that without ever having *seen* `isApplying` go true — an apply
  // that settles before the notify lands never renders a pending snapshot at
  // all, and App leaves the dialog open on failure, so a lock stranded by that
  // ordering blocks every retry.
  it("holds the lock until the apply settles, then releases it", async () => {
    let settle!: () => void;
    const onApply = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settle = resolve;
        }),
    );
    render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(onApply).toHaveBeenCalledTimes(1);

    // A later task, still mid-flight, with `isApplying` held false: blocked.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(onApply).toHaveBeenCalledTimes(1);

    // Settled: retryable again.
    await act(async () => settle());
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(onApply).toHaveBeenCalledTimes(2);
  });

  // A void-returning `onApply` narrows the lock to the current tick rather than
  // stranding it — the prop type allows one, so the release must not assume a
  // promise came back.
  it("stays retryable when onApply returns void", async () => {
    const onApply = vi.fn();
    render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    await act(async () => {});
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(onApply).toHaveBeenCalledTimes(2);
  });

  // The lock is a ref, and a ref change does not re-render. Without a render
  // mirror the controls stay live for the task it takes `isApplying` to
  // arrive — locked but not looking it. `busySessions` stays empty here, so
  // only the mirror can account for the disabled state.
  it("looks applying as soon as the apply is dispatched, before isApplying arrives", async () => {
    const onApply = pendingApply();
    render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));

    const apply = screen.getByRole("button", {
      name: /applying/i,
    }) as HTMLButtonElement;
    expect(apply.disabled).toBe(true);
    expect(apply.querySelector(".animate-spin")).not.toBeNull();
    expect((screen.getByRole("combobox") as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  // A failed apply re-enables the controls: App keeps the dialog open on
  // failure precisely so the selection can be retried.
  it("stops looking applying once the apply settles", async () => {
    let settle!: () => void;
    const onApply = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settle = resolve;
        }),
    );
    render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(screen.getByRole("button", { name: /applying/i })).toBeTruthy();

    await act(async () => settle());
    const apply = screen.getByRole("button", {
      name: /^apply$/i,
    }) as HTMLButtonElement;
    expect(apply.disabled).toBe(false);
  });

  // Retarget clears the mirror with the lock, so the next session never opens
  // already claiming to apply.
  it("does not carry the applying state to a different session", async () => {
    const onApply = pendingApply();
    const { rerender } = render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(screen.getByRole("button", { name: /applying/i })).toBeTruthy();

    rerender(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-2", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    expect(screen.queryByRole("button", { name: /applying/i })).toBeNull();
  });

  // An apply outlives the dialog's target: App keeps the dialog open on
  // failure, but the user can dismiss it mid-flight and open another session's.
  // The late settle must not clear the lock the *new* target is holding.
  it("does not let a stale apply release a retargeted session's lock", async () => {
    const settles: Array<() => void> = [];
    const onApply = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settles.push(resolve);
        }),
    );
    const { rerender } = render(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-1", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(onApply).toHaveBeenCalledTimes(1);

    // Retarget to another session; its apply takes the lock.
    rerender(
      <ChangeProfileDialog
        session={makeSession({ id: "sess-2", profile: null })}
        onClose={() => {}}
        onApply={onApply}
      />,
    );
    await selectProfile("review");
    fireEvent.click(screen.getByRole("button", { name: /^apply/i }));
    expect(onApply).toHaveBeenCalledTimes(2);

    // sess-1 finally settles. Its release belongs to a generation the dialog
    // has moved past, so sess-2's in-flight apply keeps both the lock and the
    // pending look.
    await act(async () => settles[0]());
    expect(screen.getByRole("button", { name: /applying/i })).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onApply).toHaveBeenCalledTimes(2);
  });

  it("ignores another session's in-flight profile change", () => {
    mocks.busySessions = { "some-other-session": "profile" };
    render(
      <ChangeProfileDialog
        session={makeSession()}
        onClose={() => {}}
        onApply={() => {}}
      />,
    );
    expect(screen.queryByRole("button", { name: /applying/i })).toBeNull();
  });
});
