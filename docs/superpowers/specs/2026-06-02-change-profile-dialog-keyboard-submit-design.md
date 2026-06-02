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
  the handler). No keyboard library — all native React keydown handling.

### Why Cmd/Ctrl+Enter (not plain Enter)

The "plain Enter" precedent applies to custom pickers built on a free text
input. `ChangeProfileDialog` instead uses a Radix `Select`, where plain Enter
already opens the dropdown and selects the highlighted option. A dialog-level
plain-Enter handler would collide with that. Radix `Select` does **not** treat
Cmd/Ctrl+Enter specially — it folds any modifier into a bare `Enter` (opening the
trigger or selecting the highlighted option) — so the dialog-level handler must
run in the capture phase and stop the shortcut from reaching the Select (see
"Keyboard handler" and "Behavior & edge cases"). With that guard in place,
Cmd/Ctrl+Enter cleanly maps to "Apply & close" and matches the established
dialog-confirm precedent (`NewSessionDialog`).

## Design

`web/src/components/ChangeProfileDialog/index.tsx` is the primary change. The
same capture-phase guard was also applied to
`web/src/components/NewSessionDialog/index.tsx`, which shares the
`DialogContent` Cmd/Ctrl+Enter pattern with a Radix `Select` (the provider
selector) inside it and was latently affected by the same dropdown interactions.

### 1. Keyboard handler

Add an `onKeyDownCapture` to `<DialogContent>` (`NewSessionDialog` uses the same
capture-phase guard):

```tsx
<DialogContent
  onKeyDownCapture={(e) => {
    // Skip events from the portaled Select dropdown (its options bubble through
    // the React tree but live outside this DOM subtree). Returning early — before
    // stopPropagation — lets Radix commit the highlighted option normally.
    if (!e.currentTarget.contains(e.target as Node)) return;
    if (e.nativeEvent.isComposing) return;          // IME guard
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      e.stopPropagation();                          // keep the focused trigger from opening
      if (!unchanged) handleApply();                // mirror the button's disabled state
    }
  }}
>
```

- `(e.metaKey || e.ctrlKey)` covers macOS and Win/Linux, matching every other
  handler in the repo.
- The `!unchanged` guard makes Cmd/Ctrl+Enter a no-op when the selection has not
  changed — identical to the disabled `Apply` button.
- **Capture phase + `stopPropagation`** are required because the Select trigger
  is focused on open and Radix opens the dropdown on *any* `Enter` (it does not
  exclude modifier keys). Running in capture and stopping propagation prevents an
  unchanged Cmd/Ctrl+Enter from popping the dropdown open instead of no-op'ing.
- The `contains(e.target)` guard prevents a different failure: portaled option
  keydowns bubble to `DialogContent`, where the still-stale `selected` closure
  would otherwise apply the *previous* value. See "Behavior & edge cases".
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
- **While the Select dropdown is open**, its options render in a portal but
  React synthetic events still bubble through the React tree, so the keydown
  *does* reach `DialogContent`. The handler guards against this with
  `if (!e.currentTarget.contains(e.target as Node)) return;`, which ignores
  keydowns originating in the portaled dropdown. Without the guard, Radix only
  *schedules* the new selection, so Cmd/Ctrl+Enter would apply the previous
  value and close. With the guard, the Select's own Enter (pick highlighted
  option) keeps working, and the user presses Cmd/Ctrl+Enter again once the
  dropdown closes.
- **Unchanged selection**: Cmd/Ctrl+Enter is a no-op, consistent with the
  disabled button. The Select trigger is focused on open and Radix opens the
  dropdown on *any* `Enter` (including with a modifier), so the handler runs in
  the **capture phase** — a bubble handler would fire after the trigger has
  already opened. In capture, calling `preventDefault()` before the trigger's
  own handler is enough to suppress the open (Radix composes its handler to skip
  opening once the event is `defaultPrevented`); `stopPropagation()` is kept as a
  version-independent hard stop.
- **Radix `Select` does respond to `Enter`** — it treats Cmd/Ctrl+Enter the same
  as a bare `Enter` (no modifier exclusion), opening the trigger or selecting the
  highlighted option. The capture-phase handler is what keeps that from
  conflicting with the dialog's Apply shortcut.

## Testing

Automated tests cover the keyboard behavior (`ChangeProfileDialog/index.test.tsx`,
`NewSessionDialog/index.test.tsx`), including the apply/no-op/plain-Enter cases
and two regressions that fail without the capture-phase guard: applying a stale
profile via an open dropdown, and an unchanged Cmd/Ctrl+Enter popping the
dropdown open. Also verify manually in the running app:

1. Open the Change Profile dialog, change the selection, press Cmd/Ctrl+Enter →
   profile applies and the dialog closes.
2. With the selection unchanged, Cmd/Ctrl+Enter does nothing (no dropdown).
3. Escape still closes the dialog.
4. The `⌘ ↵` / `Ctrl ↵` hint renders on desktop and is hidden on mobile.

## Out of scope

- Plain-Enter submission (rejected to avoid the Radix `Select` conflict).
- Changes to dialogs other than Change Profile and New Session, or to shared UI
  components.
