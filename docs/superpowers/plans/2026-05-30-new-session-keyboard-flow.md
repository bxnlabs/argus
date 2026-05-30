# Keyboard-complete New Session flow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the web New Session dialog fully keyboard-operable end to end — open the Source/Branch pickers with the keyboard, return focus to the field just used when a picker closes, and submit via `⌘/Ctrl+Enter`.

**Architecture:** Replace the readonly `<Input>` triggers for Source and Branch with a reusable `PickerTriggerField` rendered as a real `<button type="button">` (keyboard-activatable, cannot submit the form). Add trigger refs and restore focus to them on picker close. Rework the dialog's key handling so Enter never submits and `⌘/Ctrl+Enter` does. All changes are confined to `web/src/components/NewSessionDialog/`; no backend, API, or data-model changes.

**Tech Stack:** React + TypeScript, Tailwind, Radix UI (`Dialog`, `Select`, `Switch`), Vitest + `@testing-library/react` (jsdom, `fireEvent` only — no `user-event`).

---

## Background notes for the implementer

- Work happens entirely in `web/`. Run all commands from the `web/` directory.
- The full test command is `pnpm test` (alias for `vitest run`). A single file: `pnpm test src/components/NewSessionDialog/index.test.tsx`. Typecheck/build: `pnpm build` (runs `tsc -b && vite build`).
- **jsdom keyboard caveat:** in jsdom, `fireEvent.keyDown(button, { key: "Enter" })` does **not** trigger a native `<button>`'s `onClick`. Real browsers do translate Enter/Space on a button into activation. So we test the click→open wiring with `fireEvent.click`, and separately assert the element is a real `<button type="button">` (which guarantees native Enter/Space activation and guarantees it cannot submit the form). The `⌘/Ctrl+Enter` submit path uses an explicit `onKeyDown` handler and IS directly testable.
- **Do NOT add explicit Enter/Space `onKeyDown` handlers to the trigger button.** A native `<button>` already activates on Enter/Space; adding our own would double-fire (native click + handler) in real browsers. Native button behavior is the correct, sufficient mechanism.
- The current file under change is `web/src/components/NewSessionDialog/index.tsx`. Read it before starting.
- `isMac()` lives at `web/src/lib/device.ts` (`import { isMac } from "@/lib/device"`).
- `useViewport()` returns `{ isMobile, isDesktop, isHydrated }` and is imported from `@/hooks/useViewport`.

---

## File Structure

- **Create** `web/src/components/NewSessionDialog/PickerTriggerField.tsx` — a presentational trigger field: label (+ optional tag) and a styled `<button>` that opens a picker. Forwards a ref to the button. One responsibility: render a keyboard-activatable picker trigger that looks like the existing input.
- **Create** `web/src/components/NewSessionDialog/PickerTriggerField.test.tsx` — unit tests for the component.
- **Modify** `web/src/components/NewSessionDialog/index.tsx` — use `PickerTriggerField` for Source and Branch; add `sourceTriggerRef`/`branchTriggerRef` and focus-restore on picker close; rework key handling (remove `Shift+Enter`, add `⌘/Ctrl+Enter`, suppress plain Enter in Name); add desktop footer chord hint.
- **Create** `web/src/components/NewSessionDialog/index.test.tsx` — integration tests for keyboard behaviors.

---

## Task 1: `PickerTriggerField` component

**Files:**
- Create: `web/src/components/NewSessionDialog/PickerTriggerField.tsx`
- Test: `web/src/components/NewSessionDialog/PickerTriggerField.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/components/NewSessionDialog/PickerTriggerField.test.tsx`:

```tsx
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pnpm test src/components/NewSessionDialog/PickerTriggerField.test.tsx`
Expected: FAIL — `Failed to resolve import "./PickerTriggerField"` (module does not exist yet).

- [ ] **Step 3: Write minimal implementation**

Create `web/src/components/NewSessionDialog/PickerTriggerField.tsx`:

```tsx
import { forwardRef } from "react";
import { cn } from "@/lib/utils";

interface PickerTriggerFieldProps {
  label: string;
  value: string;
  placeholder: string;
  onOpen: () => void;
  optional?: boolean;
  open?: boolean;
}

export const PickerTriggerField = forwardRef<
  HTMLButtonElement,
  PickerTriggerFieldProps
>(function PickerTriggerField(
  { label, value, placeholder, onOpen, optional, open },
  ref,
) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">
        {label}
        {optional && (
          <span className="text-muted-foreground font-normal"> (optional)</span>
        )}
      </label>
      <button
        ref={ref}
        type="button"
        onClick={onOpen}
        aria-haspopup="dialog"
        aria-expanded={open ?? false}
        className={cn(
          "border-input bg-transparent flex h-9 w-full items-center rounded-md border px-3 py-1 text-left text-sm shadow-sm transition-colors",
          "focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring",
          value ? "text-foreground" : "text-muted-foreground",
        )}
      >
        <span className="truncate">{value || placeholder}</span>
      </button>
    </div>
  );
});
```

