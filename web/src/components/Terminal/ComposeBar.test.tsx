import { describe, it, expect, vi, afterEach, beforeEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
  act,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ComposeBar } from "./ComposeBar";
import { StubNodeProvider } from "@/test/node-context";
import type { ComponentProps } from "react";

afterEach(cleanup);

// jsdom has no ResizeObserver and no layout engine, so tests drive the observer
// by hand to assert the observed-height -> overlay-height conversion.
class MockResizeObserver {
  static instances: MockResizeObserver[] = [];
  callback: ResizeObserverCallback;

  constructor(cb: ResizeObserverCallback) {
    this.callback = cb;
    MockResizeObserver.instances.push(this);
  }
  observe() {}
  unobserve() {}
  disconnect() {}

  emit(height: number) {
    act(() => {
      this.callback(
        [{ contentRect: { height } } as ResizeObserverEntry],
        this as unknown as ResizeObserver,
      );
    });
  }
}

beforeEach(() => {
  MockResizeObserver.instances = [];
  globalThis.ResizeObserver =
    MockResizeObserver as unknown as typeof ResizeObserver;
});

function observer() {
  const ro = MockResizeObserver.instances[0];
  if (!ro) throw new Error("ComposeBar did not observe its panel");
  return ro;
}

// ComposeBar renders FilePicker (Task 2's insertPaths consumer), which pulls
// in the upload mutation (useQueryClient) and the active-node scope
// (useNodeContext) regardless of whether the picker is open. Wrap every
// render with the same fixtures FileBrowser.test.tsx uses so those hooks
// resolve instead of throwing "No QueryClient set" / "must be used within
// NodeProvider".
function renderComposeBar(props: ComponentProps<typeof ComposeBar>) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <ComposeBar {...props} />
      </StubNodeProvider>
    </QueryClientProvider>,
  );
}

function textarea() {
  return screen.getByRole("textbox") as HTMLTextAreaElement;
}

