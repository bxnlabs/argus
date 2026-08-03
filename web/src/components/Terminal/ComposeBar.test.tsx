import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

import { ComposeBar } from "./ComposeBar";
import { StubNodeProvider } from "@/test/node-context";
import type { ComponentProps } from "react";

afterEach(cleanup);

// ComposeBar renders FilePicker (Task 2's insertPaths consumer), which pulls
// in the upload mutation (useQueryClient) and the active-node scope
// (useNodeContext) regardless of whether the picker is open. Wrap every
// render with the same fixtures FileBrowser.test.tsx uses so those hooks
// resolve instead of throwing "No QueryClient set" / "must be used within
// NodeProvider".
function renderComposeBar(props: ComponentProps<typeof ComposeBar>) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <StubNodeProvider>
        <ComposeBar {...props} />
      </StubNodeProvider>
    </QueryClientProvider>,
  );
}

function textarea() {
  return screen.getByRole("textbox") as HTMLTextAreaElement;
}

describe("ComposeBar", () => {
  it("sends the typed text and clears the input", () => {
    const onSend = vi.fn();
    renderComposeBar({ onSend, connected: true });

    fireEvent.change(textarea(), { target: { value: "run the tests" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(onSend).toHaveBeenCalledWith("run the tests");
    expect(textarea().value).toBe("");
  });

  it("keeps focus on the input after sending so the keyboard stays up", () => {
    renderComposeBar({ onSend: () => {}, connected: true });

    fireEvent.focus(textarea());
    fireEvent.change(textarea(), { target: { value: "hi" } });
    fireEvent.click(screen.getByRole("button", { name: /send/i }));

    expect(document.activeElement).toBe(textarea());
  });

  it("disables send when the draft is only whitespace", () => {
    renderComposeBar({ onSend: () => {}, connected: true });

    fireEvent.change(textarea(), { target: { value: "   " } });

    expect(screen.getByRole("button", { name: /send/i })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("disables send while disconnected but preserves the draft", () => {
    renderComposeBar({ onSend: () => {}, connected: false });

    fireEvent.change(textarea(), { target: { value: "queued message" } });

    expect(screen.getByRole("button", { name: /send/i })).toHaveProperty(
      "disabled",
      true,
    );
    expect(textarea().value).toBe("queued message");
  });

  it("hides the send button only when the bar is unfocused AND empty", () => {
    renderComposeBar({ onSend: () => {}, connected: true });

    expect(screen.queryByRole("button", { name: /send/i })).toBeNull();

    fireEvent.focus(textarea());
    expect(screen.queryByRole("button", { name: /send/i })).not.toBeNull();

    fireEvent.change(textarea(), { target: { value: "draft" } });
    fireEvent.blur(textarea());
    // A draft must stay sendable after tapping away to the terminal.
    expect(screen.queryByRole("button", { name: /send/i })).not.toBeNull();
  });

  it("swaps the placeholder between focused and unfocused", () => {
    renderComposeBar({ onSend: () => {}, connected: true });

    expect(textarea().placeholder).toBe("Tap to compose");

    fireEvent.focus(textarea());
    expect(textarea().placeholder).toBe("Message…");

    fireEvent.blur(textarea());
    expect(textarea().placeholder).toBe("Tap to compose");
  });

  it("never dims the draft text itself when focus leaves", () => {
    renderComposeBar({ onSend: () => {}, connected: true });

    fireEvent.change(textarea(), { target: { value: "still readable" } });
    fireEvent.blur(textarea());

    // Dimming is chrome-only — the textarea must not carry an opacity class.
    expect(textarea().className).not.toMatch(/opacity-/);
  });

  it("does not send on Enter — Enter inserts a newline", () => {
    const onSend = vi.fn();
    renderComposeBar({ onSend, connected: true });

    fireEvent.change(textarea(), { target: { value: "line one" } });
    fireEvent.keyDown(textarea(), { key: "Enter" });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("opens the file picker from the attach button", () => {
    renderComposeBar({
      onSend: () => {},
      connected: true,
      workingDirectory: "/w",
    });

    expect(screen.queryByRole("dialog")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /attach/i }));

    expect(screen.queryByRole("dialog")).not.toBeNull();
  });
});
