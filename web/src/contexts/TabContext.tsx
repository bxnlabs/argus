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
  // Mirrors `state` for synchronous reads. `attachSessionToTab` has to decide
  // whether its target is still valid *and report that back*, which a setState
  // updater cannot do.
  const stateRef = useRef(state);
  stateRef.current = state;
  const [hydrated, setHydrated] = useState(false);
  const { isMobile } = useViewport();

  // ------ Load this node's persisted state from localStorage on mount ------
  useEffect(() => {
    const saved = loadTabState(nodeScope);
    if (saved) setState(saved);
    setHydrated(true);
  }, [nodeScope]);

  // ------ Persist to this node's key on every state change ------
  useEffect(() => {
    if (hydrated) saveTabState(state, nodeScope);
  }, [state, hydrated, nodeScope]);

  // ------ Tab management ------

  const addTab = useCallback(() => {
    setState((prev) => {
      const newTab = createTab();
      return {
        tabs: [...prev.tabs, newTab],
        activeTabId: newTab.id,
      };
    });
  }, []);

  const closeTab = useCallback((tabId: string) => {
    setState((prev) => {
      if (prev.tabs.length <= 1) return prev; // keep at least one tab

      const closingIndex = prev.tabs.findIndex((t) => t.id === tabId);
      const newTabs = prev.tabs.filter((t) => t.id !== tabId);
      const newActiveTabId =
        prev.activeTabId === tabId
          ? newTabs[Math.min(closingIndex, newTabs.length - 1)].id
          : prev.activeTabId;

      return { tabs: newTabs, activeTabId: newActiveTabId };
    });
  }, []);

  const switchTab = useCallback((tabId: string) => {
    setState((prev) => ({ ...prev, activeTabId: tabId }));
  }, []);

  // ------ Session attach / detach (on active tab) ------

  const attachSession = useCallback(
    (sessionId: string) => {
      setState((prev) => ({
        ...prev,
        tabs: prev.tabs.map((tab) =>
          tab.id === prev.activeTabId
            ? { ...tab, sessionId }
            : tab,
        ),
      }));
    },
    [],
  );

  const detachSession = useCallback(() => {
    setState((prev) => ({
      ...prev,
      tabs: prev.tabs.map((tab) =>
        tab.id === prev.activeTabId
          ? { ...tab, sessionId: null }
          : tab,
      ),
    }));
  }, []);

  const detachSessionById = useCallback((sessionId: string) => {
    setState((prev) => {
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
  }, []);

  // Attach to a specific tab, but only if it still holds what it held when the
  // caller snapshotted it. Work that resolves asynchronously (create, clone)
  // otherwise attaches to whatever tab is active at *completion*, landing the
  // session wherever the user has since moved. Returns whether it attached, so
  // the caller can say something instead of failing silently.
  const attachSessionToTab = useCallback(
    (tabId: string, sessionId: string, expected: string | null): boolean => {
      const tab = stateRef.current.tabs.find((t) => t.id === tabId);
      if (!tab || tab.sessionId !== expected) return false;

      // Write through the mirror before queueing the update. React refreshes
      // `stateRef` only on render, so two calls landing in one tick would
      // otherwise both read the pre-update snapshot, both pass the guard, and
      // both return true — while the second write silently replaced the first.
      const tabs = stateRef.current.tabs.map((t) =>
        t.id === tabId ? { ...t, sessionId } : t,
      );
      stateRef.current = { ...stateRef.current, tabs };

      setState((prev) => ({
        ...prev,
        tabs: prev.tabs.map((t) =>
          t.id === tabId ? { ...t, sessionId } : t,
        ),
      }));
      return true;
    },
    [],
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
