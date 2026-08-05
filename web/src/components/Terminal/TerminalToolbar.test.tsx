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

  it("renders the card's bottom half", () => {
    render(<TerminalToolbar onKeyPress={() => {}} />);

    const bar = screen.getByTestId("terminal-toolbar");
    expect(bar.className).toContain("rounded-b-lg");
    expect(bar.className).toContain("border-x");
    expect(bar.className).toContain("border-b");
    expect(bar.className).toContain("mx-2");
    expect(bar.className).toContain("mb-1.5");
  });

  it("brightens its card edge with the compose bar, so the halves move together", () => {
    const { rerender } = render(
      <TerminalToolbar onKeyPress={() => {}} focused={false} />,
    );
    const bar = () => screen.getByTestId("terminal-toolbar");
    expect(bar().className).toContain("border-[hsl(0_0%_20%)]");

    rerender(<TerminalToolbar onKeyPress={() => {}} focused={true} />);
    expect(bar().className).toContain("border-[hsl(0_0%_30%)]");
  });

  it("keeps the row divider a distinct value from the card edge", () => {
    render(<TerminalToolbar onKeyPress={() => {}} focused={false} />);

    // The seam between the two halves is the divider, quieter than the card's
    // own edge so it separates without competing.
    //
    // This is also a twMerge guard. `border-t-<color>` and `border-<color>`
    // are conflicting groups: a generic `border-[...]` appearing AFTER
    // `border-t-[...]` in the cn() arguments silently deletes the divider.
    // The divider must therefore be the LAST border colour passed.
    const bar = screen.getByTestId("terminal-toolbar");
    expect(bar.className).toContain("border-t-[hsl(0_0%_14%)]");
  });
});
