import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StubNodeProvider } from "@/test/node-context";
import { sessionKeys } from "./keys";
import { useDeleteSession } from "./queries";

const listKey = sessionKeys.list("local:");

beforeEach(() => {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ success: true, branch_deleted: false }), {
          status: 200,
        }),
    ),
  );
});
afterEach(() => vi.unstubAllGlobals());

describe("useDeleteSession", () => {
  it("drops the deleted session from the cached list on success", async () => {
    const qc = new QueryClient();
    qc.setQueryData(listKey, {
      home_dir: "/home/user",
      sessions: [{ id: "a" }, { id: "b" }],
    });

    const { result } = renderHook(() => useDeleteSession(), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={qc}>
          <StubNodeProvider>{children}</StubNodeProvider>
        </QueryClientProvider>
      ),
    });

    act(() => {
      result.current.mutate({ sessionId: "a" });
    });

    await waitFor(() => {
      const cached = qc.getQueryData<{ sessions: { id: string }[] }>(listKey);
      expect(cached?.sessions.map((s) => s.id)).toEqual(["b"]);
    });
  });
});