Note: the className mirrors the project's `Input` styling (`web/src/components/ui/input.tsx`) so the button is visually indistinguishable from the readonly inputs it replaces. If `Input`'s classes differ in your checkout, copy its border/height/focus classes verbatim and keep the `flex items-center text-left` additions.

- [ ] **Step 4: Run test to verify it passes**

Run: `pnpm test src/components/NewSessionDialog/PickerTriggerField.test.tsx`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/NewSessionDialog/PickerTriggerField.tsx web/src/components/NewSessionDialog/PickerTriggerField.test.tsx
git commit -m "feat(new-session): add PickerTriggerField keyboard trigger button"
```

---

## Task 2: Use `PickerTriggerField` for Source and Branch

**Files:**
- Modify: `web/src/components/NewSessionDialog/index.tsx`

This task swaps the two readonly `<Input>` triggers for the new button component and wires up refs (refs are added now and consumed in Task 3). No behavior test here — the swap is covered by Task 4's integration tests; this task is verified by typecheck + existing tests still passing.

- [ ] **Step 1: Add the import and trigger refs**

In `web/src/components/NewSessionDialog/index.tsx`, add to the imports near the top:

```tsx
import { PickerTriggerField } from "./PickerTriggerField";
```

Inside the `NewSessionDialog` component body, alongside the existing `childPickerClosingRef` declaration (around `index.tsx:56`), add:

```tsx
  const sourceTriggerRef = useRef<HTMLButtonElement>(null);
  const branchTriggerRef = useRef<HTMLButtonElement>(null);
```

(`useRef` is already imported.)

- [ ] **Step 2: Replace the Source field markup**

Find the Source block (currently around `index.tsx:164-173`):

```tsx
            <div className="space-y-2">
              <label className="text-sm font-medium">Source</label>
              <Input
                value={source}
                readOnly
                onClick={() => setShowSourcePicker(true)}
                placeholder="Click to select a folder or repository..."
                className="cursor-pointer"
              />
            </div>
```

Replace it with:

```tsx
            <PickerTriggerField
              ref={sourceTriggerRef}
              label="Source"
              value={source}
              placeholder="Select a folder or repository..."
              onOpen={() => setShowSourcePicker(true)}
              open={showSourcePicker}
            />
```

- [ ] **Step 3: Replace the Branch field markup**

Find the Branch block (currently around `index.tsx:175-188`):

```tsx
            {showBranchField && (
              <div className="space-y-2">
                <label className="text-sm font-medium">
                  Branch <span className="text-muted-foreground font-normal">(optional)</span>
                </label>
                <Input
                  value={branch}
                  readOnly
                  onClick={() => setShowBranchPicker(true)}
                  placeholder="Click to select or type a branch..."
                  className="cursor-pointer"
                />
              </div>
            )}
```

Replace it with:

```tsx
            {showBranchField && (
              <PickerTriggerField
                ref={branchTriggerRef}
                label="Branch"
                optional
                value={branch}
                placeholder="Select or type a branch..."
                onOpen={() => setShowBranchPicker(true)}
                open={showBranchPicker}
              />
            )}
```

- [ ] **Step 4: Remove the now-unused `Input` import if nothing else uses it**

Search the file for other `<Input` usages:

Run: `grep -n "<Input" web/src/components/NewSessionDialog/index.tsx`
Expected: one remaining match — the Name field (`index.tsx` ~line 156). Since `Input` is still used by Name, **leave the `Input` import in place.** (Do not remove it.)

- [ ] **Step 5: Typecheck and run existing tests**

Run: `pnpm build`
Expected: PASS (no TypeScript errors).

Run: `pnpm test`
Expected: PASS (existing suite unaffected; PickerTriggerField tests still pass).

- [ ] **Step 6: Commit**

```bash
git add web/src/components/NewSessionDialog/index.tsx
git commit -m "refactor(new-session): use PickerTriggerField for Source and Branch"
```

---

## Task 3: Restore focus to the trigger when a picker closes

**Files:**
- Modify: `web/src/components/NewSessionDialog/index.tsx`

After a picker closes (select or cancel), focus must return to the trigger button just used. Both pickers funnel through a single close path each, so we attach the focus restore there.

- [ ] **Step 1: Add a focus-restore helper**

In `index.tsx`, inside the component body (near `closeBranchPicker`, around `index.tsx:88`), add:

```tsx
  const restoreFocus = (ref: React.RefObject<HTMLButtonElement>) => {
    requestAnimationFrame(() => ref.current?.focus());
  };
