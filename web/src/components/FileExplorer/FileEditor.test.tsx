import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";

// Monaco is heavy and irrelevant here: this file is about the options and
// handlers FileEditor hands it, not about editing.
const editorProps = vi.fn();
vi.mock("@monaco-editor/react", () => ({
  default: (props: Record<string, unknown>) => {
    editorProps(props);
    return <div data-testid="monaco" />;
  },
}));

const { FileEditor } = await import("./FileEditor");

afterEach(() => {
  vi.clearAllMocks();
  cleanup();
});

describe("FileEditor", () => {
  // An editor that accepts keystrokes it cannot save is worse than one that
  // refuses them: the user types, nothing objects, and the work is gone on the
  // next remount with no save that could have prevented it.
  it("puts Monaco in read-only mode", () => {
    render(
      <FileEditor content="hello" language="typescript" isBinary={false} isLarge={false} />,
    );

    expect(screen.getByTestId("monaco")).toBeTruthy();
    const props = editorProps.mock.calls[0][0] as { options: { readOnly?: boolean } };
    expect(props.options.readOnly).toBe(true);
  });

  it("wires no change handler at all", () => {
    render(
      <FileEditor content="hello" language="typescript" isBinary={false} isLarge={false} />,
    );

    const props = editorProps.mock.calls[0][0] as { onChange?: unknown };
    expect(props.onChange).toBeUndefined();
  });
});
