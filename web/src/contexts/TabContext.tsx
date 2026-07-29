import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  type ReactNode,
} from "react";
import {
  type TabState,
  type TabData,
  createInitialTabState,
  createTab,
  saveTabState,
  loadTabState,
} from "../lib/tabs";
import { useViewport } from "../hooks/useViewport";

// ---------------------------------------------------------------------------
// Context value
// ---------------------------------------------------------------------------

interface TabContextValue {
  tabs: TabData[];
  activeTabId: string;
  activeTab: TabData | null;
  isMobile: boolean;

  // Tab management
  addTab: () => void;
  closeTab: (tabId: string) => void;
  switchTab: (tabId: string) => void;

  // Session management (operates on the active tab)
  attachSession: (sessionId: string) => void;
  detachSession: () => void;
  detachSessionById: (sessionId: string) => void;

  // Session management (targeted — for work that resolves asynchronously)
  attachSessionToTab: (
    tabId: string,
    sessionId: string,
    expected: string | null,
  ) => boolean;
}

const TabContext = createContext<TabContextValue | null>(null);

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function TabProvider({
  nodeScope,
  children,
}: {
  // Scopes persisted tab state to the active node's id+origin. The provider is
  // keyed on this same scope in App, so nodeScope is stable for a given mount.
  nodeScope: string;
  children: ReactNode;
}) {
  const [state, setState] = useState<TabState>(createInitialTabState);
  // Mirrors `state`, kept in lockstep by every writer via `applyTabState`
  // below. `attachSessionToTab` has to decide whether its target is still
  // valid *and report that back*, which a setState updater cannot do — it
  // needs a synchronous read that is never stale, even for another writer's
  // update landing in the same tick with no render between them.
  const stateRef = useRef(state);
  const [hydrated, setHydrated] = useState(false);
  const { isMobile } = useViewport();

  // Whether this provider is still the live one. App keys TabProvider on the
  // node scope, so switching nodes unmounts this instance while async work it
  // started keeps running against these closures. `stateRef` stays readable and
  // still matches, so the attach guard below would pass and `setState` would
  // no-op — reporting success for a write nobody will ever see, and which the
  // persist effect (already torn down) will never save either.
  const mountedRef = useRef(true);
  useEffect(() => {
    // Set on the way in, not just cleared on the way out: StrictMode's
    // double-invoke runs the cleanup before re-running this effect.
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  // Single writer for tab state: updates the mirror and React state with the
  // same result, so `stateRef` is never stale for a synchronous read that
  // happens before the next render. `attachSessionToTab` depends on that.
  const applyTabState = useCallback((update: (prev: TabState) => TabState) => {
    const next = update(stateRef.current);
    if (next === stateRef.current) return;
    stateRef.current = next;
    setState(next);
  }, []);

  // ------ Load this node's persisted state from localStorage on mount ------
  useEffect(() => {
    const saved = loadTabState(nodeScope);
    if (saved) applyTabState(() => saved);
    setHydrated(true);
  }, [nodeScope, applyTabState]);

  // ------ Persist to this node's key on every state change ------
  useEffect(() => {
    if (hydrated) saveTabState(state, nodeScope);
  }, [state, hydrated, nodeScope]);

  // ------ Tab management ------

  const addTab = useCallback(() => {
    applyTabState((prev) => {
      const newTab = createTab();
      return {
        tabs: [...prev.tabs, newTab],
        activeTabId: newTab.id,
      };
    });
  }, [applyTabState]);

  const closeTab = useCallback(
    (tabId: string) => {
      applyTabState((prev) => {
        if (prev.tabs.length <= 1) return prev; // keep at least one tab

        const closingIndex = prev.tabs.findIndex((t) => t.id === tabId);
        const newTabs = prev.tabs.filter((t) => t.id !== tabId);
        const newActiveTabId =
          prev.activeTabId === tabId
            ? newTabs[Math.min(closingIndex, newTabs.length - 1)].id
            : prev.activeTabId;

        return { tabs: newTabs, activeTabId: newActiveTabId };
      });
    },
    [applyTabState],
  );

  const switchTab = useCallback(
    (tabId: string) => {
      applyTabState((prev) => ({ ...prev, activeTabId: tabId }));
    },
    [applyTabState],
  );

  // ------ Session attach / detach (on active tab) ------

  const attachSession = useCallback(
    (sessionId: string) => {
      applyTabState((prev) => ({
        ...prev,
        tabs: prev.tabs.map((tab) =>
          tab.id === prev.activeTabId
            ? { ...tab, sessionId }
            : tab,
        ),
      }));
    },
    [applyTabState],
  );

  const detachSession = useCallback(() => {
    applyTabState((prev) => ({
      ...prev,
      tabs: prev.tabs.map((tab) =>
        tab.id === prev.activeTabId
          ? { ...tab, sessionId: null }
          : tab,
      ),
    }));
  }, [applyTabState]);

  const detachSessionById = useCallback(
    (sessionId: string) => {
      applyTabState((prev) => {
        let anyChanged = false;
        const newTabs = prev.tabs.map((tab) => {
          if (tab.sessionId === sessionId) {
            anyChanged = true;
            return { ...tab, sessionId: null };
          }
          return tab;
        });
        return anyChanged ? { ...prev, tabs: newTabs } : prev;
      });
    },
    [applyTabState],
  );

  // Attach to a specific tab, but only if it still holds what it held when the
  // caller snapshotted it. Work that resolves asynchronously (create, clone)
  // otherwise attaches to whatever tab is active at *completion*, landing the
  // session wherever the user has since moved. Returns whether it attached, so
  // the caller can say something instead of failing silently.
  const attachSessionToTab = useCallback(
    (tabId: string, sessionId: string, expected: string | null): boolean => {
      // Nothing this provider writes can reach the user any more, so an attach
      // is not something it can honestly claim to have done.
      if (!mountedRef.current) return false;

      const tab = stateRef.current.tabs.find((t) => t.id === tabId);
      if (!tab || tab.sessionId !== expected) return false;

      // Routed through `applyTabState`, whose mirror write-through is what
      // makes this safe: two calls landing in one tick would otherwise both
      // read the pre-update snapshot, both pass the guard above, and both
      // return true — while the second write silently replaced the first.
      applyTabState((prev) => ({
        ...prev,
        tabs: prev.tabs.map((t) =>
          t.id === tabId ? { ...t, sessionId } : t,
        ),
      }));
      return true;
    },
    [applyTabState],
  );

  // ------ Active tab ------

  const activeTab =
    state.tabs.find((t) => t.id === state.activeTabId) ?? null;

  return (
    <TabContext.Provider
      value={{
        tabs: state.tabs,
        activeTabId: state.activeTabId,
        activeTab,
        isMobile,
        addTab,
        closeTab,
        switchTab,
        attachSession,
        attachSessionToTab,
        detachSession,
        detachSessionById,
      }}
    >
      {children}
    </TabContext.Provider>
  );
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useTabs(): TabContextValue {
  const context = useContext(TabContext);
  if (!context) {
    throw new Error("useTabs must be used within a TabProvider");
  }
  return context;
}