```

- [ ] **Step 2: Restore focus on Source picker close**

Find the `SourcePicker`'s `onOpenChange` handler (currently around `index.tsx:236-240`):

```tsx
        onOpenChange={(o) => {
          if (!o) childPickerClosingRef.current = true;
          setShowSourcePicker(o);
        }}
```

Replace it with:

```tsx
        onOpenChange={(o) => {
          if (!o) {
            childPickerClosingRef.current = true;
            restoreFocus(sourceTriggerRef);
          }
          setShowSourcePicker(o);
        }}
```

- [ ] **Step 3: Restore focus on Branch picker close**

Find `closeBranchPicker` (currently around `index.tsx:88-91`):

```tsx
  const closeBranchPicker = () => {
    childPickerClosingRef.current = true;
    setShowBranchPicker(false);
  };
```

Replace it with:

```tsx
  const closeBranchPicker = () => {
    childPickerClosingRef.current = true;
    setShowBranchPicker(false);
    restoreFocus(branchTriggerRef);
  };
```

Also update the `BranchDialog`'s `onOpenChange` (currently around `index.tsx:252-255`) so closing via the dialog (e.g. clicking the overlay) also restores focus:

```tsx
          onOpenChange={(o) => {
            if (!o) {
              childPickerClosingRef.current = true;
              restoreFocus(branchTriggerRef);
            }
            setShowBranchPicker(o);
          }}
```

- [ ] **Step 4: Typecheck**

Run: `pnpm build`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/NewSessionDialog/index.tsx
git commit -m "feat(new-session): return focus to trigger when picker closes"
```

(Focus-return is verified by the integration test in Task 5 and by manual in-browser check.)

---

## Task 4: Rework submit — `⌘/Ctrl+Enter` only, Enter never submits

**Files:**
- Modify: `web/src/components/NewSessionDialog/index.tsx`

- [ ] **Step 1: Replace the `DialogContent` `onKeyDown` (Shift+Enter → ⌘/Ctrl+Enter)**

Find the existing handler (around `index.tsx:141-146`):

```tsx
          onKeyDown={(e) => {
            if (e.key === "Enter" && e.shiftKey) {
              e.preventDefault();
              handleSubmit(e as unknown as React.FormEvent);
            }
          }}
```

Replace it with:

```tsx
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              handleSubmit(e as unknown as React.FormEvent);
            }
          }}
```

- [ ] **Step 2: Suppress plain Enter in the Name input**

Find the Name `<Input>` (around `index.tsx:155-162`):

```tsx
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-feature"
                autoFocus
              />
```

Replace it with:

```tsx
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  // Enter never submits; submit is ⌘/Ctrl+Enter or the Create
                  // button. Let ⌘/Ctrl+Enter bubble to the dialog handler.
                  if (e.key === "Enter" && !(e.metaKey || e.ctrlKey)) {
                    e.preventDefault();
                  }
                }}
                placeholder="my-feature"
                autoFocus
              />
```

- [ ] **Step 3: Typecheck**

Run: `pnpm build`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/components/NewSessionDialog/index.tsx
git commit -m "feat(new-session): submit via Cmd/Ctrl+Enter, Enter no longer submits"
```

---

## Task 5: Integration tests for keyboard behaviors

**Files:**
- Create: `web/src/components/NewSessionDialog/index.test.tsx`

These tests mock the data hooks and the heavy child pickers (matching the stubbing approach in `web/src/components/GitPanel/index.test.tsx`) so we can assert the dialog's orchestration in isolation.

- [ ] **Step 1: Write the test file**

Create `web/src/components/NewSessionDialog/index.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeAll, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup, waitFor } from "@testing-library/react";
import { NewSessionDialog } from "./index";

// --- Mock data hooks ---
vi.mock("@/data/sessions", () => ({
  useProfilesQuery: () => ({
    data: { profiles: ["default", "review"] },
    refetch: vi.fn(),
  }),
}));
vi.mock("@/data/git/queries", () => ({
  useGitCheckQuery: () => ({ data: false }),
}));
vi.mock("@/hooks/useViewport", () => ({
  useViewport: () => ({ isMobile: false, isDesktop: true, isHydrated: true }),
}));

