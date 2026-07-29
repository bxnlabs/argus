import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { render, act, cleanup } from "@testing-library/react";
import { TabProvider, useTabs } from "./TabContext";

vi.mock("@/hooks/useViewport", () => ({
  useViewport: () => ({ isMobile: false, isDesktop: true, isHydrated: true }),
}));

// The provider's value, refreshed on every render so assertions read current
// state and calls run against current state.
let ctx!: ReturnType<typeof useTabs>;

function Probe() {
  ctx = useTabs();
  return <span data-testid="tabs">{JSON.stringify(ctx.tabs)}</span>;
}

function renderTabs() {
  return render(
    <TabProvider nodeScope="test-scope">
      <Probe />
    </TabProvider>,
  );
}

function tabById(id: string) {
  return ctx.tabs.find((t) => t.id === id);
}

beforeEach(() => localStorage.clear());
afterEach(cleanup);

describe("attachSessionToTab", () => {
  it("attaches to the snapshotted tab and reports that it attached", async () => {
    renderTabs();
    const target = ctx.activeTabId;

    let attached!: boolean;
    await act(async () => {
      attached = ctx.attachSessionToTab(target, "sess-1", null);
    });

    expect(attached).toBe(true);
    expect(tabById(target)?.sessionId).toBe("sess-1");
  });

  // The tab that started the work is gone by the time it finishes. Attaching
  // anywhere else would be the hijack this exists to prevent.
  it("does not attach when the snapshotted tab has been closed", async () => {
    renderTabs();
    const target = ctx.activeTabId;
    await act(async () => ctx.addTab());
    await act(async () => ctx.closeTab(target));

    let attached!: boolean;
    await act(async () => {
      attached = ctx.attachSessionToTab(target, "sess-1", null);
    });

    expect(attached).toBe(false);
    expect(ctx.tabs.some((t) => t.sessionId === "sess-1")).toBe(false);
  });

  // The tab is still open but the user put something else in it. Overwriting
  // would trade one hijack for another.
  it("does not attach when the snapshotted tab holds a different session", async () => {
    renderTabs();
    const target = ctx.activeTabId;
    await act(async () => ctx.attachSession("other-session"));

    let attached!: boolean;
    await act(async () => {
      attached = ctx.attachSessionToTab(target, "sess-1", null);
    });

    expect(attached).toBe(false);
    expect(tabById(target)?.sessionId).toBe("other-session");
  });

  it("matches a non-null expected session and rejects a changed one", async () => {
    renderTabs();
    const target = ctx.activeTabId;
    await act(async () => ctx.attachSession("sess-old"));

    let attached!: boolean;
    await act(async () => {
      attached = ctx.attachSessionToTab(target, "sess-new", "sess-old");
    });
    expect(attached).toBe(true);
    expect(tabById(target)?.sessionId).toBe("sess-new");

    // The same snapshot is now stale and must not apply a second time.
    await act(async () => {
      attached = ctx.attachSessionToTab(target, "sess-other", "sess-old");
    });
    expect(attached).toBe(false);
    expect(tabById(target)?.sessionId).toBe("sess-new");
  });

  it("leaves other tabs alone and does not switch the active tab", async () => {
    renderTabs();
    const target = ctx.activeTabId;
    await act(async () => ctx.addTab());
    const other = ctx.activeTabId;

    await act(async () => {
      ctx.attachSessionToTab(target, "sess-1", null);
    });

    expect(tabById(target)?.sessionId).toBe("sess-1");
    expect(tabById(other)?.sessionId).toBeNull();
    expect(ctx.activeTabId).toBe(other);
  });

  // Two completions landing in one tick, with no render between them, is the
  // case the mirror exists for: without a write-through the second call reads
  // a stale snapshot, passes the guard, and clobbers the first write while
  // both callers are told they attached.
  it("rejects a second same-tick call against the same snapshot", async () => {
    renderTabs();
    const target = ctx.activeTabId;

    let first!: boolean;
    let second!: boolean;
    await act(async () => {
      first = ctx.attachSessionToTab(target, "sess-1", null);
      second = ctx.attachSessionToTab(target, "sess-2", null);
    });

    expect(first).toBe(true);
    expect(second).toBe(false);
    expect(tabById(target)?.sessionId).toBe("sess-1");
  });

  // The mirror has to stay fresh for every mutator, not just
  // attachSessionToTab's own writes. Close a tab and, in the same tick with no
  // intervening render, target it with attachSessionToTab: a stale mirror
  // would still see the pre-close tab, pass the guard, and report success —
  // while the tab it "attached" to is actually gone from the committed state.
  it("sees a tab closed by a different mutator in the same tick", async () => {
    renderTabs();
    const closingTarget = ctx.activeTabId;
    await act(async () => ctx.addTab());

    let attached!: boolean;
    await act(async () => {
      ctx.closeTab(closingTarget);
      attached = ctx.attachSessionToTab(closingTarget, "sess-1", null);
    });

    expect(attached).toBe(false);
    expect(ctx.tabs.some((t) => t.id === closingTarget)).toBe(false);
  });

  // App keys TabProvider on the node scope, so switching nodes unmounts this
  // instance mid-flight. The guard above still passes against the mirror and
  // setState no-ops, so without a liveness check the caller is told it attached
  // — while the write reaches no render and the persist effect, torn down with
  // the provider, never saves it either. The caller needs the false so it can
  // fall back to a toast instead of completing in silence.
  it("does not attach once the provider has unmounted", async () => {
    const { unmount } = renderTabs();
    const target = ctx.activeTabId;
    unmount();

    let attached!: boolean;
    await act(async () => {
      attached = ctx.attachSessionToTab(target, "sess-1", null);
    });

    expect(attached).toBe(false);
    expect(JSON.stringify(localStorage)).not.toContain("sess-1");
  });
});
