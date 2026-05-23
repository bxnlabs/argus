import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useScrollToFileCorrection } from "./useScrollToFileCorrection";

type Params = Parameters<typeof useScrollToFileCorrection>[0];

function rect(top: number): DOMRect {
  return {
    top,
    bottom: top,
    left: 0,
    right: 0,
    width: 0,
    height: 0,
    x: 0,
    y: top,
    toJSON: () => ({}),
  } as DOMRect;
}

// Pane is fixed at viewport top=100. The target sits `docTop` below the content
// origin and moves up as the pane scrolls, mimicking real scroll geometry so
// the correction converges (delta -> 0) instead of running away.
function makePane(): HTMLElement {
  const pane = document.createElement("div");
  pane.getBoundingClientRect = () => rect(100);
  return pane;
}
function makeTarget(docTop: number, pane: HTMLElement, connected = true): HTMLElement {
  const t = document.createElement("div");
  t.getBoundingClientRect = () => rect(docTop - pane.scrollTop);
  if (connected) document.body.appendChild(t);
  return t;
}

// Deterministic rAF + clock so we can step frames by hand.
let frames: Array<{ id: number; cb: FrameRequestCallback }>;
let nextId: number;
let now: number;

beforeEach(() => {
  frames = [];
  nextId = 1;
  now = 0;
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
    const id = nextId++;
    frames.push({ id, cb });
    return id;
  });
  vi.stubGlobal("cancelAnimationFrame", (id: number) => {
    frames = frames.filter((f) => f.id !== id);
  });
  vi.spyOn(performance, "now").mockImplementation(() => now);
});

afterEach(() => {
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function runFrame() {
  const batch = frames;
  frames = [];
  for (const f of batch) f.cb(now);
}

function render(props: Params) {
  return renderHook((p: Params) => useScrollToFileCorrection(p), {
    initialProps: props,
  });
}

function baseProps(over: Partial<Params>): Params {
  return {
    scrollContainerRef: { current: null },
    getTarget: () => null,
    selectedPath: "a.txt",
    isMobile: false,
    mobileShowDiffs: false,
    requestId: 1,
    ...over,
  };
}

describe("useScrollToFileCorrection", () => {
  it("aligns the target to the pane top, then holds steady", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    render(baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target }));

    runFrame(); // delta = 600 - 100 = 500
    expect(pane.scrollTop).toBe(500);

    runFrame(); // target now at 600-500=100 => delta 0
    expect(pane.scrollTop).toBe(500);
  });

  it("does not scroll a detached target", () => {
    const pane = makePane();
    const target = makeTarget(600, pane, /* connected */ false);
    render(baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target }));

    runFrame();
    expect(pane.scrollTop).toBe(0);
  });

  it("is inert when there is no selected path or no pane", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    render(baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target, selectedPath: null }));
    expect(frames.length).toBe(0);

    render(baseProps({ scrollContainerRef: { current: null }, getTarget: () => target }));
    expect(frames.length).toBe(0);
  });

  it("does not run on mobile until the diff view is shown", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    const { rerender } = render(
      baseProps({
        scrollContainerRef: { current: pane },
        getTarget: () => target,
        isMobile: true,
        mobileShowDiffs: false,
      }),
    );
    expect(frames.length).toBe(0);

    rerender(
      baseProps({
        scrollContainerRef: { current: pane },
        getTarget: () => target,
        isMobile: true,
        mobileShowDiffs: true,
        requestId: 2,
      }),
    );
    runFrame();
    expect(pane.scrollTop).toBe(500);
  });

  it("stops correcting on user scroll input (wheel)", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    render(baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target }));

    runFrame(); // scrollTop -> 500, reschedules next frame
    expect(pane.scrollTop).toBe(500);
    expect(frames.length).toBe(1);

    pane.dispatchEvent(new Event("wheel"));
    expect(frames.length).toBe(0); // pending frame cancelled

    runFrame();
    expect(pane.scrollTop).toBe(500); // no further writes
  });

  it("stops correcting on window keydown", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    render(baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target }));

    runFrame();
    expect(pane.scrollTop).toBe(500);

    window.dispatchEvent(new Event("keydown"));
    expect(frames.length).toBe(0);
  });

  it("cancels the loop and removes listeners on unmount", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    const removeSpy = vi.spyOn(pane, "removeEventListener");
    const windowRemoveSpy = vi.spyOn(window, "removeEventListener");
    const { unmount } = render(
      baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target }),
    );
    expect(frames.length).toBe(1); // initial frame queued

    unmount();
    expect(frames.length).toBe(0); // rAF cancelled
    expect(removeSpy).toHaveBeenCalledWith("wheel", expect.any(Function));
    expect(removeSpy).toHaveBeenCalledWith("touchstart", expect.any(Function));
    expect(windowRemoveSpy).toHaveBeenCalledWith("keydown", expect.any(Function));
  });

  it("stops scheduling frames once the correction window elapses", () => {
    const pane = makePane();
    const target = makeTarget(600, pane);
    render(baseProps({ scrollContainerRef: { current: pane }, getTarget: () => target }));

    now = 500; // past the default 400ms window (deadline was 0+400)
    runFrame(); // aligns once more, but does not reschedule
    expect(pane.scrollTop).toBe(500);
    expect(frames.length).toBe(0);
  });

  it("re-runs for a new request and targets the new file", () => {
    const pane = makePane();
    const targetA = makeTarget(600, pane);
    const targetB = makeTarget(1100, pane);
    const { rerender } = render(
      baseProps({ scrollContainerRef: { current: pane }, getTarget: () => targetA, requestId: 1 }),
    );

    runFrame(); // converge toward A
    expect(pane.scrollTop).toBe(500);

    rerender(
      baseProps({
        scrollContainerRef: { current: pane },
        getTarget: () => targetB,
        selectedPath: "b.txt",
        requestId: 2,
      }),
    );
    runFrame(); // B at 1100-500=600 => delta 500 => scrollTop 1000
    expect(pane.scrollTop).toBe(1000);

    runFrame(); // B at 1100-1000=100 => delta 0
    expect(pane.scrollTop).toBe(1000);
  });
});
