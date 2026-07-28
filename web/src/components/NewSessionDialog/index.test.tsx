import {
  describe,
  it,
  expect,
  vi,
  beforeAll,
  afterEach,
} from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
  waitFor,
  act,
} from "@testing-library/react";
import { NewSessionDialog } from "./index";

const mocks = vi.hoisted(() => ({ isCreating: false }));
vi.mock("@/hooks/useSessionMutationState", () => ({
  useSessionMutationState: () => ({
    isCreating: mocks.isCreating,
    busySessions: {},
  }),
}));

// --- Mock data hooks ---
vi.mock("@/data/sessions", () => ({
  useProfilesQuery: () => ({
    data: {
      profiles: [
        { name: "default", type: "host" },
        { name: "review", type: "host" },
        { name: "sandbox", type: "docker" },
      ],
    },
    refetch: vi.fn(),
  }),
}));
vi.mock("@/data/git/queries", () => ({
  useGitCheckQuery: () => ({ data: false }),
}));
vi.mock("@/hooks/useViewport", () => ({
  useViewport: () => ({ isMobile: false, isDesktop: true, isHydrated: true }),
}));

// --- Stub the child pickers so we can observe open state and drive close ---
vi.mock("@/components/SourcePicker", () => ({
  SourcePicker: ({
    open,
    onOpenChange,
    onSelect,
  }: {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onSelect: (value: string, tab: "local" | "remote") => void;
  }) =>
    open ? (
      <div data-testid="source-picker">
        <button
          type="button"
          onClick={() => {
            onSelect("~/projects/argus", "local");
            onOpenChange(false);
          }}
        >
          pick-source
        </button>
      </div>
    ) : null,
}));

beforeAll(() => {
  if (!("requestAnimationFrame" in globalThis)) {
    globalThis.requestAnimationFrame = ((cb: FrameRequestCallback) =>
      setTimeout(() => cb(0), 0)) as typeof requestAnimationFrame;
  }
  // The Provider/Profile <Select>s render real Radix internals that depend on
  // jsdom-missing APIs (ResizeObserver, pointer-capture, scrollIntoView).
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

// The provider <Select> trigger displays the current provider's label; the
// profile <Select> shows a placeholder. Disambiguate by visible text.
function providerTrigger(): HTMLElement {
  const trigger = screen
    .getAllByRole("combobox")
    .find((c) => c.textContent?.includes("Claude Code"));
  if (!trigger) throw new Error("provider combobox not found");
  return trigger;
}

// The profile <Select> trigger shows the "Select a profile..." placeholder.
function profileTrigger(): HTMLElement {
  const trigger = screen
    .getAllByRole("combobox")
    .find((c) => c.textContent?.includes("Select a profile"));
  if (!trigger) throw new Error("profile combobox not found");
  return trigger;
}

afterEach(cleanup);
afterEach(() => {
  mocks.isCreating = false;
});

function renderDialog(
  overrides: Partial<Parameters<typeof NewSessionDialog>[0]> = {},
) {
  const onCreateSession = vi.fn();
  const onClose = vi.fn();
  // A fresh element each time: React bails out of a re-render when handed the
  // referentially identical element.
  const element = () => (
    <NewSessionDialog
      open
      onClose={onClose}
      onCreateSession={onCreateSession}
      {...overrides}
    />
  );
  const { rerender } = render(element());
  // Re-renders the same tree (state preserved) so a test can flip the
  // useSessionMutationState mock and have the component read the new value.
  return { onCreateSession, onClose, rerender: () => rerender(element()) };
}

function sourceTrigger(): HTMLButtonElement {
  return screen.getByRole("button", {
    name: /select a folder or repository/i,
  }) as HTMLButtonElement;
}

function nameInput(): HTMLInputElement {
  return screen.getByPlaceholderText("my-feature") as HTMLInputElement;
}

describe("NewSessionDialog keyboard flow", () => {
  it("renders Source as a non-submitting dialog-trigger button", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    const trigger = sourceTrigger();
    expect(trigger.type).toBe("button");
    expect(trigger.getAttribute("aria-haspopup")).toBe("dialog");
  });

  it("opens the Source picker when the trigger is activated", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    expect(screen.queryByTestId("source-picker")).toBeNull();
    fireEvent.click(sourceTrigger());
    expect(screen.getByTestId("source-picker")).toBeTruthy();
  });

  it("submits with Cmd+Enter when the name is non-empty", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "  my-feature  " } });
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ name: "my-feature", provider_type: "claude" }),
    );
  });

  it("submits with Ctrl+Enter as well", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      ctrlKey: true,
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
  });

  it("does not submit on Cmd+Enter when the name is empty", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onCreateSession).not.toHaveBeenCalled();
  });

  it("does not submit on plain Enter in the Name field", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });
    fireEvent.keyDown(nameInput(), { key: "Enter" });
    expect(onCreateSession).not.toHaveBeenCalled();
  });

  it("submits when the Create button is clicked", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(onCreateSession).toHaveBeenCalledTimes(1);
  });

  it("right-aligns the footer action buttons (consistent across mobile/desktop)", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    const createButton = screen.getByRole("button", { name: "Create" });
    const buttonRow = createButton.parentElement as HTMLElement;
    // justify-end right-aligns the buttons in the mobile column layout;
    // sm:ml-auto pushes the whole row right once the footer becomes a row,
    // so the buttons stay right-aligned even when the keyboard-hint child is
    // absent (e.g. landscape phones where the footer is a row but isMobile).
    expect(buttonRow.className).toContain("justify-end");
    expect(buttonRow.className).toContain("sm:ml-auto");
  });

  it("does not submit a stale provider on Cmd+Enter while the provider dropdown is open", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });

    // Open the provider dropdown and keyboard-submit while the "Codex" option
    // is focused. Radix only schedules the new provider, so without the guard
    // this bubbled to the dialog and submitted the stale "claude".
    fireEvent.click(providerTrigger());
    const codex = await screen.findByRole("option", { name: /Codex/ });
    codex.focus();
    fireEvent.keyDown(codex, { key: "Enter", metaKey: true });
    expect(onCreateSession).not.toHaveBeenCalled();

    // A follow-up Cmd+Enter submits with the newly-selected "codex" provider.
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Enter", metaKey: true });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ name: "feat", provider_type: "codex" }),
    );
  });

  it("renders the dockerized badge for a dockerized profile", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    // Open the profile dropdown so its options (and their badges) render.
    fireEvent.click(profileTrigger());
    await screen.findByRole("option", { name: /sandbox/ });
    expect(screen.getByLabelText("dockerized")).toBeTruthy();
  });

  it("returns focus to the Source trigger after the picker closes", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    // Capture the trigger up front: after selecting a source its label changes
    // from the placeholder to the value, so it can no longer be found by the
    // placeholder query — but it is the same DOM element that must regain focus.
    const trigger = sourceTrigger();
    fireEvent.click(trigger);
    fireEvent.click(screen.getByText("pick-source"));
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});

