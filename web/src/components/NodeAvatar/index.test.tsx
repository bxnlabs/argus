import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

afterEach(() => { cleanup(); });
import { NodeAvatar } from "./index";
import type { NodeWithStatus } from "@/types";

const node = (over: Partial<NodeWithStatus>): NodeWithStatus => ({
  id: "local", name: "prime", url: "", source: "local",
  self: true, summary: null, online: true, pending: false, ...over,
});

describe("NodeAvatar", () => {
  it("shows the first two letters with only the first capitalized (Slack style)", () => {
    render(<NodeAvatar node={node({ name: "argus-bumblebee" })} />);
    expect(screen.getByText("Ar")).toBeTruthy();
  });

  it("derives a stable per-node color that differs between nodes", () => {
    const { container: a } = render(<NodeAvatar node={node({ id: "local" })} />);
    const colorA = (a.firstChild as HTMLElement).style.backgroundColor;
    cleanup();
    const { container: b } = render(<NodeAvatar node={node({ id: "discovered:bee" })} />);
    const colorB = (b.firstChild as HTMLElement).style.backgroundColor;
    expect(colorA).toBeTruthy();
    expect(colorA).not.toBe(colorB);
  });

  it("carries the connection-status presence dot, and can hide it", () => {
    render(<NodeAvatar node={node({ online: false, pending: true })} />);
    expect(screen.getByTestId("node-avatar-dot").className).toContain("bg-amber-500");
    cleanup();
    const { container } = render(<NodeAvatar node={node({})} showStatus={false} />);
    expect(container.querySelector("[data-testid='node-avatar-dot']")).toBeNull();
  });
});
