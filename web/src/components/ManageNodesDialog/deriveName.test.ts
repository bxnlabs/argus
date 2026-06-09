import { describe, it, expect } from "vitest";
import { deriveNodeName } from "./deriveName";

describe("deriveNodeName", () => {
  it("reverses the default tailscale hostname (first label, strip argus-)", () => {
    expect(deriveNodeName("argus-bumblebee.tail06de7.ts.net")).toBe("bumblebee");
  });

  it("keeps a bare hostname without an argus- prefix", () => {
    expect(deriveNodeName("gpu-box")).toBe("gpu-box");
  });

  it("strips argus- even without a domain", () => {
    expect(deriveNodeName("argus-laptop")).toBe("laptop");
  });

  it("tolerates a pasted scheme, port, and path", () => {
    expect(deriveNodeName("https://argus-bumblebee.tail06de7.ts.net:8443/")).toBe("bumblebee");
  });

  it("returns empty for empty/whitespace input", () => {
    expect(deriveNodeName("   ")).toBe("");
  });
});
