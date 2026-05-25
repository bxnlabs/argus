import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { ShortcutHintOverlay } from "./index";
import type { ChordMap, ChordPending } from "@/hooks/useKeyboardChords";

const bindings: ChordMap = {
  g: {
    label: "Git",
    children: {
      h: { label: "History" },
      c: { label: "Compare" },
    },
  },
  n: { label: "New session" },
  "?": { label: "Help" },
};

const noop = () => {};

describe("ShortcutHintOverlay", () => {
  it("renders nothing when pending is null and helpOpen is false", () => {
    const { container } = render(
      <ShortcutHintOverlay
        pending={null}
        bindings={bindings}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders a row per entry with its key and label when at top-level pending", () => {
    const pending: ChordPending = { level: bindings, path: [] };
    render(
      <ShortcutHintOverlay
        pending={pending}
        bindings={bindings}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );

    // Heading shows the leader label (appears in the <p> heading)
    expect(screen.getAllByText("⌘ ;").length).toBeGreaterThan(0);

    // Each top-level entry's label is rendered
    expect(screen.getByText("Git")).toBeTruthy();
    expect(screen.getByText("New session")).toBeTruthy();
    expect(screen.getByText("Help")).toBeTruthy();

    // Keys rendered as kbd chips
    expect(screen.getByText("g")).toBeTruthy();
    expect(screen.getByText("n")).toBeTruthy();
    expect(screen.getByText("?")).toBeTruthy();

    // Git has children → sub-chord chevron present
    expect(screen.getAllByText("›").length).toBeGreaterThan(0);
  });

  it("renders breadcrumb heading when path is ['g']", () => {
    const pending: ChordPending = {
      level: bindings.g.children!,
      path: ["g"],
    };
    render(
      <ShortcutHintOverlay
        pending={pending}
        bindings={bindings}
        leaderLabel="⌘ ;"
        helpOpen={false}
        onHelpOpenChange={noop}
      />,
    );

    // The heading <p> should contain "Git ›" (breadcrumb from path ["g"])
    expect(screen.getByText("Git ›")).toBeTruthy();

    // Sub-chord entries rendered
    expect(screen.getByText("History")).toBeTruthy();
    expect(screen.getByText("Compare")).toBeTruthy();
  });
});
