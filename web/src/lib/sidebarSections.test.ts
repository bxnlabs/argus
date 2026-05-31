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

describe("saveSectionCollapse / loadSectionCollapse", () => {
  it("round-trips a saved state", () => {
    saveSectionCollapse({ pinned: true, recents: false });
    expect(loadSectionCollapse()).toEqual({ pinned: true, recents: false });
  });

  it("returns the default when nothing is stored", () => {
    expect(loadSectionCollapse()).toEqual(DEFAULT_COLLAPSE_STATE);
  });

  it("returns a fresh object, not the shared default", () => {
    const loaded = loadSectionCollapse();
    expect(loaded).not.toBe(DEFAULT_COLLAPSE_STATE);
  });
});

describe("loadSectionCollapse with bad data", () => {
  it("falls back to default on non-JSON", () => {
    localStorage.setItem("argus-sidebar-section-collapse", "not json{");
    expect(loadSectionCollapse()).toEqual(DEFAULT_COLLAPSE_STATE);
  });

  it("falls back to default on a non-object payload", () => {
    localStorage.setItem("argus-sidebar-section-collapse", "42");
    expect(loadSectionCollapse()).toEqual(DEFAULT_COLLAPSE_STATE);
  });

  it("fills missing or wrong-typed keys from the default", () => {
    localStorage.setItem(
      "argus-sidebar-section-collapse",
      JSON.stringify({ pinned: true }),
    );
    expect(loadSectionCollapse()).toEqual({ pinned: true, recents: false });

    localStorage.setItem(
      "argus-sidebar-section-collapse",
      JSON.stringify({ pinned: "yes", recents: true }),
    );
    expect(loadSectionCollapse()).toEqual({ pinned: false, recents: true });
  });
});
