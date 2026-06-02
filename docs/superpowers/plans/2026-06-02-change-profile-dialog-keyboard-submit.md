# Change Profile dialog: Cmd/Ctrl+Enter submit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users apply-and-close the Change Profile dialog with Cmd/Ctrl+Enter, with a discoverable keyboard hint on the Apply button.

**Architecture:** Add a single `onKeyDown` handler to the dialog's `DialogContent` that runs the existing `handleApply()` on Cmd/Ctrl+Enter (no-op when the selection is unchanged), plus a `<kbd>` hint on the Apply button. One source file changes; one new test file is added. No new dependencies.

**Tech Stack:** React + TypeScript, Radix UI Dialog/Select, Vitest + @testing-library/react (`fireEvent`, no `user-event`), Tailwind.

**Spec:** `docs/superpowers/specs/2026-06-02-change-profile-dialog-keyboard-submit-design.md`

---

## File Structure

- **Modify:** `web/src/components/ChangeProfileDialog/index.tsx` — add the keydown handler and the Apply-button hint; add two imports and one hook call.
- **Create:** `web/src/components/ChangeProfileDialog/index.test.tsx` — Vitest coverage for the keyboard behavior and the hint.

All commands run from the `web/` directory (the front-end package). The repo uses **pnpm**.

---

## Task 1: Apply & close on Cmd/Ctrl+Enter

**Files:**
- Create: `web/src/components/ChangeProfileDialog/index.test.tsx`
- Modify: `web/src/components/ChangeProfileDialog/index.tsx` (add `onKeyDown` to `<DialogContent>`, currently at `index.tsx:61`)

- [ ] **Step 1: Write the failing test file**

Create `web/src/components/ChangeProfileDialog/index.test.tsx` with this exact content. The `Session` factory and the three jsdom polyfills mirror `SessionInfoDialog/index.test.tsx`; the mock style mirrors `NewSessionDialog/index.test.tsx`. (The `useViewport` mock is unused until Task 2 but is harmless here.)

```tsx
import { describe, it, expect, vi, beforeAll, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  cleanup,
} from "@testing-library/react";
import { ChangeProfileDialog } from "./index";
import type { Session } from "@/types";

// --- Mock data + viewport hooks ---
vi.mock("@/data/sessions", () => ({
  useProfilesQuery: () => ({ data: { profiles: ["default", "review"] } }),
}));
vi.mock("@/hooks/useViewport", () => ({
  useViewport: () => ({ isMobile: false, isDesktop: true, isHydrated: true }),
}));

// Radix Select renders internals that depend on these jsdom-missing APIs.
beforeAll(() => {
  if (!("ResizeObserver" in globalThis)) {
    globalThis.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
  }
  if (!Element.prototype.hasPointerCapture) {
    Element.prototype.hasPointerCapture = () => false;
  }
  if (!Element.prototype.scrollIntoView) {
    Element.prototype.scrollIntoView = () => {};
  }
});

afterEach(cleanup);

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "sess-123",
    name: "My Session",
    tmux_name: "claude-sess-123",
    created_at: "2026-01-01 00:00:00",
    updated_at: "2026-01-01 00:00:00",
    working_directory: "/home/user/project",
    worktree_branch: null,
    git_parent_dir: null,
    git_remote_url: null,
    provider_session_id: null,
    model: null,
    system_prompt: null,
    provider_type: "claude",
    auto_approve: false,
    profile: null,
    pinned: false,
    ...overrides,
  };
}

function renderDialog(sessionOverrides: Partial<Session> = {}) {
  const onApply = vi.fn();
  const onClose = vi.fn();
  render(
    <ChangeProfileDialog
      session={makeSession(sessionOverrides)}
      onClose={onClose}
      onApply={onApply}
    />,
  );
  return { onApply, onClose };
}

// Open the Radix Select and pick an option by visible name.
async function selectProfile(name: string) {
  fireEvent.click(screen.getByRole("combobox"));
  fireEvent.click(await screen.findByRole("option", { name }));
}

describe("ChangeProfileDialog keyboard submit", () => {
  it("does not apply on Cmd+Enter when the selection is unchanged", async () => {
    const { onApply, onClose } = renderDialog({ profile: null });
    await screen.findByRole("dialog");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onApply).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it("applies the changed profile and closes on Cmd+Enter", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      metaKey: true,
    });
    expect(onApply).toHaveBeenCalledWith("sess-1", "default");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("applies on Ctrl+Enter as well", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("review");
    fireEvent.keyDown(screen.getByRole("dialog"), {
      key: "Enter",
      ctrlKey: true,
    });
    expect(onApply).toHaveBeenCalledWith("sess-1", "review");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("ignores plain Enter (no modifier)", async () => {
    const { onApply, onClose } = renderDialog({ id: "sess-1", profile: null });
    await screen.findByRole("dialog");
    await selectProfile("default");
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Enter" });
    expect(onApply).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `pnpm exec vitest run src/components/ChangeProfileDialog/index.test.tsx`
Expected: FAIL — the "applies … on Cmd+Enter" and "Ctrl+Enter" tests fail because no keydown handler exists yet, so `onApply`/`onClose` are never called. (The "does not apply" and "ignores plain Enter" tests may already pass; that's fine.)

- [ ] **Step 3: Add the keydown handler**

In `web/src/components/ChangeProfileDialog/index.tsx`, change the opening `<DialogContent>` tag (currently `index.tsx:61`) from:

```tsx
      <DialogContent>
