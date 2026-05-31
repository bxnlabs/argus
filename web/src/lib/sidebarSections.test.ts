import { describe, it, expect, beforeEach } from "vitest";
import {
  DEFAULT_COLLAPSE_STATE,
  toggleSection,
  loadSectionCollapse,
  saveSectionCollapse,
  type SectionCollapseState,
} from "./sidebarSections";

beforeEach(() => {
  localStorage.clear();
});

describe("DEFAULT_COLLAPSE_STATE", () => {
  it("starts with both sections expanded (not collapsed)", () => {
    expect(DEFAULT_COLLAPSE_STATE).toEqual({ pinned: false, recents: false });
  });
});

describe("toggleSection", () => {
  it("flips the targeted section and leaves the other untouched", () => {
    const start: SectionCollapseState = { pinned: false, recents: false };
    const afterPinned = toggleSection(start, "pinned");
    expect(afterPinned).toEqual({ pinned: true, recents: false });

    const afterRecents = toggleSection(afterPinned, "recents");
    expect(afterRecents).toEqual({ pinned: true, recents: true });
  });

  it("returns a new object (does not mutate the input)", () => {
    const start: SectionCollapseState = { pinned: false, recents: false };
    const next = toggleSection(start, "pinned");
    expect(next).not.toBe(start);
    expect(start).toEqual({ pinned: false, recents: false });
  });
});
