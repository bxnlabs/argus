import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import { createRef } from "react";
import { PickerTriggerField } from "./PickerTriggerField";

afterEach(cleanup);

describe("PickerTriggerField", () => {
  it("renders the label and the placeholder when value is empty", () => {
    render(
      <PickerTriggerField
        label="Source"
        value=""
        placeholder="Click to select a folder..."
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("Source")).toBeTruthy();
    const button = screen.getByRole("button", { name: /select a folder/i });
    expect(button.textContent).toContain("Click to select a folder...");
  });

  it("renders the value when set", () => {
    render(
      <PickerTriggerField
        label="Source"
        value="~/projects/argus"
        placeholder="Click to select a folder..."
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("~/projects/argus")).toBeTruthy();
  });

  it("renders an optional tag when optional is set", () => {
    render(
      <PickerTriggerField
        label="Branch"
        optional
        value=""
        placeholder="Select a branch..."
        onOpen={() => {}}
      />,
    );
    expect(screen.getByText("(optional)")).toBeTruthy();
  });

  it("is a non-submitting button with dialog popup semantics", () => {
    render(
      <PickerTriggerField
        label="Source"
        value=""
        placeholder="Select..."
        onOpen={() => {}}
      />,
    );
    const button = screen.getByRole("button") as HTMLButtonElement;
    expect(button.type).toBe("button");
    expect(button.getAttribute("aria-haspopup")).toBe("dialog");
  });

  it("calls onOpen when activated", () => {
    const onOpen = vi.fn();
    render(
      <PickerTriggerField
        label="Source"
        value=""
        placeholder="Select..."
        onOpen={onOpen}
      />,
    );
    fireEvent.click(screen.getByRole("button"));
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it("forwards a ref to the underlying button", () => {
    const ref = createRef<HTMLButtonElement>();
    render(
      <PickerTriggerField
        ref={ref}
        label="Source"
        value=""
        placeholder="Select..."
        onOpen={() => {}}
      />,
    );
    expect(ref.current).toBeInstanceOf(HTMLButtonElement);
  });
});
