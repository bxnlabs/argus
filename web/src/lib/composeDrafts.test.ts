import { describe, it, expect, beforeEach, vi } from "vitest";
import { loadDraft, saveDraft, draftStorageKey } from "./composeDrafts";

beforeEach(() => {
  localStorage.clear();
});

describe("composeDrafts", () => {
  it("round-trips a draft under its scope and key", () => {
    saveDraft("local:", "sess-1", "run the tests");

    expect(loadDraft("local:", "sess-1")).toBe("run the tests");
  });

  it("returns an empty string for a key that has no draft", () => {
    expect(loadDraft("local:", "never-typed-in")).toBe("");
  });

  it("keeps each node's drafts apart, so switching nodes never surfaces the other's text", () => {
    saveDraft("local:", "sess-1", "local text");
    saveDraft("gpu:http://gpu:80", "sess-1", "remote text");

    expect(loadDraft("local:", "sess-1")).toBe("local text");
    expect(loadDraft("gpu:http://gpu:80", "sess-1")).toBe("remote text");
  });

  it("cannot let one scope's key collide with another's", () => {
    // Both halves of the storage key carry colons of their own — a node scope
    // is "<id>:<url>". Unencoded, these two would address the same entry.
    saveDraft("a", "b:c", "first");
    saveDraft("a:b", "c", "second");

    expect(loadDraft("a", "b:c")).toBe("first");
    expect(loadDraft("a:b", "c")).toBe("second");
  });

  it("drops the entry when the draft is emptied, rather than storing a blank", () => {
    saveDraft("local:", "sess-1", "typed then deleted");
    saveDraft("local:", "sess-1", "");

    expect(loadDraft("local:", "sess-1")).toBe("");
    expect(localStorage.getItem(draftStorageKey("local:", "sess-1"))).toBeNull();
  });

  it("writes only the draft being edited, not its neighbours", () => {
    // The reason for a key per draft rather than one map per scope: under a
    // map, a keystroke re-serialises every other draft in the scope with it.
    // That cost is what would make a per-keystroke write worth deferring, and
    // avoiding it is what lets the write stay immediate.
    saveDraft("local:", "sess-1", "first");
    saveDraft("local:", "sess-2", "second");
    const untouched = localStorage.getItem(draftStorageKey("local:", "sess-1"));

    const setItem = vi.spyOn(localStorage, "setItem");
    saveDraft("local:", "sess-2", "second, edited");

    expect(setItem).toHaveBeenCalledTimes(1);
    expect(setItem.mock.calls[0][0]).toBe(draftStorageKey("local:", "sess-2"));
    expect(localStorage.getItem(draftStorageKey("local:", "sess-1"))).toBe(
      untouched,
    );
    setItem.mockRestore();
  });

  it("survives storage being unavailable", () => {
    // Private mode throws on access. loadDraft is called during render, where
    // a throw would blank the workspace rather than cost a draft.
    const getItem = vi.spyOn(localStorage, "getItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });
    const setItem = vi.spyOn(localStorage, "setItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });

    expect(() => saveDraft("local:", "sess-1", "text")).not.toThrow();
    expect(loadDraft("local:", "sess-1")).toBe("");

    getItem.mockRestore();
    setItem.mockRestore();
  });
});
