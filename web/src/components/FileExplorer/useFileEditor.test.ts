import { describe, it, expect } from "vitest";
import { renderHook, act, cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
import { useFileEditor } from "./useFileEditor";

afterEach(cleanup);

describe("useFileEditor", () => {
  // Bookkeeping only: content comes from the query now, so opening a file
  // must not fetch anything or need awaiting.
  it("tracks open paths and the active one without loading content", () => {
    const { result } = renderHook(() => useFileEditor());

    act(() => result.current.openFile("/repo/a.ts"));
    expect(result.current.openPaths).toEqual(["/repo/a.ts"]);
    expect(result.current.activeFilePath).toBe("/repo/a.ts");

    act(() => result.current.openFile("/repo/b.ts"));
    expect(result.current.openPaths).toEqual(["/repo/a.ts", "/repo/b.ts"]);
    expect(result.current.activeFilePath).toBe("/repo/b.ts");
  });

  it("re-opening a path focuses it instead of duplicating it", () => {
    const { result } = renderHook(() => useFileEditor());
    act(() => result.current.openFile("/repo/a.ts"));
    act(() => result.current.openFile("/repo/b.ts"));

    act(() => result.current.openFile("/repo/a.ts"));

    expect(result.current.openPaths).toEqual(["/repo/a.ts", "/repo/b.ts"]);
    expect(result.current.activeFilePath).toBe("/repo/a.ts");
  });

  it("closing the active file falls back to a neighbour", () => {
    const { result } = renderHook(() => useFileEditor());
    act(() => result.current.openFile("/repo/a.ts"));
    act(() => result.current.openFile("/repo/b.ts"));

    act(() => result.current.closeFile("/repo/b.ts"));

    expect(result.current.openPaths).toEqual(["/repo/a.ts"]);
    expect(result.current.activeFilePath).toBe("/repo/a.ts");
  });

  it("closing the first of several open tabs activates its right neighbour", () => {
    const { result } = renderHook(() => useFileEditor());
    act(() => result.current.openFile("/repo/a.ts"));
    act(() => result.current.openFile("/repo/b.ts"));
    act(() => result.current.openFile("/repo/c.ts"));
    act(() => result.current.setActiveFile("/repo/a.ts"));

    act(() => result.current.closeFile("/repo/a.ts"));

    expect(result.current.openPaths).toEqual(["/repo/b.ts", "/repo/c.ts"]);
    expect(result.current.activeFilePath).toBe("/repo/b.ts");
  });

  it("closing the last file leaves nothing active", () => {
    const { result } = renderHook(() => useFileEditor());
    act(() => result.current.openFile("/repo/a.ts"));

    act(() => result.current.closeFile("/repo/a.ts"));

    expect(result.current.openPaths).toEqual([]);
    expect(result.current.activeFilePath).toBeNull();
  });
});
