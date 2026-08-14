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
import { loadDraft, saveDraft, draftStorageKey } from "@/lib/composeDrafts";
import type { ComponentProps } from "react";

// StubNodeProvider fixes the active node to local, whose scope is "local:".
const SCOPE = "local:";

afterEach(cleanup);

// jsdom has no ResizeObserver and no layout engine, so tests drive the observer
// by hand and stub the two boxes it measures (see `layout` below).
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

  // The component reads the current geometry off the elements rather than out
  // of the entry, so a delivery carries no size of its own.
  emit() {
    act(() => {
      this.callback([] as ResizeObserverEntry[], this as unknown as ResizeObserver);
    });
  }
}

beforeEach(() => {
  localStorage.clear();
  MockResizeObserver.instances = [];
  globalThis.ResizeObserver =
    MockResizeObserver as unknown as typeof ResizeObserver;
});

function observer() {
  const ro = MockResizeObserver.instances[0];
  if (!ro) throw new Error("ComposeBar did not observe its panel");
  return ro;
}

// The overlay is panel height minus spacer height, both border-box. jsdom
// reports 0 for offsetHeight, so tests stand in for the layout engine. The
// numbers used below are the real ones: the spacer rests at 50px (21px row +
// the mirror's 12px py-1.5 + the panel's 16px py-2 + its 1px border-t) and the
// panel grows one 21px row at a time from there.
function layout({ spacer, panel }: { spacer: number; panel: number }) {
  const panelEl = screen.getByTestId("compose-grow-wrapper").parentElement!;
  const spacerEl = panelEl.parentElement!;
  Object.defineProperty(spacerEl, "offsetHeight", {
    configurable: true,
    value: spacer,
  });
  Object.defineProperty(panelEl, "offsetHeight", {
    configurable: true,
    value: panel,
  });
}

// ComposeBar renders FilePicker (Task 2's insertPaths consumer), which pulls
// in the upload mutation (useQueryClient) and the active-node scope
// (useNodeContext) regardless of whether the picker is open. Wrap every
// render with the same fixtures FileBrowser.test.tsx uses so those hooks
// resolve instead of throwing "No QueryClient set" / "must be used within
// NodeProvider".
// draftKey is the id of the tab that owns this bar, and the key its draft is
// stored under. Tests that aren't about persistence get one default key, so
// they read as before.
type ComposeBarTestProps = Omit<
  ComponentProps<typeof ComposeBar>,
  "draftKey"
> & { draftKey?: string };

