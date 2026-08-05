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
    const classes = bar.className.split(" ");
    expect(classes).toContain("rounded-b-lg");
    expect(classes).toContain("border-x");
    expect(classes).toContain("border-b");
    expect(classes).toContain("mx-2");
    expect(classes).toContain("mb-1.5");
  });

  it("brightens its card edge with the compose bar, so the halves move together", () => {
    const { rerender } = render(
      <TerminalToolbar onKeyPress={() => {}} focused={false} />,
    );
    const bar = () => screen.getByTestId("terminal-toolbar");
    expect(bar().className.split(" ")).toContain("border-[hsl(0_0%_20%)]");

    rerender(<TerminalToolbar onKeyPress={() => {}} focused={true} />);
    expect(bar().className.split(" ")).toContain("border-[hsl(0_0%_30%)]");
  });

  it("animates its border colour, so both halves of the card's shared edge fade together", () => {
    // The card's left and right edges are one continuous vertical line split
    // across this box and ComposeBar's panel. If only one half transitions,
    // the edge resolves two-tone over the transition instead of moving as a
    // single line — the exact artifact the two-box construction exists to
    // hide. ComposeBar's panel already carries transition-colors; this pins
    // the toolbar's matching half so the two can't drift apart again.
    render(<TerminalToolbar onKeyPress={() => {}} />);

    const bar = screen.getByTestId("terminal-toolbar");
    expect(bar.className.split(" ")).toContain("transition-colors");
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
    expect(bar.className.split(" ")).toContain("border-t-[hsl(0_0%_14%)]");
  });
});
