# Keyboard-complete New Session flow (BXN-100)

## Overview

Creating a session today forces a mouse/keyboard ping-pong: open the dialog
(`⌘;` then `n`), type a Name, then **switch to the mouse** to open the Source
picker, select, switch back to the keyboard, lose focus on return, manually
find the Profile field, and finally **click Create**. The goal is to make the
entire flow keyboard-operable end to end, while fixing the focus-loss and
Enter-key ambiguity that make the current dialog feel broken.

This is a focused interaction pass over `web/src/components/NewSessionDialog/`.
**No backend, API, data-model, or leader-chord-trigger changes.** The child
pickers (`FileBrowser`, `RepoBrowser`, `BranchPicker`) already have full
internal keyboard support (`↑↓` navigate, `↵` select, `esc` close, `⇥`
complete); the gap is entirely in the parent dialog's orchestration — how
pickers are *opened*, how *focus* is handed back, and what *Enter* means.

The inline-combobox alternative (replacing the modal pickers with anchored
typeahead popovers) was considered and **deferred** — see Non-Goals.

## Goals

- Every step of the flow is reachable and operable with the keyboard alone:
  open the dialog, fill Name, open and use the Source picker, open and use the
  Branch picker, choose a Profile, toggle Auto-approve, and submit — no mouse
  required at any point.
- Source and Branch fields open their pickers via **Enter or Space** when
  focused (in addition to mouse click).
- After a picker closes (whether a value was selected or it was cancelled),
  keyboard focus **returns to the trigger field that was just used**, so the
  user keeps their place and can Tab onward or re-open.
- **Enter never submits the form.** It only operates the focused control.
  Submission is exclusively `⌘/Ctrl+Enter` from anywhere, or activating the
  Create button (which is Tab-reachable, preserving keyboard a11y).
- A subtle desktop-only footer hint advertises the `⌘↵` / `Ctrl ↵` submit chord.
- Natural top-to-bottom Tab order across all controls, with Name retaining
  autofocus on open.

## Non-Goals

- **No inline-combobox refactor.** Source and Branch keep their existing modal
  `Dialog` pickers; we only change how those modals are opened and how focus is
  restored. Replacing them with anchored typeahead popovers is a larger refactor
  deferred to a future issue.
- **No field reordering.** Order stays Provider → Name → Source → [Branch] →
  Profile → Auto-approve. (Considered front-loading the per-session essentials;
  rejected as unnecessary layout churn for this pass.)
- **No "type-name-then-Enter" express lane.** Deliberately dropped in favor of a
  single consistent Enter rule (see Decisions).
- No backend, API, or data-model changes.
- No changes to the `⌘; n` leader-chord that opens the dialog.

## Decisions (resolved during brainstorming)

- **Picker model:** keep the existing modal `Dialog` pickers; make them
  keyboard-openable and fix focus hand-off. (Inline combobox deferred.)
- **Focus on close:** focus **returns to the field just set** (Source/Branch
  trigger), not auto-advance to the next field. Predictable; the user Tabs
  onward themselves.
- **Submit model:** Enter never submits; it only operates the focused control.
  Submit is `⌘/Ctrl+Enter` or the Create button only. This removes Enter's
  focus-dependent overloading (submit in Name vs. open-picker on Source vs.
  choose-option in Profile), which was the root of the perceived
  inconsistency. The implicit HTML form-submit on Enter in the Name field is
  suppressed with `preventDefault`.
- **Field order:** unchanged.

## Design

All changes live in `web/src/components/NewSessionDialog/`. The current
`index.tsx` is ~280 lines; the picker-trigger markup is duplicated between
Source and Branch, so it is extracted into a small reusable component.

### 1. Source & Branch become trigger buttons (`PickerTriggerField`)

Today Source and Branch render as `<Input readOnly onClick={...}>`. A readonly
input inside a `<form>` can be focused by Tab but cannot be activated by
keyboard, and pressing Enter on it triggers the form's implicit submit. Both
problems disappear by rendering a real button instead.

New component `PickerTriggerField` (`NewSessionDialog/PickerTriggerField.tsx`):

- Props: `{ label: string; optional?: boolean; value: string; placeholder:
  string; onOpen: () => void; }`, plus a forwarded `ref` to the underlying
  `<button>`.
- Renders the field label (with an optional muted `(optional)` tag, matching the
  current Branch label) and a `<button type="button">` styled to visually match
  the existing `Input` (same border, height, padding, rounded corners; muted
  text when showing the placeholder, normal text when showing a value; left
  aligned).
- `type="button"` guarantees it never submits the form.
- `onClick={onOpen}` handles mouse; Enter/Space activation is the browser's
  native button behavior and also calls `onOpen` — no extra key handling needed.
- Accessibility: `aria-haspopup="dialog"` and `aria-expanded={open}` so it reads
  as a picker trigger.

Source and Branch in `index.tsx` both render `PickerTriggerField`, passing the
relevant `ref` (see §2).

### 2. Focus returns to the trigger after the picker closes

Add two refs in `index.tsx`: `sourceTriggerRef` and `branchTriggerRef`, passed
to the respective `PickerTriggerField`s.

