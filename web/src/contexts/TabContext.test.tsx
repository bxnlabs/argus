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
  render(
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
});