// --- Stub the child pickers so we can observe open state and drive close ---
vi.mock("@/components/SourcePicker", () => ({
  SourcePicker: ({
    open,
    onOpenChange,
    onSelect,
  }: {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onSelect: (value: string, tab: "local" | "remote") => void;
  }) =>
    open ? (
      <div data-testid="source-picker">
        <button
          type="button"
          onClick={() => {
            onSelect("~/projects/argus", "local");
            onOpenChange(false);
          }}
        >
          pick-source
        </button>
      </div>
    ) : null,
}));

beforeAll(() => {
  if (!("requestAnimationFrame" in globalThis)) {
    globalThis.requestAnimationFrame = ((cb: FrameRequestCallback) =>
      setTimeout(() => cb(0), 0)) as typeof requestAnimationFrame;
  }
});

afterEach(cleanup);

function renderDialog(overrides: Partial<Parameters<typeof NewSessionDialog>[0]> = {}) {
  const onCreateSession = vi.fn();
  const onClose = vi.fn();
  render(
    <NewSessionDialog
      open
      onClose={onClose}
      onCreateSession={onCreateSession}
      {...overrides}
    />,
  );
  return { onCreateSession, onClose };
}

function sourceTrigger(): HTMLButtonElement {
  return screen.getByRole("button", {
    name: /select a folder or repository/i,
  }) as HTMLButtonElement;
}

function nameInput(): HTMLInputElement {
  return screen.getByPlaceholderText("my-feature") as HTMLInputElement;
}

describe("NewSessionDialog keyboard flow", () => {
  it("renders Source as a non-submitting dialog-trigger button", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    const trigger = sourceTrigger();
    expect(trigger.type).toBe("button");
    expect(trigger.getAttribute("aria-haspopup")).toBe("dialog");
  });

  it("opens the Source picker when the trigger is activated", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    expect(screen.queryByTestId("source-picker")).toBeNull();
    fireEvent.click(sourceTrigger());
    expect(screen.getByTestId("source-picker")).toBeTruthy();
  });

  it("submits with Cmd+Enter when the name is non-empty", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "  my-feature  " } });
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
    expect(onCreateSession).toHaveBeenCalledWith(
      expect.objectContaining({ name: "my-feature", provider_type: "claude" }),
    );
  });

  it("submits with Ctrl+Enter as well", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      ctrlKey: true,
    });
    expect(onCreateSession).toHaveBeenCalledTimes(1);
  });

  it("does not submit on Cmd+Enter when the name is empty", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onCreateSession).not.toHaveBeenCalled();
  });

  it("does not submit on plain Enter in the Name field", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });
    fireEvent.keyDown(nameInput(), { key: "Enter" });
    expect(onCreateSession).not.toHaveBeenCalled();
  });

  it("submits when the Create button is clicked", async () => {
    const { onCreateSession } = renderDialog();
    await screen.findByRole("dialog");
    fireEvent.change(nameInput(), { target: { value: "feat" } });
    fireEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(onCreateSession).toHaveBeenCalledTimes(1);
  });

  it("returns focus to the Source trigger after the picker closes", async () => {
    renderDialog();
    await screen.findByRole("dialog");
    fireEvent.click(sourceTrigger());
    fireEvent.click(screen.getByText("pick-source"));
    await waitFor(() => expect(document.activeElement).toBe(sourceTrigger()));
  });
});
```

Notes for the implementer:
- The "plain Enter in Name" test is a weak guard in jsdom (jsdom does not perform implicit form submission on Enter anyway), but it documents intent and catches a regression where someone wires Enter to submit.
- If the focus-return test is flaky under jsdom's portal/rAF timing, increase the `waitFor` timeout (`{ timeout: 1000 }`). If it remains unreliable, mark it with `it.skip` and add a code comment that focus-return is verified manually in-browser (per the spec's testing section) — do **not** delete the assertion.

- [ ] **Step 2: Run the test to verify it passes**

Run: `pnpm test src/components/NewSessionDialog/index.test.tsx`
Expected: PASS (8 tests; or 7 pass + 1 skipped if focus-return is skipped per the note).

- [ ] **Step 3: Commit**

```bash
git add web/src/components/NewSessionDialog/index.test.tsx
git commit -m "test(new-session): cover keyboard open/submit/focus behaviors"
```

---

## Task 6: Desktop footer hint for the submit chord

**Files:**
- Modify: `web/src/components/NewSessionDialog/index.tsx`

- [ ] **Step 1: Add the `isMac` import and `useViewport`**

In `index.tsx`, add to imports:

```tsx
import { isMac } from "@/lib/device";
```

`useViewport` is already imported and `isMobile` is already destructured (`const { isMobile } = useViewport();` around `index.tsx:59`). If for some reason it is not destructured, add `isMobile` to that call.

- [ ] **Step 2: Render the hint in the footer**

Find the `DialogFooter` (around `index.tsx:224-231`):

```tsx
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={!name.trim()}>
                Create
              </Button>
            </DialogFooter>
