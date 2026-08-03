import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { TerminalToolbar } from "./TerminalToolbar";

afterEach(cleanup);

describe("TerminalToolbar", () => {
  it("renders exactly the nine special keys", () => {
    render(<TerminalToolbar onKeyPress={() => {}} />);

    const buttons = screen.getAllByRole("button");
    expect(buttons).toHaveLength(9);
  });

  it("no longer offers compose or attach — those live in the ComposeBar", () => {
    render(<TerminalToolbar onKeyPress={() => {}} />);

    expect(screen.queryByRole("button", { name: /attach/i })).toBeNull();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("sends the escape sequence for a plain key", () => {
    const onKeyPress = vi.fn();
    render(<TerminalToolbar onKeyPress={onKeyPress} />);

    fireEvent.click(screen.getByText("esc"));

    expect(onKeyPress).toHaveBeenCalledWith("\x1b");
  });

  it("opens a popover for a menu key instead of sending anything", () => {
    const onKeyPress = vi.fn();
    render(<TerminalToolbar onKeyPress={onKeyPress} />);

    fireEvent.click(screen.getByText("ctrl"));

    expect(onKeyPress).not.toHaveBeenCalled();
    expect(screen.getByText("^C")).toBeTruthy();
  });
});