Each picker already funnels both "selected" and "cancelled" exits through a
single close path in the parent:

- **SourcePicker:** `SourcePicker` calls `onSelect(...)` *then* `onOpenChange(false)`
  on selection, and `onOpenChange(false)` on cancel/escape. So the parent's
  `onOpenChange(false)` handler is the single choke point.
- **BranchPicker:** both `onSelect` and `onClose` route through the parent's
  `closeBranchPicker()` (and the `BranchDialog` `onOpenChange(false)`).

In each close handler, after setting the picker's `open` state to `false`,
schedule a focus restore to the corresponding trigger ref using
`requestAnimationFrame` (or `setTimeout(…, 0)`), so it runs after the child
`Dialog` has torn down and Radix has finished its own focus management. This
mirrors the existing `setTimeout(() => inputRef.current?.focus(), 50)` pattern
the pickers already use.

The existing `childPickerClosingRef` guard — which prevents the *outer* dialog
from closing when a child picker's outside-pointer/focus events fire — stays
exactly as-is. The focus restore is additive and does not interact with it.

### 3. Submit behavior

- Remove the current hidden `Shift+Enter` handler in `DialogContent`'s
  `onKeyDown`.
- Replace it with: `if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
  e.preventDefault(); handleSubmit(e); }`.
- Suppress implicit form submit from the Name field: in the Name `<Input>`'s
  `onKeyDown`, if `e.key === "Enter"` (without meta/ctrl), `e.preventDefault()`
  so it neither submits nor bubbles to a submit. (Equivalently, the `<form>`'s
  `onSubmit` only ever runs via the explicit chord or the Create button.)
- The Create button stays `type="submit"`; clicking it, or focusing it and
  pressing Enter/Space, submits as normal. This keeps the form keyboard
  submittable via Tab → Create for users who don't know the chord.
- `handleSubmit` keeps its existing `if (!trimmedName) return` guard, so the
  chord is a no-op when Name is empty.
- Profile (`Select`) and Auto-approve (`Switch`) are Radix primitives that are
  already keyboard-operable (Enter/Space/arrows) and consume their own Enter —
  no change.

### 4. Discoverability — footer hint

Add a subtle, desktop-only hint in the dialog footer advertising the submit
chord, using the same `kbd` styling the child pickers use
(`bg-muted rounded px-1.5 py-0.5`). Show `⌘ ↵` on macOS and `Ctrl ↵` elsewhere
via the existing `isMac()` helper. Hidden on mobile (no physical keyboard / no
chord). Label: e.g. `⌘↵ create`.

### 5. Tab order

With Source/Branch as buttons, every control is natively focusable and operable
in DOM order: Provider (`Select`) → Name (`Input`, autofocused) → Source
(button) → [Branch button, when shown] → Profile (`Select`) → Auto-approve
(`Switch`) → Cancel → Create. No `tabIndex` overrides are needed. Name keeps its
`autoFocus`.

### Edge cases

- **Branch field is conditional** (`showBranchField`). When hidden, Tab flows
  Source → Profile naturally; the Branch ref is simply never focused. When the
  field appears/disappears due to source/provider changes, focus is not forcibly
  moved (matches today's behavior).
- **No profiles** (`profiles.length === 0`): Profile field absent; Tab flows to
  Auto-approve. Unaffected.
- **Mobile:** trigger buttons work with tap exactly like the old readonly inputs;
  the footer chord hint is hidden. Child pickers already have their mobile
  layouts.

## Testing

Add `web/src/components/NewSessionDialog/index.test.tsx` (Vitest +
Testing Library, matching the style of `SessionInfoDialog/index.test.tsx`):

- Pressing **Enter** on the focused Source trigger opens the Source picker and
  does **not** call `onCreateSession`.
- Pressing **Space** on the focused Source trigger opens the picker.
- **`⌘/Ctrl+Enter`** calls `onCreateSession` once with the trimmed name when Name
  is non-empty, and is a **no-op** when Name is empty.
- Pressing **Enter** in the Name field does **not** call `onCreateSession`.
- Clicking **Create** (and activating it via keyboard) submits with the expected
  params.
- The Source/Branch triggers expose an accessible role/name
  (`aria-haspopup`, label association).

Focus-return assertions are included where jsdom + Radix portal teardown allow
reliable verification; where jsdom cannot faithfully reproduce the
portal/focus timing, the behavior is called out for manual verification in the
browser (consistent with repo precedent of verifying presentational/focus
behavior in-browser rather than over-investing in brittle jsdom focus tests).

## Files

- `web/src/components/NewSessionDialog/PickerTriggerField.tsx` — **new**;
  label + styled trigger `<button>`, forwarded ref.
- `web/src/components/NewSessionDialog/index.tsx` — use `PickerTriggerField` for
  Source/Branch; add trigger refs + focus restore on picker close; rework
  `onKeyDown` (remove `Shift+Enter`, add `⌘/Ctrl+Enter` submit); suppress Enter
  in the Name input; add the desktop footer chord hint.
- `web/src/components/NewSessionDialog/index.test.tsx` — **new**; tests above.
