import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
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
}

const TabContext = createContext<TabContextValue | null>(null);

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

export function TabProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<TabState>(createInitialTabState);
  const [hydrated, setHydrated] = useState(false);
  const { isMobile } = useViewport();

  // ------ Load persisted state from localStorage on mount ------
  useEffect(() => {
    const saved = loadTabState();
    if (saved) setState(saved);
    setHydrated(true);
  }, []);

  // ------ Persist to localStorage on every state change ------
  useEffect(() => {
    if (hydrated) saveTabState(state);
  }, [state, hydrated]);

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