describe("NewSessionDialog busy state", () => {
  it("does not close itself on submit — App closes it on success", async () => {
    const { onCreateSession, onClose } = renderDialog();
    fireEvent.change(nameInput(), { target: { value: "my-feature" } });
    fireEvent.click(screen.getByRole("button", { name: /^create/i }));
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onClose).not.toHaveBeenCalled();
  });

  // A create still in flight, as a real one is for most of its life.
  function pendingCreate() {
    return vi.fn(() => new Promise<void>(() => {}));
  }

  // The serialisation guard cannot lean on `isCreating` alone: TanStack
  // recomputes its snapshot on each cache event but schedules React's re-read,
  // so `isCreating` is still false for anything dispatched in the same tick as
  // the first submit. Holding the mock false reproduces that window — App's
  // single-slot toast handoff breaks if a second create slips through it.
  it("submits once for repeat submits inside the isCreating gap", async () => {
    const onCreateSession = pendingCreate();
    renderDialog({ onCreateSession });
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "my-feature" } });

    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    // Keyboard submit runs off the dialog's capture handler, so it reaches
    // `createSession` even once the button is disabled — the ref is the only
    // thing standing between this and a second create.
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });

    expect(onCreateSession).toHaveBeenCalledTimes(1);
    // And the button is already out of reach for a pointer submit, with
    // `isCreating` still false — that is the render mirror, not the query.
    const submit = screen.getByRole("button", {
      name: /creating/i,
    }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(mocks.isCreating).toBe(false);
  });

  // The flip side of that lock: it must release on settle, and it must do so
  // without depending on ever having *seen* `isCreating` go true. A create
  // that settles before TanStack's scheduled notify reaches React never
  // renders a pending snapshot — an immediately-rejecting fetch dispatches
  // pending then error in the same task — so `isCreating` stays false for the
  // whole of this test. App leaves the dialog open on failure, so a lock
  // stranded by that ordering means the user can never retry.
  it("stays retryable when the create settles unobserved", async () => {
    let settle!: () => void;
    const onCreateSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settle = resolve;
        }),
    );
    renderDialog({ onCreateSession });
    fireEvent.change(nameInput(), { target: { value: "my-feature" } });
    fireEvent.click(screen.getByRole("button", { name: /^create/i }));
    expect(onCreateSession).toHaveBeenCalledTimes(1);

    await act(async () => settle());
    expect(mocks.isCreating).toBe(false);
    // The render mirror has to come back down with the lock, or the form stays
    // greyed out with no way to retry.
    expect(
      (document.querySelector("fieldset") as HTMLFieldSetElement).disabled,
    ).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: /^create/i }));
    expect(onCreateSession).toHaveBeenCalledTimes(2);
  });

  // Same mirror as ChangeProfileDialog: locked and looking it, in the commit
  // that dispatches rather than a task later when `isCreating` lands.
  it("looks creating as soon as the submit is dispatched, before isCreating arrives", () => {
    const onCreateSession = pendingCreate();
    renderDialog({ onCreateSession });
    fireEvent.change(nameInput(), { target: { value: "my-feature" } });
    fireEvent.click(screen.getByRole("button", { name: /^create/i }));

    const submit = screen.getByRole("button", {
      name: /creating/i,
    }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(submit.querySelector(".animate-spin")).not.toBeNull();
    expect(
      (document.querySelector("fieldset") as HTMLFieldSetElement).disabled,
    ).toBe(true);
    expect(mocks.isCreating).toBe(false);
    // Its own create — not someone else's.
    expect(
      screen.queryByText("Another session is still being created…"),
    ).toBeNull();
  });

  // `isCreating` must go true *after* this form submits, or the submit is
  // swallowed by createSession's serialisation guard and `submitted` never
  // gets set — which is exactly the state a reopened dialog is in.
  function submitThenGoBusy() {
    const handles = renderDialog();
    fireEvent.change(nameInput(), { target: { value: "my-feature" } });
    fireEvent.click(screen.getByRole("button", { name: /^create/i }));
    mocks.isCreating = true;
    handles.rerender();
    return handles;
  }

  it("shows a spinner and 'Creating…' on the submit button for its own create", () => {
    const { onCreateSession } = submitThenGoBusy();
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    const submit = screen.getByRole("button", {
      name: /creating/i,
    }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(submit.textContent).toContain("Creating…");
    expect(submit.querySelector(".animate-spin")).not.toBeNull();
  });

  // A dialog reopened mid-create sees a global `isCreating` it did not cause.
  // It must not claim to be creating the blank form it is showing.
  it("reads 'Create' and explains itself when another create is in flight", () => {
    mocks.isCreating = true;
    renderDialog();
    const submit = screen.getByRole("button", {
      name: /^create/i,
    }) as HTMLButtonElement;
    expect(submit.disabled).toBe(true);
    expect(submit.textContent).not.toContain("Creating…");
    expect(submit.querySelector(".animate-spin")).toBeNull();
    expect(
      screen.getByText("Another session is still being created…"),
    ).toBeTruthy();
  });

  it("omits the other-create note while creating its own session", () => {
    submitThenGoBusy();
    expect(
      screen.queryByText("Another session is still being created…"),
    ).toBeNull();
  });

  // A disabled <fieldset> does NOT set the `disabled` IDL property on its
  // descendants — that property reflects the content attribute only. Assert on
  // the fieldset itself, and that the fields live inside it.
  it("disables the form fields while creating", () => {
    mocks.isCreating = true;
    renderDialog();
    const fieldset = document.querySelector("fieldset") as HTMLFieldSetElement;
    expect(fieldset).not.toBeNull();
    expect(fieldset.disabled).toBe(true);
    expect(fieldset.contains(nameInput())).toBe(true);
    expect(fieldset.contains(sourceTrigger())).toBe(true);
  });

  it("keeps Cancel live and outside the disabled fieldset while creating", () => {
    mocks.isCreating = true;
    const { onClose } = renderDialog();
    const cancel = screen.getByRole("button", {
      name: /cancel/i,
    }) as HTMLButtonElement;
    const fieldset = document.querySelector("fieldset") as HTMLFieldSetElement;
    expect(cancel.disabled).toBe(false);
    expect(fieldset.contains(cancel)).toBe(false);
    fireEvent.click(cancel);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not submit again on Cmd+Enter while creating", () => {
    mocks.isCreating = true;
    const { onCreateSession } = renderDialog();
    fireEvent.change(nameInput(), {
      target: { value: "my-feature" },
    });
    fireEvent.keyDown(nameInput(), { key: "Enter", metaKey: true });
    expect(onCreateSession).not.toHaveBeenCalled();
  });

  // A create outlives the dialog's target: the user can dismiss the dialog
  // mid-create (App hands it off to a toast) and reopen it to start the next
  // session. The late settle from the abandoned create must not clear the
  // lock the *new* submit is holding.
  it("does not let a stale release clear a retargeted submit's lock", async () => {
    const settles: Array<() => void> = [];
    const onCreateSession = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          settles.push(resolve);
        }),
    );
    const { rerender } = render(
      <NewSessionDialog open onClose={() => {}} onCreateSession={onCreateSession} />,
    );
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "first" } });
    fireEvent.click(screen.getByRole("button", { name: /^create/i }));
    expect(onCreateSession).toHaveBeenCalledTimes(1);

    // Dismiss and reopen — the dialog's own lock resets even though the
    // first create keeps running in the background.
    rerender(
      <NewSessionDialog
        open={false}
        onClose={() => {}}
        onCreateSession={onCreateSession}
      />,
    );
    rerender(
      <NewSessionDialog open onClose={() => {}} onCreateSession={onCreateSession} />,
    );
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "second" } });
    fireEvent.click(screen.getByRole("button", { name: /^create/i }));
    expect(onCreateSession).toHaveBeenCalledTimes(2);

    // The first create finally settles. Its release belongs to a generation
    // the dialog has moved past, so the second create keeps both the lock
    // and the pending look.
    await act(async () => settles[0]());
    expect(screen.getByRole("button", { name: /creating/i })).toBeTruthy();
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onCreateSession).toHaveBeenCalledTimes(2);
  });
});
