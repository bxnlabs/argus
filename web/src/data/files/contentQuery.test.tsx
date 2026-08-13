import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { StubNodeProvider } from "@/test/node-context";
import { apiFetch, ApiError } from "@/api/client";

// Only the fetch helper is stubbed. ApiError stays real: it is the type
// isDefinitiveReadError branches on, so a fake one would make every status
// test pass for the wrong reason.
vi.mock("@/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/client")>()),
  apiFetch: vi.fn(),
}));

const { useFileContentQuery, isDefinitiveReadError } = await import("./queries");

let client: QueryClient;
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider client={client}>
      <StubNodeProvider>{children}</StubNodeProvider>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  // retry: false — an error test would otherwise wait through react-query's
  // default retry/backoff before isError ever flips true.
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  vi.mocked(apiFetch).mockResolvedValue({
    content: "hello",
    size: 5,
    isBinary: false,
    isLarge: false,
    etag: "5-1000",
    path: "/repo/a.ts",
  });
});
afterEach(() => {
  vi.resetAllMocks();
  cleanup();
});

describe("useFileContentQuery", () => {
  it("reads the file in a single request", async () => {
    const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toEqual({
      content: "hello",
      language: "typescript",
      isBinary: false,
      isLarge: false,
      etag: "5-1000",
    });
    // One request is the point: a metadata call and a content call are two
    // looks at a path an agent can rewrite in between, and the pane would act
    // on one version's classification with another version's bytes.
    expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(apiFetch).mock.calls[0][1]).toContain("/api/node/files/content");
    // Nothing to validate against on a first read.
    expect(vi.mocked(apiFetch).mock.calls[0][1]).not.toContain("known=");
  });

  // The poll runs every 30s against a file that has usually not moved, so the
  // round trip that carries no content is the common one.
  describe("polling an unchanged file", () => {
    it("offers the node the version it already holds", async () => {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      await result.current.refetch();

      expect(vi.mocked(apiFetch).mock.calls[1][1]).toContain("known=5-1000");
    });

    it("keeps the content, and the exact object, when the node says unchanged", async () => {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      const first = result.current.data;

      vi.mocked(apiFetch).mockResolvedValue({
        unchanged: true,
        etag: "5-1000",
        path: "/repo/a.ts",
      });
      await result.current.refetch();

      // Identity, not equality: an unchanged poll must not hand Monaco a new
      // object, or it discards cursor and scroll position every 30 seconds.
      expect(result.current.data).toBe(first);
      expect(result.current.data?.content).toBe("hello");
    });

    it("still validates against the held version after an unchanged reply", async () => {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      vi.mocked(apiFetch).mockResolvedValue({
        unchanged: true,
        etag: "5-1000",
        path: "/repo/a.ts",
      });
      await result.current.refetch();
      await result.current.refetch();

      // The etag survives in the returned entry, so the next poll can still
      // ask the cheap question rather than falling back to a full transfer.
      expect(vi.mocked(apiFetch).mock.calls[2][1]).toContain("known=5-1000");
    });

    // `reload` is the escape hatch from the one case the etag cannot see: a
    // rewrite landing on both the same size and the same nanosecond. A reload
    // that validated like a poll would be answered `unchanged` too, and the
    // pane would have no way back to the real bytes at all.
    it("asks unconditionally when the user reloads, then validates again after", async () => {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      await result.current.reload();
      expect(vi.mocked(apiFetch).mock.calls[1][1]).not.toContain("known=");

      // One request, not every request after it: the poll goes back to the
      // cheap question as soon as the reload has been served.
      await result.current.refetch();
      expect(vi.mocked(apiFetch).mock.calls[2][1]).toContain("known=5-1000");
    });

    // A reload is not served until a full body actually arrives. Clearing the
    // flag when the request is *built* lets everything after the first attempt
    // fall back to the validator the reload exists to bypass — and the node
    // then answers `unchanged` against the very bytes the user asked to
    // replace, successfully, with the stale-content banner cleared.
    it("stays unconditional until a forced read actually lands", async () => {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      vi.mocked(apiFetch).mockRejectedValueOnce(new Error("network down"));
      await result.current.reload();
      expect(vi.mocked(apiFetch).mock.calls[1][1]).not.toContain("known=");

      // The reload failed, so the next read is still the one the user asked
      // for — not a poll that can be answered from the collided etag.
      await result.current.refetch();
      expect(vi.mocked(apiFetch).mock.calls[2][1]).not.toContain("known=");

      // ...and once one lands, validation resumes.
      await result.current.refetch();
      expect(vi.mocked(apiFetch).mock.calls[3][1]).toContain("known=5-1000");
    });

    // Production runs `retry: 1` (lib/query-client.ts). React Query serves a
    // retry by invoking queryFn again, so a flag consumed by the first attempt
    // leaves the attempt that actually succeeds sending `known`.
    it("keeps every attempt of a reload unconditional, including the retry", async () => {
      const retrying = new QueryClient({ defaultOptions: { queries: { retry: 1 } } });
      const retryWrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={retrying}>
          <StubNodeProvider>{children}</StubNodeProvider>
        </QueryClientProvider>
      );
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), {
        wrapper: retryWrapper,
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      vi.mocked(apiFetch).mockRejectedValueOnce(new Error("network down"));
      await result.current.reload();

      // Attempt 1 (rejected) and attempt 2 (the retry that succeeds).
      expect(vi.mocked(apiFetch).mock.calls[1][1]).not.toContain("known=");
      expect(vi.mocked(apiFetch).mock.calls[2][1]).not.toContain("known=");
    });

    // The overlap the `if (force)` guard exists for, in the shape that can
    // actually happen: the reload's first attempt fails, and a poll that was
    // already in flight resolves during the retry delay. Unguarded, that poll's
    // response clears the flag, so the retry — the request that finally reaches
    // the node — carries `known` and gets the `unchanged` the reload exists to
    // escape.
    it("does not let a poll in flight clear the flag before a reload's retry", async () => {
      const retrying = new QueryClient({
        defaultOptions: { queries: { retry: 1, retryDelay: 20 } },
      });
      const retryWrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={retrying}>
          <StubNodeProvider>{children}</StubNodeProvider>
        </QueryClientProvider>
      );
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), {
        wrapper: retryWrapper,
      });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));

      // A poll held open, so it can resolve during the retry delay below.
      let releasePoll!: (v: unknown) => void;
      vi.mocked(apiFetch).mockImplementationOnce(
        () => new Promise((resolve) => (releasePoll = resolve)),
      );
      const poll = result.current.refetch();

      // The reload's first attempt fails, putting a retry behind the delay.
      vi.mocked(apiFetch).mockRejectedValueOnce(new Error("network down"));
      const reload = result.current.reload();

      // Land the stale poll inside that window.
      releasePoll({ unchanged: true, etag: "5-1000", path: "/repo/a.ts" });
      await poll;
      await reload;

      const calls = vi.mocked(apiFetch).mock.calls;
      const last = calls[calls.length - 1][1];
      expect(last).not.toContain("known=");
    });

    it("takes the new bytes once the node stops saying unchanged", async () => {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await waitFor(() => expect(result.current.isSuccess).toBe(true));
      // Reading `data` here is load-bearing, not decoration: react-query only
      // re-renders for result properties the consumer has touched, so a test
      // that never reads `data` before the change goes on observing the old
      // value forever. The panes read it while rendering, so they always track
      // it.
      expect(result.current.data?.content).toBe("hello");

      vi.mocked(apiFetch).mockResolvedValue({
        unchanged: true,
        etag: "5-1000",
        path: "/repo/a.ts",
      });
      await result.current.refetch();

      vi.mocked(apiFetch).mockResolvedValue({
        content: "agent wrote this",
        size: 16,
        isBinary: false,
        isLarge: false,
        etag: "16-2000",
        path: "/repo/a.ts",
      });
      await result.current.refetch();

      await waitFor(() => expect(result.current.data?.content).toBe("agent wrote this"));
      expect(result.current.data?.etag).toBe("16-2000");
    });
  });

  // Monaco renders neither, so the node sends no bytes for either.
  it("takes the node's word that a file is binary", async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      content: "",
      size: 12,
      isBinary: true,
      isLarge: false,
      etag: "12-1000",
      path: "/repo/a.bin",
    });
    const { result } = renderHook(() => useFileContentQuery("/repo/a.bin"), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.isBinary).toBe(true);
    expect(result.current.data?.content).toBe("");
  });

  it("takes the node's word that a file is over the ceiling", async () => {
    vi.mocked(apiFetch).mockResolvedValue({
      content: "",
      size: 6 * 1024 * 1024,
      isBinary: false,
      isLarge: true,
      etag: "6291456-1000",
      path: "/repo/big.log",
    });
    const { result } = renderHook(() => useFileContentQuery("/repo/big.log"), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.isLarge).toBe(true);
    expect(result.current.data?.content).toBe("");
  });

  // The property the whole refresh design rests on: a refetch that finds the
  // file unchanged must not hand React a new object, or every poll and tab
  // switch re-renders Monaco and throws away cursor and scroll position.
  it("keeps the same data reference when the file has not changed", async () => {
    const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    const first = result.current.data;

    await result.current.refetch();

    expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(2);
    expect(result.current.data).toBe(first);
  });

  it("replaces the data when the file has changed", async () => {
    const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.content).toBe("hello");

    vi.mocked(apiFetch).mockResolvedValue({
      content: "rewritten",
      size: 9,
      isBinary: false,
      isLarge: false,
      etag: "9-2000",
      path: "/repo/a.ts",
    });
    await result.current.refetch();

    await waitFor(() => expect(result.current.data?.content).toBe("rewritten"));
  });

  // An open pane is the thing that has to notice an agent's edit, and neither
  // remount nor window focus fires while you are sitting and watching one.
  it("polls an open file", async () => {
    vi.useFakeTimers();
    try {
      const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
      await vi.waitFor(() => expect(result.current.isSuccess).toBe(true));
      expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(1);

      await vi.advanceTimersByTimeAsync(30_000);

      expect(vi.mocked(apiFetch)).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });

  // The spec calls out "a deleted file shows the error state" — prove the
  // real queryFn (not a mocked hook) actually surfaces an apiFetch rejection
  // as isError, rather than swallowing or hanging on it.
  it("surfaces a rejected fetch as an error", async () => {
    vi.mocked(apiFetch).mockRejectedValue(new Error("not found"));
    const { result } = renderHook(() => useFileContentQuery("/repo/gone.ts"), { wrapper });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error).toEqual(new Error("not found"));
  });

  // A poll the browser answers out of its own HTTP cache is a poll that proves
  // nothing: the pane reports success while showing bytes an agent overwrote
  // minutes ago, which is the staleness this whole design exists to remove.
  it("keeps the read out of the browser's HTTP cache", async () => {
    const { result } = renderHook(() => useFileContentQuery("/repo/a.ts"), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(vi.mocked(apiFetch).mock.calls[0][2]).toMatchObject({ cache: "no-store" });
  });

  // The panes only ever exercise 404 directly, so pin the whole contract here:
  // which failures are the node's verdict (drop the cached bytes) and which
  // are worth riding out on stale content.
  describe("isDefinitiveReadError", () => {
    it.each([
      [400, "path is a directory, not a file"],
      [403, "permission denied"],
      [404, "file not found"],
    ])("treats %i as the node's verdict", (status, message) => {
      expect(isDefinitiveReadError(new ApiError(message, status))).toBe(true);
    });

    it.each([
      [500, "internal error"],
      [502, "bad gateway"],
    ])("treats %i as worth retrying", (status, message) => {
      expect(isDefinitiveReadError(new ApiError(message, status))).toBe(false);
    });

    // A dropped connection never reaches ApiError at all — baseFetch only
    // constructs one from a response the node actually sent.
    it("treats a network failure as worth retrying", () => {
      expect(isDefinitiveReadError(new Error("Failed to fetch"))).toBe(false);
    });

    it("treats a non-error as worth retrying", () => {
      expect(isDefinitiveReadError(undefined)).toBe(false);
    });
  });

  it("is idle for an empty path", () => {
    const { result } = renderHook(() => useFileContentQuery(""), { wrapper });
    expect(result.current.fetchStatus).toBe("idle");
    expect(vi.mocked(apiFetch)).not.toHaveBeenCalled();
  });
});