function renderComposeBar(props: ComposeBarTestProps) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <ComposeBar draftKey="tab-1" {...props} />
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
    // Deliberately under MAX_SLUG_CHARS so this stays a test about naming;
    // truncation has its own test below.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "slack-compose-card",
    });

    expect(textarea().placeholder).toBe("Send to #slack-compose-card");
  });

  it("keeps one placeholder across focus and blur", () => {
    // Replaces the old focused/unfocused swap. Focus is now carried by the
    // card edge brightening alone, so the placeholder must hold still.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "argus",
    });

    expect(textarea().placeholder).toBe("Send to #argus");
    fireEvent.focus(textarea());
    expect(textarea().placeholder).toBe("Send to #argus");
    fireEvent.blur(textarea());
    expect(textarea().placeholder).toBe("Send to #argus");
  });

  it("drops the channel when there is no session, as on a raw-shell tab", () => {
    // No "#": that prefix stands for a specific session everywhere else, and
    // a raw shell has none to name.
    renderComposeBar({ onSend: () => true, connected: true });

    expect(textarea().placeholder).toBe("Send to session");
  });

  it("truncates a long slug rather than letting it clip mid-word at the input edge", () => {
    // A textarea placeholder cannot ellipsize itself. This is a real session
    // name from `argus session ls`, and the one the cap was measured against:
    // in Chrome at 390x844 it renders at 282px against 276px of room when cut
    // to the plan's original 28 chars, so it still clipped. jsdom has no
    // layout engine, so this asserts the length the browser measurement
    // settled on rather than re-deriving it.
    renderComposeBar({
      onSend: () => true,
      connected: true,
      sessionSlug: "review-mike-dp-host-self-registration",
    });

    expect(textarea().placeholder).toBe("Send to #review-mike-dp-host-s…");
    expect(textarea().placeholder.length).toBe("Send to #".length + 22);
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
  it("reports zero while the panel sits at its resting height", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    layout({ spacer: 50, panel: 50 });
    observer().emit();

    // toHaveBeenCalledWith only checks that SOME call matched; pin down the
    // call count too so an implementation that fires onOverlayHeightChange(0)
    // unconditionally (regardless of the measured height) can't pass this in
    // isolation.
    expect(onOverlayHeightChange).toHaveBeenCalledTimes(1);
    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("measures a restored draft's overflow on the very first observation", () => {
    // The regression that came with persisting drafts: the bar can now MOUNT
    // already grown, because useState hydrates a saved multi-line draft before
    // the observer ever runs. The old implementation took its first
    // observation as the collapsed baseline, so a restored three-line draft
    // reported 0 overflow — the terminal stayed unshifted and the card sat on
    // top of live output until that draft was sent. Measuring against the
    // spacer (whose height is the resting height by construction) has no
    // first-observation state to poison.
    saveDraft("local:", "tab-1", "line one\nline two\nline three");
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      draftKey: "tab-1",
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    expect(textarea().value).toBe("line one\nline two\nline three");

    layout({ spacer: 50, panel: 92 }); // three lines: 50 + 2 * 21
    observer().emit();

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(42);
  });

  it("never rebuilds the observer when onOverlayHeightChange's identity churns", () => {
    // If the ResizeObserver effect depended on onOverlayHeightChange, a parent
    // that doesn't memoize its callback (a fresh function identity every
    // render) would tear down and rebuild the observer on every render — churn
    // on the hot path of typing. The component builds the observer exactly
    // once and reads the callback through a ref, so this must hold regardless
    // of how often the parent re-renders with a new callback identity.
    const calls: number[] = [];
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const tree = (onOverlayHeightChange: (height: number) => void) => (
      <QueryClientProvider client={queryClient}>
        <StubNodeProvider>
          <ComposeBar
            draftKey="tab-1"
            onSend={() => true}
            connected={true}
            onOverlayHeightChange={onOverlayHeightChange}
          />
        </StubNodeProvider>
      </QueryClientProvider>
    );

    // A fresh inline callback on the first render, like an unmemoized parent.
    const { rerender } = render(tree((h) => calls.push(h)));

    layout({ spacer: 50, panel: 50 }); // one line
    observer().emit();

    // Force a parent re-render that changes onOverlayHeightChange's identity
    // before any further observation fires.
    rerender(tree((h) => calls.push(h)));

    layout({ spacer: 50, panel: 92 }); // grown to three lines
    observer().emit();

    // Exactly one ResizeObserver should ever be constructed for the life of
    // the component instance, and the later callback must still be the one
    // that receives the measurement.
    expect(MockResizeObserver.instances.length).toBe(1);
    expect(calls).toEqual([0, 42]);
  });

  it("reports the overflow past the spacer as the panel grows", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    layout({ spacer: 50, panel: 50 }); // one line
    observer().emit();
    layout({ spacer: 50, panel: 71 }); // two lines
    observer().emit();
    layout({ spacer: 50, panel: 92 }); // three lines
    observer().emit();

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(42);
  });

  it("never reports a negative height if the panel measures under the spacer", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    layout({ spacer: 50, panel: 30 });
    observer().emit();

    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);
  });

  it("reports zero for a hidden tab, where both boxes measure zero", () => {
    // An inactive tab is mounted inside a display:none ancestor (Workspace
    // hides inactive tabs with `hidden`), and a ResizeObserver fires
    // immediately in that state. Both boxes collapse to 0 together, so the
    // difference is 0 — the truth for a hidden tab — and nothing about that
    // observation can corrupt the measurement taken once it is shown.
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    layout({ spacer: 0, panel: 0 }); // hidden mount
    observer().emit();
    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(0);

    layout({ spacer: 50, panel: 92 }); // shown, three lines
    observer().emit();
    expect(onOverlayHeightChange).toHaveBeenLastCalledWith(42);
  });

  it("returns to zero when the panel shrinks back down to the spacer", () => {
    const onOverlayHeightChange = vi.fn();
    renderComposeBar({
      onSend: () => true,
      connected: true,
      onOverlayHeightChange,
    });

    layout({ spacer: 50, panel: 50 });
    observer().emit();
    layout({ spacer: 50, panel: 92 });
    observer().emit();
    layout({ spacer: 50, panel: 50 });
    observer().emit();

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
    // one row (line-height + the mirror's py-1.5) + the panel's py-2 + its
    // 1px border-t. Only the PANEL's padding grew to un-squish the input row;
    // the MIRROR keeps py-1.5, which is why --compose-max-h below is
    // untouched.
    expect(spacer.style.height).toBe(
      "calc(var(--compose-row-h) + 1.75rem + 1px)",
    );
    expect(wrapper.style.getPropertyValue("--compose-max-h")).toContain(
      "var(--compose-row-h)",
    );

    // The formula's other two terms are class-driven, so pin them too: the
    // 1.75rem is the mirror's py-1.5 (12px) PLUS the panel's py-2 (16px), and
    // the 1px is the panel's border-t. Without these, changing the panel's
    // padding leaves the spacer at its old value while the panel resting
    // height moves — silent drift that every assertion above still passes.
    expect(panel.className.split(" ")).toContain("py-2");
    expect(panel.className.split(" ")).toContain("border-t");
    expect(mirror.className.split(" ")).toContain("py-1.5");

    // The action buttons feed the resting height too, and none of the
    // assertions above would notice if they grew. The panel is `items-end`,
    // so its content height is max(row, button): the row is 33px (21px
    // line-height + the mirror's 12px py-1.5) and h-8 is 32px, so the buttons
    // clear it by ONE pixel. At h-10 the panel becomes 57px against a 50px
    // spacer — the exact permanent overflow this test exists to prevent.
    expect(
      screen.getByRole("button", { name: /attach/i }).className.split(" "),
    ).toContain("h-8");
    expect(
      screen.getByRole("button", { name: /send/i }).className.split(" "),
    ).toContain("h-8");

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

  it("floors the mirror's automatic minimum size, so an unbreakable token cannot stretch the grid column past the card", () => {
    // Regression test for a pasted URL rendering as one clipped line that
    // ran off under the send button instead of wrapping.
    //
    // The mirror is a grid item, so its automatic minimum size is min-width:
    // auto => its min-content width. Per CSS Text 3, the soft wrap
    // opportunities `overflow-wrap: break-word` introduces deliberately do
    // NOT reduce min-content — so a 78-char URL's min-content stayed the
    // whole unbroken token. That floored the single grid column at 406px
    // inside a 280px wrapper (measured in Chrome at 390x844); the textarea is
    // `w-full` of that column, so it laid its text out at 406px and the
    // overflow was clipped by the wrapper's overflow-hidden. The text WAS
    // wrapping — just to a line box wider than the card.
    //
    // The textarea needs no such floor: `overflow-y-auto` already makes its
    // own automatic minimum size zero. The mirror's overflow stays visible
    // precisely so its content can size the row, which is why it, and only
    // it, has to opt out by hand.
    //
    // jsdom has no layout engine, so this pins the class contract; the pixel
    // behaviour was verified in Chrome (see the fix commit).
    renderComposeBar({ onSend: () => true, connected: true });

    const mirror = screen.getByTestId("compose-mirror");
    expect(mirror.className.split(" ")).toContain("min-w-0");
  });

  it("renders the panel as the card's top half", () => {
    renderComposeBar({ onSend: () => true, connected: true });

    const panel = screen.getByTestId("compose-grow-wrapper").parentElement!;
    const spacer = panel.parentElement!;

    // The panel and the toolbar are separate boxes that meet flush; each
    // renders half the card's border.
    const panelClasses = panel.className.split(" ");
    expect(panelClasses).toContain("rounded-t-lg");
    expect(panelClasses).toContain("border-x");
    expect(panelClasses).toContain("inset-x-2");
    expect(panelClasses).toContain("border-[hsl(0_0%_20%)]");
    expect(spacer.className.split(" ")).toContain("mt-1.5");

    // Pin the focused value too: TerminalToolbar.test.tsx already asserts
    // both of its border states, but this file only asserted the resting
    // one. Without this, the two halves could be given different focused
    // colours and every test would still pass — the card would visibly
    // split in two on focus even though each half's own tests are green.
    fireEvent.focus(textarea());
    expect(panel.className.split(" ")).toContain("border-[hsl(0_0%_30%)]");
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

describe("ComposeBar draft persistence", () => {
  it("restores the draft after a remount, so nothing outside the bar can swallow it", () => {
    // The bug this exists for: on a weak network one failed 5s node-summary
    // poll used to unmount the whole workspace — this bar with it — and the
    // half-typed message was gone by the time connectivity returned. The
    // draft is client-side text, so it must outlive the mount.
    const first = renderComposeBar({ draftKey: "tab-1", onSend: () => true, connected: true });
    fireEvent.change(textarea(), { target: { value: "half-typed message" } });
    first.unmount();

    renderComposeBar({ draftKey: "tab-1", onSend: () => true, connected: true });

    expect(textarea().value).toBe("half-typed message");
  });

  it("stores each keystroke as it is typed, so a teardown React never sees loses nothing", () => {
    // The write is immediate rather than deferred. A reload, a tab close, or an
    // OS kill runs no React cleanup, so anything still waiting on a timer would
    // be lost with it — and the worst case is not the last few characters but a
    // CLEARED box, whose pending removal would never land and so resurrect the
    // stale stored text on the next mount.
    //
    // Raw storage, not loadDraft: this has to pin that the text reached
    // localStorage, not merely that the module can hand it back.
    renderComposeBar({ draftKey: "tab-1", onSend: () => true, connected: true });

    fireEvent.change(textarea(), { target: { value: "half-typed message" } });
    expect(localStorage.getItem(draftStorageKey(SCOPE, "tab-1"))).toBe(
      "half-typed message",
    );

    fireEvent.change(textarea(), { target: { value: "" } });
    expect(localStorage.getItem(draftStorageKey(SCOPE, "tab-1"))).toBeNull();
  });

  it("clears the stored draft once the text has actually been sent", () => {
    renderComposeBar({ draftKey: "tab-1", onSend: () => true, connected: true });

    fireEvent.change(textarea(), { target: { value: "run the tests" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(textarea().value).toBe("");
    // A sent message must not come back on the next mount.
    expect(loadDraft(SCOPE, "tab-1")).toBe("");
  });

  it("keeps the stored draft when the send never reached the socket", () => {
    // The in-memory counterpart of this is asserted above; this pins that the
    // failed send also leaves the PERSISTED copy alone, so a remount right
    // after the failed tap still has the text.
    renderComposeBar({ draftKey: "tab-1", onSend: () => false, connected: true });

    fireEvent.change(textarea(), { target: { value: "queued message" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(localStorage.getItem(draftStorageKey(SCOPE, "tab-1"))).toBe(
      "queued message",
    );
  });

  it("keeps each tab's draft to itself", () => {
    saveDraft(SCOPE, "tab-2", "written in the other tab");
    renderComposeBar({ draftKey: "tab-1", onSend: () => true, connected: true });

    expect(textarea().value).toBe("");
    fireEvent.change(textarea(), { target: { value: "written in this one" } });

    expect(loadDraft(SCOPE, "tab-2")).toBe("written in the other tab");
  });

  it("leaves the draft alone when the tab is pointed at a different session", () => {
    // The draft belongs to the box, and the box belongs to the tab. Attaching
    // another session changes where a send goes, but the tab is still open and
    // the half-typed text is still the user's — so it stays exactly where it
    // is, and the placeholder is the only thing that moves.
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const tree = (sessionSlug: string) => (
      <QueryClientProvider client={queryClient}>
        <StubNodeProvider>
          <ComposeBar
            draftKey="tab-1"
            sessionSlug={sessionSlug}
            onSend={() => true}
            connected={true}
          />
        </StubNodeProvider>
      </QueryClientProvider>
    );

    const { rerender } = render(tree("first-agent"));
    fireEvent.change(textarea(), { target: { value: "half-typed message" } });

    rerender(tree("second-agent"));

    expect(textarea().value).toBe("half-typed message");
    expect(screen.getByPlaceholderText("Send to #second-agent")).toBeTruthy();
  });

  it("keeps two mounted tabs' drafts independent", () => {
    // Workspace hides inactive tabs rather than unmounting them, so several
    // bars are mounted at once. Keyed by tab, none of them can address another
    // tab's draft — so no bar can go stale behind another's write, and none of
    // the machinery that would take to fix has to exist.
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    render(
      <QueryClientProvider client={queryClient}>
        <StubNodeProvider>
          <div data-testid="tab-a">
            <ComposeBar draftKey="tab-1" onSend={() => true} connected={true} />
          </div>
          <div data-testid="tab-b">
            <ComposeBar draftKey="tab-2" onSend={() => true} connected={true} />
          </div>
        </StubNodeProvider>
      </QueryClientProvider>,
    );

    const boxIn = (tab: "a" | "b") =>
      screen
        .getByTestId(`tab-${tab}`)
        .querySelector("textarea") as HTMLTextAreaElement;

    fireEvent.change(boxIn("a"), { target: { value: "written in tab one" } });
    fireEvent.change(boxIn("b"), { target: { value: "written in tab two" } });

    expect(boxIn("a").value).toBe("written in tab one");
    expect(boxIn("b").value).toBe("written in tab two");
    expect(localStorage.getItem(draftStorageKey(SCOPE, "tab-1"))).toBe(
      "written in tab one",
    );
    expect(localStorage.getItem(draftStorageKey(SCOPE, "tab-2"))).toBe(
      "written in tab two",
    );
  });
});