```

Replace it with:

```tsx
            <DialogFooter className="sm:items-center sm:justify-between">
              {!isMobile && (
                <span className="text-muted-foreground hidden text-xs sm:inline-flex sm:items-center sm:gap-1">
                  <kbd className="bg-muted rounded px-1.5 py-0.5">
                    {isMac() ? "⌘↵" : "Ctrl ↵"}
                  </kbd>
                  create
                </span>
              )}
              <div className="flex gap-2">
                <Button type="button" variant="outline" onClick={onClose}>
                  Cancel
                </Button>
                <Button type="submit" disabled={!name.trim()}>
                  Create
                </Button>
              </div>
            </DialogFooter>
```

Note: `DialogFooter` (shadcn) defaults to `flex-col-reverse sm:flex-row sm:justify-end`. The `sm:justify-between` override pushes the hint to the left and the buttons to the right on desktop. If the buttons misalign in your checkout, inspect `web/src/components/ui/dialog.tsx`'s `DialogFooter` classes and adjust the wrapper to keep the two `Button`s grouped on the right.

- [ ] **Step 3: Typecheck and confirm the integration tests still pass**

Run: `pnpm build`
Expected: PASS.

Run: `pnpm test src/components/NewSessionDialog/index.test.tsx`
Expected: PASS — note the Create-button test uses `getByRole("button", { name: "Create" })`, which still resolves uniquely (the hint is a `span`/`kbd`, not a button).

- [ ] **Step 4: Commit**

```bash
git add web/src/components/NewSessionDialog/index.tsx
git commit -m "feat(new-session): show Cmd/Ctrl+Enter submit hint in footer"
```

---

## Task 7: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full test suite**

Run (from `web/`): `pnpm test`
Expected: PASS — all suites, including the two new test files.

- [ ] **Step 2: Run the typecheck/build**

Run (from `web/`): `pnpm build`
Expected: PASS — no TypeScript errors, Vite build succeeds.

- [ ] **Step 3: Manual in-browser verification (keyboard-only)**

Start the dev stack if not running (see repo `Procfile`/`Makefile`), open the web app, then with the keyboard only:

1. Press `⌘;` then `n` (or `Ctrl ;` then `n`) — dialog opens, focus on Name.
2. Type a name. Press Enter — **nothing submits** (confirms Enter is suppressed).
3. Tab to **Source**, press **Enter** (then repeat with **Space**) — the Source picker opens each time. Pick a folder. Focus returns to the **Source** trigger.
4. If the source is git-backed, Tab to **Branch**, press Enter, pick/type a branch — focus returns to the **Branch** trigger.
5. Tab to **Profile**, choose with the keyboard. Tab to **Auto-approve**, toggle with Space.
6. Press `⌘/Ctrl+Enter` — the session is created. Confirm the footer shows the `⌘↵` / `Ctrl ↵` hint on desktop.
7. Re-open and confirm Tab can also reach **Create** and submit with Enter/Space.

- [ ] **Step 4: Final commit (if any manual fixes were needed)**

```bash
git add -A
git commit -m "fix(new-session): polish keyboard flow after manual verification"
```

(Skip if no changes were required.)

---

## Self-Review (completed by plan author)

**Spec coverage:**
- Keyboard-openable Source/Branch → Tasks 1–2 (button trigger). ✓
- Focus returns to field just set → Task 3. ✓
- Enter never submits; ⌘/Ctrl+Enter submits; Create button submits → Task 4. ✓
- Desktop-only footer hint → Task 6. ✓
- Natural Tab order, Name autofocus retained → preserved by Task 2 (no tabIndex changes; `autoFocus` left intact). ✓
- Tests (open/submit/focus) → Task 5; component tests → Task 1. ✓
- Non-goals (no inline combobox, no reorder, no API change) → respected; only the listed files change. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code.

**Type consistency:** `PickerTriggerField` props (`label`, `value`, `placeholder`, `onOpen`, `optional`, `open`) are defined in Task 1 and used identically in Task 2. `sourceTriggerRef`/`branchTriggerRef` (`RefObject<HTMLButtonElement>`) defined in Task 2, consumed by `restoreFocus` in Task 3. `handleSubmit` signature unchanged. Test helper names (`sourceTrigger`, `nameInput`) are self-contained within Task 5.
