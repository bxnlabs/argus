import { describe, it, expect } from "vitest";
import { getStatusMeta } from "./sessionStatus";

describe("getStatusMeta", () => {
  it("maps known statuses to color, animation, and label", () => {
    expect(getStatusMeta("active")).toEqual({
      label: "Active",
      color: "bg-green-500",
      animation: "animate-pulse-green",
    });
    expect(getStatusMeta("idle")).toEqual({
      label: "Idle",
      color: "bg-muted-foreground",
      animation: "",
    });
    expect(getStatusMeta("dead")).toEqual({
      label: "Dead",
      color: "bg-red-500/50",
      animation: "",
    });
  });

  it("falls back to a muted, unlabeled meta for unknown or undefined status", () => {
    const fallback = {
      label: "",
      color: "bg-muted-foreground/40",
      animation: "",
    };
    expect(getStatusMeta(undefined)).toEqual(fallback);
    expect(getStatusMeta("bogus")).toEqual(fallback);
  });
});
