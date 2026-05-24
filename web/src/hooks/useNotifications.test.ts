import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, act } from "@testing-library/react";

vi.mock("sonner", () => ({
  toast: { info: vi.fn() },
}));

import { toast } from "sonner";
import { useNotifications } from "./useNotifications";

const infoMock = toast.info as unknown as ReturnType<typeof vi.fn>;

beforeEach(() => {
  infoMock.mockClear();
});

afterEach(() => {
  vi.restoreAllMocks();
});

function read(id: string) {
  return { id, name: id.toUpperCase(), status: "idle", unreadSince: null as string | null };
}

function unread(id: string) {
  return { id, name: id.toUpperCase(), status: "idle", unreadSince: "2026-05-23 12:00:00" };
}

describe("useNotifications", () => {
  it("notifies when a non-active session newly becomes unread", () => {
    const { result } = renderHook(() => useNotifications());

    // First call seeds the baseline (no notifications on init).
    act(() => result.current.checkStateChanges([read("s1")], null));
    // Session transitions read -> unread.
    act(() => result.current.checkStateChanges([unread("s1")], null));

    expect(infoMock).toHaveBeenCalledTimes(1);
    expect(infoMock).toHaveBeenCalledWith("S1 finished working");
  });

  it("does not notify for the active session", () => {
    const { result } = renderHook(() => useNotifications());

    act(() => result.current.checkStateChanges([read("s1")], "s1"));
    act(() => result.current.checkStateChanges([unread("s1")], "s1"));

    expect(infoMock).not.toHaveBeenCalled();
  });
});