```

to:

```tsx
      <DialogContent
        onKeyDown={(e) => {
          if (e.nativeEvent.isComposing) return;
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            if (!unchanged) handleApply();
          }
        }}
      >
```

`unchanged` (`index.tsx:57`) and `handleApply` (`index.tsx:50`) are already defined in the component scope, so the handler closes over them directly. `handleApply` already calls `onApply(...)` then `onClose()`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `pnpm exec vitest run src/components/ChangeProfileDialog/index.test.tsx`
Expected: PASS (4 passed).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/ChangeProfileDialog/index.tsx web/src/components/ChangeProfileDialog/index.test.tsx
git commit -m "feat(change-profile): apply & close on Cmd/Ctrl+Enter"
```

---

## Task 2: Show the Cmd/Ctrl+Enter hint on the Apply button

**Files:**
- Modify: `web/src/components/ChangeProfileDialog/index.tsx` (imports at top; `useViewport()` call in the component body; the Apply `<Button>` at `index.tsx:93-95`)
- Modify: `web/src/components/ChangeProfileDialog/index.test.tsx` (add one test)

- [ ] **Step 1: Write the failing test**

Append this test inside the existing `describe("ChangeProfileDialog keyboard submit", ...)` block in `web/src/components/ChangeProfileDialog/index.test.tsx`:

```tsx
  it("renders the Cmd/Ctrl+Enter hint on the Apply button (desktop)", async () => {
    renderDialog({ profile: null });
    await screen.findByRole("dialog");
    // The kbd hint is aria-hidden, so the button's accessible name stays "Apply".
    const apply = screen.getByRole("button", { name: "Apply" });
    expect(apply.textContent).toMatch(/↵/);
  });
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `pnpm exec vitest run src/components/ChangeProfileDialog/index.test.tsx -t "renders the Cmd/Ctrl+Enter hint"`
Expected: FAIL — the Apply button has no `↵` hint yet (`textContent` is just "Apply").

- [ ] **Step 3: Add the imports and the `useViewport` call**

In `web/src/components/ChangeProfileDialog/index.tsx`, add two imports. After the existing `import type { Session } from "@/types";` (`index.tsx:18`), add:

```tsx
import { isMac } from "@/lib/device";
import { useViewport } from "@/hooks/useViewport";
```

Then, inside the `ChangeProfileDialog` component body, just after `const profiles = profilesData?.profiles ?? [];` (`index.tsx:37`), add:

```tsx
  const { isMobile } = useViewport();
```

- [ ] **Step 4: Add the hint to the Apply button**

In the same file, change the Apply `<Button>` (currently `index.tsx:93-95`) from:

```tsx
          <Button type="button" onClick={handleApply} disabled={unchanged}>
            Apply
          </Button>
```

to:

```tsx
          <Button type="button" onClick={handleApply} disabled={unchanged}>
            Apply
            {!isMobile && (
              <kbd
                aria-hidden="true"
                className="bg-primary-foreground/15 hidden rounded px-1 py-0.5 text-[10px] sm:inline-block"
              >
                {isMac() ? "⌘ ↵" : "Ctrl ↵"}
              </kbd>
            )}
          </Button>
```

This matches `NewSessionDialog/index.tsx:245-255` exactly (the dialog-footer pill style).

- [ ] **Step 5: Run the test to verify it passes**

Run: `pnpm exec vitest run src/components/ChangeProfileDialog/index.test.tsx`
Expected: PASS (5 passed).

- [ ] **Step 6: Lint/typecheck the changed file**

Run: `pnpm exec tsc --noEmit` (from `web/`)
Expected: no errors. If the repo exposes a lint script, also run `pnpm lint`.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/ChangeProfileDialog/index.tsx web/src/components/ChangeProfileDialog/index.test.tsx
git commit -m "feat(change-profile): show Cmd/Ctrl+Enter hint on Apply button"
```

---

## Manual verification (after both tasks)

Per the spec, confirm in the running app:

1. Open the Change Profile dialog, change the selection, press Cmd/Ctrl+Enter → profile applies and the dialog closes.
2. With the selection unchanged, Cmd/Ctrl+Enter does nothing.
3. Escape still closes the dialog (Radix built-in, unchanged).
4. The `⌘ ↵` / `Ctrl ↵` hint renders on desktop and is hidden on mobile.

---

## Self-Review Notes

- **Spec coverage:** keydown handler (Task 1) covers the Cmd/Ctrl+Enter apply-and-close + unchanged no-op + plain-Enter-ignored requirements; the button hint (Task 2) covers the discoverability requirement matching `NewSessionDialog`. Escape-to-close is unchanged (Radix built-in) — no task needed. The "dropdown-open portal" edge case requires no code (documented behavior).
- **Type consistency:** `handleApply` and `unchanged` are referenced exactly as defined in `index.tsx`. `isMac` is a function (called), `useViewport()` returns `{ isMobile }` — both used as in `NewSessionDialog`.
- **No placeholders:** every step shows the exact code and command.
