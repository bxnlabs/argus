# Change Profile dialog: Cmd/Ctrl+Enter to Apply & close

## Goal

Let users confirm the Change Profile dialog from the keyboard. Pressing
**Cmd+Enter** (macOS) or **Ctrl+Enter** (Windows/Linux) applies the selected
profile and closes the dialog, mirroring the disabled-when-unchanged behavior of
the existing `Apply` button. A keyboard hint on the button makes the shortcut
discoverable.

## Context

`web/src/components/ChangeProfileDialog/index.tsx` is a small dialog containing a
single Radix `Select` (profile picker, including a "None (detach)" sentinel),
plus `Cancel` and `Apply` buttons. `Apply` calls `handleApply()`, which runs
`onApply(...)` then `onClose()`, and is disabled when the selection is unchanged.
The dialog currently has no custom keyboard handling — it relies on Radix
built-ins (Escape closes; arrows/Enter operate the Select).

### Codebase precedent (audited)

The repo has a consistent two-branch convention:

- **Multi-line textareas** submit on **Cmd/Ctrl+Enter**; plain Enter inserts a
  newline. A keyboard hint is shown on the submit button.
  - `NewSessionDialog` (`web/src/components/NewSessionDialog/index.tsx:153-159`,
    `:245-255`)
  - `InlineCommentForm` (`web/src/components/DiffViewer/InlineCommentForm.tsx:20-29`,
    `:46-56`)
  - `ReviewSubmitButton` (`web/src/components/GitPanel/ReviewSubmitButton.tsx`)
- **Custom single-line pickers** (text input + list) confirm on **plain Enter**:
  `BranchPicker`, `FileBrowser`, `RepoBrowser`, `QuickSwitcher`, `SessionList`
  rename.
- **All dialogs** close on **Escape** (Radix built-in).
- Platform handling is always `e.metaKey || e.ctrlKey` (no per-OS branching in
  the handler). No keyboard library — all native `onKeyDown`.

### Why Cmd/Ctrl+Enter (not plain Enter)

The "plain Enter" precedent applies to custom pickers built on a free text
input. `ChangeProfileDialog` instead uses a Radix `Select`, where plain Enter
already opens the dropdown and selects the highlighted option. A dialog-level
plain-Enter handler would collide with that. **Cmd/Ctrl+Enter is unused by the
Select**, so it cleanly maps to "Apply & close" and matches the established
dialog-confirm precedent (`NewSessionDialog`).

## Design

Only `web/src/components/ChangeProfileDialog/index.tsx` changes.

### 1. Keyboard handler

Add an `onKeyDown` to `<DialogContent>`, following
`NewSessionDialog/index.tsx:153-159`:

```tsx
<DialogContent
  onKeyDown={(e) => {
    if (e.nativeEvent.isComposing) return;        // IME guard, matches precedent
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (!unchanged) handleApply();              // mirror the button's disabled state
    }
  }}
>
```

- `(e.metaKey || e.ctrlKey)` covers macOS and Win/Linux, matching every other
  handler in the repo.
- The `!unchanged` guard makes Cmd/Ctrl+Enter a no-op when the selection has not
  changed — identical to the disabled `Apply` button.
- `handleApply` already calls `onApply` then `onClose`, so "Apply & close" needs
  no extra wiring.

### 2. Button hint

Add a keyboard hint to the `Apply` button, matching the dialog-footer pill style
of `NewSessionDialog/index.tsx:245-255`:

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

New imports:

- `isMac` from `@/lib/device`
- `useViewport` from `@/hooks/useViewport` (destructure `isMobile`)

Both `NewSessionDialog` and `InlineCommentForm` render the same `⌘ ↵` / `Ctrl ↵`
glyph via `isMac()`. Their `<kbd>` styling differs — `NewSessionDialog` uses a
filled pill hidden on mobile; `InlineCommentForm` uses plain dim text always
shown. Because Change Profile is a dialog with the same `DialogFooter` and
default-size primary `Button` as New Session, we match the `NewSessionDialog`
pill style for visual consistency between the two dialogs.

## Behavior & edge cases

- **Escape to close** is unchanged (Radix built-in).
- **While the Select dropdown is open**, its options render in a portal, so the
  keydown does not reach `DialogContent` and Cmd/Ctrl+Enter does not fire there.
  Acceptable: the Select's own Enter (pick highlighted option) keeps working, and
  the user can press Cmd/Ctrl+Enter again once the dropdown closes.
- **Unchanged selection**: Cmd/Ctrl+Enter is a no-op, consistent with the
  disabled button.
- **No conflict** with the Radix `Select`, which does not use Cmd/Ctrl+Enter.

## Testing

This is a contained, convention-following UI change. Verify manually in the
running app:

1. Open the Change Profile dialog, change the selection, press Cmd/Ctrl+Enter →
   profile applies and the dialog closes.
2. With the selection unchanged, Cmd/Ctrl+Enter does nothing.
3. Escape still closes the dialog.
4. The `⌘ ↵` / `Ctrl ↵` hint renders on desktop and is hidden on mobile.

No new automated test unless desired.

## Out of scope

- Plain-Enter submission (rejected to avoid the Radix `Select` conflict).
- Changes to any other dialog or to shared UI components.
