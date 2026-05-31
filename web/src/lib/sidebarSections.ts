// Collapsed/expanded state for the session sidebar's "Pinned" and "Recents"
// sections, persisted to localStorage. Mirrors the per-feature module pattern
// used by ./tabs.ts. A value of `true` means the section is collapsed.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type SidebarSectionKey = "pinned" | "recents";

export interface SectionCollapseState {
  pinned: boolean;
  recents: boolean;
}

export const DEFAULT_COLLAPSE_STATE: SectionCollapseState = {
  pinned: false,
  recents: false,
};

// ---------------------------------------------------------------------------
// Reducer
// ---------------------------------------------------------------------------

// Return a new state with the given section's collapsed flag flipped.
export function toggleSection(
  state: SectionCollapseState,
  key: SidebarSectionKey,
): SectionCollapseState {
  return { ...state, [key]: !state[key] };
}

// ---------------------------------------------------------------------------
// localStorage persistence
// ---------------------------------------------------------------------------

const SECTION_COLLAPSE_KEY = "argus-sidebar-section-collapse";

export function saveSectionCollapse(state: SectionCollapseState): void {
  try {
    localStorage.setItem(SECTION_COLLAPSE_KEY, JSON.stringify(state));
  } catch {
    // localStorage may be unavailable
  }
}

export function loadSectionCollapse(): SectionCollapseState {
  try {
    const saved = localStorage.getItem(SECTION_COLLAPSE_KEY);
    if (!saved) return { ...DEFAULT_COLLAPSE_STATE };

    const parsed = JSON.parse(saved);
    if (typeof parsed !== "object" || parsed === null) {
      return { ...DEFAULT_COLLAPSE_STATE };
    }

    return {
      pinned:
        typeof parsed.pinned === "boolean"
          ? parsed.pinned
          : DEFAULT_COLLAPSE_STATE.pinned,
      recents:
        typeof parsed.recents === "boolean"
          ? parsed.recents
          : DEFAULT_COLLAPSE_STATE.recents,
    };
  } catch {
    // localStorage may be unavailable or data corrupt
  }
  return { ...DEFAULT_COLLAPSE_STATE };
}