describe("ComposeBar", () => {
  it("sends the typed text and clears the input", () => {
    const onSend = vi.fn(() => true);
    renderComposeBar({ onSend, connected: true });

    fireEvent.change(textarea(), { target: { value: "run the tests" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(onSend).toHaveBeenCalledWith("run the tests");
    expect(textarea().value).toBe("");
  });

  it("keeps focus on the input after sending so the keyboard stays up", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    fireEvent.focus(textarea());
    fireEvent.change(textarea(), { target: { value: "hi" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(document.activeElement).toBe(textarea());
  });

  it("disables send when the draft is only whitespace", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    fireEvent.change(textarea(), { target: { value: "   " } });

    expect(screen.getByRole("button", { name: /send/i })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("disables send while disconnected but preserves the draft", () => {
    renderComposeBar({ onSend: () => true, connected: false });

    fireEvent.change(textarea(), { target: { value: "queued message" } });

    expect(screen.getByRole("button", { name: /send/i })).toHaveProperty(
      "disabled",
      true,
    );
    expect(textarea().value).toBe("queued message");
  });

  it("keeps the draft when the send never reached the socket", () => {
    // `connected` is a render-time snapshot, so the socket can close between
    // the last connected render and the tap — the send button is still
    // enabled at that instant. onSend reports whether the write actually
    // landed; a draft must never be cleared on a write that went nowhere.
    const onSend = vi.fn(() => false);
    renderComposeBar({ onSend, connected: true });

    fireEvent.change(textarea(), { target: { value: "queued message" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(onSend).toHaveBeenCalledWith("queued message");
    expect(textarea().value).toBe("queued message");
  });

  it("keeps the send button mounted at all times, as Slack does", () => {
    // Replaces the old focus/draft-gated visibility. A ghost glyph is quiet
    // enough at rest that hiding it buys nothing, and its disabled state now
    // carries "present, not yet available" on its own.
    renderComposeBar({ onSend: () => true, connected: true });

    const send = () => screen.getByRole("button", { name: /send/i });
    expect(send()).toHaveProperty("disabled", true);

    fireEvent.focus(textarea());
    expect(send()).toHaveProperty("disabled", true);

    fireEvent.change(textarea(), { target: { value: "draft" } });
    expect(send()).toHaveProperty("disabled", false);

    fireEvent.blur(textarea());
    // A draft must stay sendable after tapping away to the terminal.
    expect(send()).toHaveProperty("disabled", false);
  });

  it("renders both actions as ghost glyphs, with send the only coloured one", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const attach = screen.getByRole("button", { name: /attach/i });
    const send = screen.getByRole("button", { name: /send/i });

    // No filled circles: a filled attach beside a ghost send would make the
    // secondary action heavier than the primary one.
    expect(attach.className).not.toMatch(/\bbg-(?:[a-z]+(?:-\d+)?|\[[^\]]*\])/);
    expect(send.className).not.toMatch(/\bbg-(primary|\[hsl)/);

    expect(attach.className).toContain("text-[hsl(0_0%_60%)]");
    expect(send.className).toContain("text-primary");
    expect(send.className).toContain("disabled:text-[hsl(0_0%_45%)]");
  });

  it("names the session in the placeholder, Slack-style", () => {
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "mobile-persistent-input",
    });

    expect(textarea().placeholder).toBe("Message #mobile-persistent-input");
  });

  it("keeps one placeholder across focus and blur", () => {
    // Replaces the old focused/unfocused swap. Focus is now carried by the
    // card edge brightening alone, so the placeholder must hold still.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "argus",
    });

    expect(textarea().placeholder).toBe("Message #argus");
    fireEvent.focus(textarea());
    expect(textarea().placeholder).toBe("Message #argus");
    fireEvent.blur(textarea());
    expect(textarea().placeholder).toBe("Message #argus");
  });

  it("drops the channel when there is no session, as on a raw-shell tab", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    expect(textarea().placeholder).toBe("Message");
  });

  it("truncates a long slug rather than letting it clip mid-word at the input edge", () => {
    // A textarea placeholder cannot ellipsize itself. 36 chars is a real
    // session name from `argus session ls`.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "review-mike-dp-host-self-registration",
    });

    expect(textarea().placeholder).toBe("Message #review-mike-dp-host-self-re…");
    expect(textarea().placeholder.length).toBe("Message #".length + 28);
  });

  it("dims the placeholder without dimming the draft", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    expect(textarea().className).toContain("placeholder:text-[hsl(0_0%_50%)]");
    expect(textarea().className).not.toMatch(/(^|\s)text-\[hsl/);
  });

  it("never dims the draft text itself when focus leaves", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    fireEvent.change(textarea(), { target: { value: "still readable" } });
    fireEvent.blur(textarea());

    // Dimming is chrome-only — the textarea must not carry an opacity class.
    expect(textarea().className).not.toMatch(/opacity-/);
  });

  it("does not send on Enter — Enter inserts a newline", () => {
    const onSend = vi.fn(() => true);
    renderComposeBar({ onSend, connected: true });

    fireEvent.change(textarea(), { target: { value: "line one" } });
    fireEvent.keyDown(textarea(), { key: "Enter" });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("prevents default on mousedown for the send button, so tapping it never blurs the textarea and drops the mobile keyboard", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    fireEvent.focus(textarea());
    fireEvent.change(textarea(), { target: { value: "hi" } });

    // fireEvent returns false when the event's default was prevented. This
    // is the actual mechanism the mobile keyboard-stays-up UX rests on:
    // handleSend's explicit re-focus only matters because the mousedown
    // that precedes the click never gets a chance to blur the textarea.
    const sendButton = screen.getByRole("button", { name: /send/i });
    expect(fireEvent.mouseDown(sendButton)).toBe(false);
  });

  it("prevents default on mousedown for the attach button, for the same reason as the send button", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const attachButton = screen.getByRole("button", { name: /attach/i });
    expect(fireEvent.mouseDown(attachButton)).toBe(false);
  });

  it("opens the file picker from the attach button", () => {
    renderComposeBar({
      onSend: () => true,
      connected: true,
      workingDirectory: "/w",
    });

    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /attach/i }));

    expect(screen.queryByRole("dialog")).not.toBeNull();
  });
});

