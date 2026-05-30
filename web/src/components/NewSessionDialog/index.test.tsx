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
    data: { profiles: ["default", "review"] },
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
  // The Profile <Select> renders real Radix internals that depend on
  // ResizeObserver, which jsdom does not provide.
  if (!("ResizeObserver" in globalThis)) {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }
});

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
    expect(buttonRow.className).toContain("justify-end");
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
