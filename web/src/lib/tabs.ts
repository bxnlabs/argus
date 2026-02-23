// Tab state types and helpers for Argus v2 SPA.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface TabData {
  id: string;
  sessionId: string | null;
}

export interface TabState {
  tabs: TabData[];
  activeTabId: string;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

export function generateTabId(): string {
  return `tab-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
}

export function createTab(): TabData {
  return {
    id: generateTabId(),
    sessionId: null,
  };
}

export function createInitialTabState(): TabState {
  const tab = createTab();
  return {
    tabs: [tab],
    activeTabId: tab.id,
  };
}

// ---------------------------------------------------------------------------
// localStorage persistence
// ---------------------------------------------------------------------------

const TAB_STATE_KEY = "argus-tab-state";

export function saveTabState(state: TabState): void {
  try {
    localStorage.setItem(TAB_STATE_KEY, JSON.stringify(state));
  } catch {
    // localStorage may be unavailable
  }
}

export function loadTabState(): TabState | null {
  try {
    const saved = localStorage.getItem(TAB_STATE_KEY);
    if (!saved) return null;

    const parsed = JSON.parse(saved);

    if (typeof parsed !== "object" || parsed === null) {
      return null;
    }

    if (Array.isArray(parsed.tabs) && typeof parsed.activeTabId === "string") {
      return parsed as TabState;
    }

    return null;
  } catch {
    // localStorage may be unavailable or data corrupt
  }
  return null;
}
