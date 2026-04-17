import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Mock IntersectionObserver before importing the hook
let observerCallback: IntersectionObserverCallback;
let observerInstance: { observe: ReturnType<typeof vi.fn>; unobserve: ReturnType<typeof vi.fn>; disconnect: ReturnType<typeof vi.fn> };

beforeEach(() => {
  observerInstance = {
    observe: vi.fn(),
    unobserve: vi.fn(),
    disconnect: vi.fn(),
  };
  vi.stubGlobal(
    "IntersectionObserver",
    vi.fn(function (callback: IntersectionObserverCallback) {
      observerCallback = callback;
      return observerInstance;
    }),
  );
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

// Dynamic import so the mock is in place
async function getHook() {
  const { useLazyMount } = await import("./useLazyMount");
  const { renderHook } = await import("@testing-library/react");
  return { useLazyMount, renderHook };
}

describe("useLazyMount", () => {
  it("starts unmounted", async () => {
    const { useLazyMount, renderHook } = await getHook();
    const { result } = renderHook(() => useLazyMount());
    expect(result.current.shouldMount).toBe(false);
  });

  it("mounts when intersection is triggered", async () => {
    const { useLazyMount, renderHook } = await getHook();
    const { result } = renderHook(() => useLazyMount());

    // Simulate a ref being set
    const div = document.createElement("div");
    result.current.ref(div);

    // Simulate intersection
    const { act } = await import("@testing-library/react");
    act(() => {
      observerCallback(
        [{ isIntersecting: true, target: div } as unknown as IntersectionObserverEntry],
        observerInstance as unknown as IntersectionObserver,
      );
    });

    expect(result.current.shouldMount).toBe(true);
  });

  it("stays mounted after leaving viewport (sticky)", async () => {
    const { useLazyMount, renderHook } = await getHook();
    const { result } = renderHook(() => useLazyMount());

    const div = document.createElement("div");
    result.current.ref(div);

    const { act } = await import("@testing-library/react");

    // Enter viewport
    act(() => {
      observerCallback(
        [{ isIntersecting: true, target: div } as unknown as IntersectionObserverEntry],
        observerInstance as unknown as IntersectionObserver,
      );
    });
    expect(result.current.shouldMount).toBe(true);

    // Leave viewport — should stay mounted
    act(() => {
      observerCallback(
        [{ isIntersecting: false, target: div } as unknown as IntersectionObserverEntry],
        observerInstance as unknown as IntersectionObserver,
      );
    });
    expect(result.current.shouldMount).toBe(true);
  });
});
