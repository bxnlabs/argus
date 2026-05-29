import { describe, it, expect } from "vitest";
import {
  getStatusColor,
  getStatusLabel,
  getStatusAnimation,
} from "./sessionStatus";

describe("sessionStatus helpers", () => {
  it("maps known statuses to colors", () => {
    expect(getStatusColor("active")).toBe("bg-green-500");
    expect(getStatusColor("idle")).toBe("bg-muted-foreground");
    expect(getStatusColor("dead")).toBe("bg-red-500/50");
    expect(getStatusColor(undefined)).toBe("bg-muted-foreground/40");
  });

  it("labels known statuses and returns empty string otherwise", () => {
    expect(getStatusLabel("active")).toBe("Active");
    expect(getStatusLabel("idle")).toBe("Idle");
    expect(getStatusLabel("dead")).toBe("Dead");
    expect(getStatusLabel(undefined)).toBe("");
  });

  it("animates only the active status", () => {
    expect(getStatusAnimation("active")).toBe("animate-pulse-green");
    expect(getStatusAnimation("idle")).toBe("");
    expect(getStatusAnimation(undefined)).toBe("");
  });
});
