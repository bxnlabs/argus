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
} from "@testing-library/react";
import { NewSessionDialog } from "./index";

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

function renderDialog(
  overrides: Partial<Parameters<typeof NewSessionDialog>[0]> = {},
) {
  const onCreateSession = vi.fn();
  const onClose = vi.fn();
  render(
    <NewSessionDialog
      open
      onClose={onClose}
      onCreateSession={onCreateSession}
      {...overrides}
    />,
  );
  return { onCreateSession, onClose };
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
