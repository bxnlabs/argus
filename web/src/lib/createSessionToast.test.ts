import { describe, it, expect, vi } from "vitest";
import { resolveCreateToastDecision } from "./createSessionToast";

describe("resolveCreateToastDecision", () => {
  it("errors with a handoff id: attaches the id so the loading toast is replaced", () => {
    const decision = resolveCreateToastDecision("error", "session", "toast-1");
    expect(decision).toEqual({
      kind: "error",
      message: "Failed to create session",
      options: { id: "toast-1" },
    });
  });

  it("errors without a handoff id: a plain new error toast", () => {
    const decision = resolveCreateToastDecision("error", "session", null);
    expect(decision).toEqual({
      kind: "error",
      message: "Failed to create session",
      options: {},
    });
  });

  it("success with neither id nor action: stays silent", () => {
    const decision = resolveCreateToastDecision("success", "foo", null);
    expect(decision).toBeNull();
  });

  it("success with an id only: replaces the handoff toast, default duration", () => {
    const decision = resolveCreateToastDecision("success", "foo", "toast-1");
    expect(decision).toEqual({
      kind: "success",
      message: "Created foo",
      options: { id: "toast-1" },
    });
  });

  it("success with an action only: a fresh toast carrying the action with a longer duration", () => {
    const action = { label: "Open", onClick: vi.fn() };
    const decision = resolveCreateToastDecision("success", "foo", null, action);
    expect(decision).toEqual({
      kind: "success",
      message: "Created foo",
      options: { action, duration: 10000 },
    });
  });

  it("success with both id and action: replaces the handoff toast and carries the action", () => {
    const action = { label: "Open", onClick: vi.fn() };
    const decision = resolveCreateToastDecision(
      "success",
      "foo",
      "toast-1",
      action,
    );
    expect(decision).toEqual({
      kind: "success",
      message: "Created foo",
      options: { id: "toast-1", action, duration: 10000 },
    });
  });
});
