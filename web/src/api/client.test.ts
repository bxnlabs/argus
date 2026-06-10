import { describe, it, expect } from "vitest";
import { nodeWsUrl } from "./client";

describe("nodeWsUrl", () => {
  it("targets a remote node's origin with a ws scheme", () => {
    expect(nodeWsUrl("http://gpu-box:80", "s1")).toBe(
      "ws://gpu-box:80/api/node/ws/sessions/s1",
    );
  });

  it("falls back to same-origin when baseUrl is empty", () => {
    // jsdom serves the test from http://localhost (see vite.config test env).
    expect(nodeWsUrl("", "s1")).toBe("ws://localhost/api/node/ws/sessions/s1");
    expect(nodeWsUrl("")).toBe("ws://localhost/api/node/ws/terminal");
  });
});
