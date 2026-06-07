import { describe, it, expect, afterEach } from "vitest";
import { getNodeBaseUrl, setActiveNodeBaseUrl, getNodeWsUrl } from "./client";

afterEach(() => setActiveNodeBaseUrl(""));

describe("active node base URL", () => {
  it("defaults to same-origin (empty)", () => {
    expect(getNodeBaseUrl()).toBe("");
  });
  it("routes calls at the selected node origin", () => {
    setActiveNodeBaseUrl("http://gpu-box:80");
    expect(getNodeBaseUrl()).toBe("http://gpu-box:80");
    expect(getNodeWsUrl("s1")).toBe("ws://gpu-box:80/api/node/ws/sessions/s1");
  });
});