describe("ComposeBar overlay height", () => {
  it("reports zero for the first observed height, which is the collapsed baseline", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    observer().emit(44);

    // toHaveBeenCalledWith only checks that SOME call matched; pin down the
    // call count too so an implementation that fires onOverlayHeightChange(0)
    // unconditionally (regardless of the observed height) can't pass this in
    // isolation.
    expect(onOverlayHeightChange).toHaveBeenCalledTimes(1);
    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("keeps the collapsed baseline and never rebuilds the observer when onOverlayHeightChange's identity churns before an early re-render", () => {
    // Regression test for a baseline-corruption race: if the ResizeObserver
    // effect depended on onOverlayHeightChange, a parent that doesn't
    // memoize its callback (a fresh function identity every render) would
    // tear down and rebuild the observer. The component's contract is that
    // collapsedHeight is the first height observed after mount; rebuilding
    // mid-life risks recapturing that baseline at an already-grown height.
    // The fix builds the observer exactly once and reads the callback
    // through a ref, so this must hold regardless of how often the parent
    // re-renders with a new callback identity.
    const calls: number[] = [];
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const tree = (onOverlayHeightChange: (height: number) => void) => (
      <QueryClientProvider client={queryClient}>
        <StubNodeProvider>
          <ComposeBar
            onSend={() => true}
            connected={true}
            onOverlayHeightChange={onOverlayHeightChange}
          />
        </StubNodeProvider>
      </QueryClientProvider>
    );

    // A fresh inline callback on the first render, like an unmemoized parent.
    const { rerender } = render(tree((h) => calls.push(h)));

    observer().emit(44); // baseline, one line

    // Force a parent re-render that changes onOverlayHeightChange's identity
    // before any further observation fires.
    rerender(tree((h) => calls.push(h)));

    observer().emit(84); // grown

    // Exactly one ResizeObserver should ever be constructed for the life of
    // the component instance.
    expect(MockResizeObserver.instances.length).toBe(1);
    // The overlay must still be measured against the ORIGINAL 44px
    // baseline, not a baseline recaptured mid-life.
    expect(calls).toEqual([0, 40]);
  });

  it("reports the overflow past the collapsed baseline as the panel grows", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    observer().emit(44); // baseline, one line
    observer().emit(64); // two lines
    observer().emit(84); // three lines

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(40);
  });

  it("never reports a negative height if the panel measures under its baseline", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    observer().emit(44);
    observer().emit(30);

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("ignores a zero-height observation instead of taking it as the collapsed baseline", () => {
    // Reachable today: an inactive tab is mounted inside a display:none
    // ancestor (Workspace hides inactive tabs with `hidden`). A
    // ResizeObserver on an element inside a display:none ancestor fires
    // immediately with contentRect.height === 0, then fires again with the
    // true height once the ancestor becomes visible — measured in a real
    // browser as [0, 18]. Zero is never a valid collapsed baseline: taking
    // it would make the real height read as pure overflow and permanently
    // shift the terminal up when the tab is later selected.
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    observer().emit(0); // hidden mount — must be ignored entirely
    observer().emit(44); // real collapsed baseline, once shown

    // The zero observation must not even reach onOverlayHeightChange as a
    // spurious "0 overlay" call — it should be skipped outright.
    expect(onOverlayHeightChange).toHaveBeenCalledTimes(1);
    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);

    // Now the panel grows: the overlay must be measured against the real
    // 44px baseline, not the poisoned 0.
    observer().emit(84);
    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(40);
  });

  it("returns to zero when the panel shrinks back down to its collapsed baseline", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    observer().emit(44);
    observer().emit(84);
    observer().emit(44);

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("mirrors the draft text so the grid row grows without measuring", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    fireEvent.change(screen.getByRole("textbox"), {
      target: { value: "line one\nline two" },
    });

    const mirror = document.querySelector("[data-testid='compose-mirror']");
    // The trailing space is what makes a trailing newline reserve a line.
    expect(mirror?.textContent).toBe("line one\nline two ");
  });

  it("keeps the mirror and textarea sizing/typography classes identical, since jsdom does no layout to catch drift itself", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const mirror = document.querySelector("[data-testid='compose-mirror']");
    const ta = textarea();

    // The mirror legitimately carries break-words/whitespace-pre-wrap that
    // the textarea doesn't — a native textarea already wraps via the UA
    // stylesheet — so full class-string equality is the wrong assertion.
    // What must never drift is the set of tokens that determine each
    // line's rendered height (padding + font size + line-height): if only
    // one element keeps py-1.5, the font size, or the line-height, the
    // mirror mis-measures the textarea's content and the grid row sizes to
    // the wrong height, so the bar grows early or late.
    const sharedSizingTokens = [
      "py-1.5",
      "text-[15px]",
      "leading-[var(--compose-row-h)]",
    ];
    for (const token of sharedSizingTokens) {
      expect(mirror?.className.split(" ")).toContain(token);
      expect(ta.className.split(" ")).toContain(token);
    }
  });

  it("derives the spacer height and the three-line cap from one row variable, so they cannot drift apart", () => {
    // The spacer's height IS the panel's resting height — the panel is
    // anchored to its bottom edge. If the two disagree, the panel overflows
    // the spacer at rest, the observer reports a nonzero overlay with an
    // empty draft, and the terminal sits permanently shifted. jsdom has no
    // layout engine so this cannot compare real pixels; what it CAN pin is
    // that both numbers are derived from the same variable rather than
    // being two independently hardcoded values that happen to agree today.
    renderComposeBar({ onSend: () => true, connected: true });

    const wrapper = screen.getByTestId("compose-grow-wrapper");
    const mirror = screen.getByTestId("compose-mirror");
    const panel = wrapper.parentElement!;
    const spacer = panel.parentElement!;

    expect(spacer.style.getPropertyValue("--compose-row-h")).toBe("21px");
    // one row (line-height + the mirror's py-1.5) + the panel's py-1 + its
    // 1px border-t. The panel's padding is compacted to pay for the card's
    // four-sided margin; the MIRROR keeps py-1.5, which is why
    // --compose-max-h below is untouched.
    expect(spacer.style.height).toBe(
      "calc(var(--compose-row-h) + 1.25rem + 1px)",
    );
    expect(wrapper.style.getPropertyValue("--compose-max-h")).toContain(
      "var(--compose-row-h)",
    );

    // The formula's other two terms are class-driven, so pin them too: the
    // 1.25rem is the mirror's py-1.5 (12px) PLUS the panel's py-1 (8px), and
    // the 1px is the panel's border-t. Without these, changing the panel's
    // padding leaves the spacer at its old value while the panel resting
    // height moves — silent drift that every assertion above still passes.
    expect(panel.className.split(" ")).toContain("py-1");
    expect(panel.className.split(" ")).toContain("border-t");
    expect(mirror.className.split(" ")).toContain("py-1.5");

    // A literal height on the spacer is exactly the regression this guards:
    // it would keep passing the cap test above while silently decoupling the
    // spacer from the row size.
    expect(spacer.className).not.toMatch(/\bh-\d/);
  });

  it("caps the mirror's own max-height, not just the wrapper's, so the textarea scrolls past three lines instead of being clipped", () => {
    // jsdom has no layout engine, so this cannot assert real pixel heights
    // or that ta.scrollTop actually moves — that was verified by hand in a
    // real browser (see the fix commit / task report). What this CAN prove
    // is the class/style contract the fix depends on: the wrapper and,
    // critically, the MIRROR both carry the same derived max-height driven
    // by one CSS variable. Before the fix, only the wrapper had a max-h
    // (coincidentally equal to 3 * its own inherited 24px line-height); the
    // mirror was uncapped, so the grid row still sized to the mirror's full
    // content and the textarea just got clipped by overflow-hidden rather
    // than becoming scrollable. A test that only checked the wrapper's
    // max-height would pass on that broken code too, so it's the mirror's
    // cap specifically that this asserts.
    renderComposeBar({ onSend: () => true, connected: true });

    const wrapper = screen.getByTestId("compose-grow-wrapper");
    const mirror = screen.getByTestId("compose-mirror");
    const ta = textarea();

    expect(wrapper.className).toMatch(/max-h-\[var\(--compose-max-h\)\]/);
    expect(mirror.className).toMatch(/max-h-\[var\(--compose-max-h\)\]/);

    // The cap must be a derived expression — 3 rows of --compose-row-h plus
    // the shared py-1.5 vertical padding (0.75rem top+bottom total) — not a
    // magic pixel value that only happens to match by coincidence.
    expect(wrapper.style.getPropertyValue("--compose-max-h")).toBe(
      "calc(3 * var(--compose-row-h) + 0.75rem)",
    );

    // The textarea must be set up to scroll internally once it exceeds the
    // capped grid row, rather than relying on the wrapper alone to grow.
    expect(ta.className.split(" ")).toContain("overflow-y-auto");
  });

  it("renders the panel as the card's top half", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const panel = screen.getByTestId("compose-grow-wrapper").parentElement!;
    const spacer = panel.parentElement!;

    // The panel and the toolbar are separate boxes that meet flush; each
    // renders half the card's border.
    expect(panel.className).toContain("rounded-t-lg");
    expect(panel.className).toContain("border-x");
    expect(panel.className).toContain("inset-x-2");
    expect(panel.className).toContain("border-[hsl(0_0%_20%)]");
    expect(spacer.className).toContain("mt-1.5");
  });

  it("reports focus changes so the toolbar half can brighten with the panel", () => {
    const onFocusedChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onFocusedChange,
    });

    fireEvent.focus(textarea());
    expect(onFocusedChange).toHaveBeenLastCalledWith(true);

    fireEvent.blur(textarea());
    expect(onFocusedChange).toHaveBeenLastCalledWith(false);
  });
});
