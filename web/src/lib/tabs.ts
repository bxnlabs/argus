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

// Tab state is scoped per node: each node has its own tabs/sessions, and the
// TabProvider remounts on node switch. A single shared key would let the new
// node's provider load, reconcile (dropping the other node's sessions), and
// save over the previous node's tabs — losing them on switch-back. The scope
// includes the node's origin (not just its id) so editing a manual node's URL
// — same id, different origin — gets fresh tabs instead of session ids that
// only exist on the old origin.
function tabStateKey(nodeScope: string): string {
  return `argus-tab-state:${nodeScope}`;
}

export function saveTabState(state: TabState, nodeScope: string): void {
  try {
    localStorage.setItem(tabStateKey(nodeScope), JSON.stringify(state));
  } catch {
    // localStorage may be unavailable
  }
}

export function loadTabState(nodeScope: string): TabState | null {
  try {
    const saved = localStorage.getItem(tabStateKey(nodeScope));
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
