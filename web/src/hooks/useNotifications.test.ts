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

  it("suppresses the toast for a manually marked-unread session", () => {
    const { result } = renderHook(() => useNotifications());

    act(() => result.current.checkStateChanges([read("s1")], null));
    act(() => result.current.suppressUnreadNotification("s1"));
    act(() => result.current.checkStateChanges([unread("s1")], null));

    expect(infoMock).not.toHaveBeenCalled();
  });

  it("still notifies on a genuine unread after a suppressed one is read again", () => {
    const { result } = renderHook(() => useNotifications());

    act(() => result.current.checkStateChanges([read("s1")], null));
    act(() => result.current.suppressUnreadNotification("s1"));
    act(() => result.current.checkStateChanges([unread("s1")], null)); // suppressed
    act(() => result.current.checkStateChanges([read("s1")], null)); // cleared on read
    act(() => result.current.checkStateChanges([unread("s1")], null)); // genuine

    expect(infoMock).toHaveBeenCalledTimes(1);
  });

  // Guards the intentional asymmetry: a manual suppression registered before the
  // first status baseline must survive the add-only seed and the mark-unread
  // landing. A rebuild-on-init seed would drop it and fire a false toast here.
  it("keeps a pre-baseline manual suppression until an observed read clears it", () => {
    const { result } = renderHook(() => useNotifications());

    // User marks unread before the first status poll establishes the baseline.
    act(() => result.current.suppressUnreadNotification("s1"));
    // First call seeds while the session still reads as read; suppression must survive.
    act(() => result.current.checkStateChanges([read("s1")], null));
    // The mark-unread lands: the unread edge must stay suppressed.
    act(() => result.current.checkStateChanges([unread("s1")], null));
    expect(infoMock).not.toHaveBeenCalled();

    // Once observed read, suppression clears; a later genuine unread notifies once.
    act(() => result.current.checkStateChanges([read("s1")], null));
    act(() => result.current.checkStateChanges([unread("s1")], null));
    expect(infoMock).toHaveBeenCalledTimes(1);
  });
});
