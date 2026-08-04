# Mobile compose bar visual refresh

Date: 2026-08-04
Branch: `jeev/mobile-persistent-input`
Follows: `2026-08-02-mobile-persistent-input-design.md`

## Problem

The persistent compose bar works — growth, the three-line cap, the terminal
shift, and draft survival are all verified in a real mobile browser. It just
does not look considered. Four things, visible the moment two states are put
side by side:

1. **No hierarchy in the actions.** Attach and Send carry the identical
   treatment — an `h-8 w-8` circle with a `border-white/20` ring. The primary
   action has no more weight than the secondary one, and both read as generic
   form controls rather than choices.
2. **The menu affordance is invisible.** The `h-1 w-1` `bg-white/30` dot at the
   corner of `more` and `ctrl` renders as roughly one physical pixel. It looks
   like dust, not like "this key opens a menu."
3. **Three stacked hairlines.** The tmux status bar's edge, the compose panel's
   `border-t`, and the toolbar's `border-t` pile up inside ~90px, none of them
   evidently more important than the others.
4. **Everything occupies one narrow value band.** `white/5` through `white/30`
   for every border, glyph and marker, so nothing leads the eye.

## Goals

- Give the input zone a clear visual hierarchy: primary action, secondary
  action, quiet chrome.
- Make the menu affordance legible at arm's length on a phone.
- Reduce the stacked-rule noise at the bottom of the screen.
- Improve draft legibility.

## Non-goals

- **No layout or interaction change.** The spacer / absolute panel / transform
  architecture stays exactly as it is. Growth still never refits the terminal.
- **No space reclamation.** The compose + toolbar chrome keeps its current
  footprint (see "Height impact" for the one unavoidable exception).
- Desktop is untouched — all of this is inside the `isMobile` branch.

## Decisions

### Surface: one zone, one edge

Compose panel and toolbar share a single background at `hsl(0 0% 8%)`. That
value sits deliberately between `--background` (4%) and `--secondary` (10%): the
zone reads as a distinct surface without becoming a slab.

The zone gets one edge — the compose panel's `border-t` at `hsl(0 0% 16%)`,
near `--border` (14%). The toolbar's own `border-t` becomes transparent, since
the shared surface already groups the two rows.

Replaces: compose `border-white/10` + `bg-transparent` / `bg-secondary/50`, and
toolbar `bg-secondary/50 border-white/5`.

### Actions: weight follows importance

| | fill | glyph |
|---|---|---|
| Attach | `hsl(0 0% 16%)`, no ring | `hsl(0 0% 76%)` |
| Send, live | `--primary` (217 91% 60%) | white |
| Send, disabled | `hsl(0 0% 18%)` | `hsl(0 0% 48%)` |

Disabled Send is a **solid dim fill at full opacity**, not `opacity-30` on an
outline. A ghost of a ghost reads as broken; a filled-but-dim control reads as
"present, not yet available."

### Toolbar: legible affordances

Key labels lift to `hsl(0 0% 72%)`.

The menu marker changes from a 1×1px corner dot to a **10×1.5px underline**,
centred beneath the label at `hsl(0 0% 45%)`. This is the single highest
ratio of legibility gained to pixels changed in the whole refresh.

### Type: 15px draft

Draft text goes 14px → 15px with an **explicit, shared 21px line-height** on
both the mirror and the textarea. The line-height must be explicit rather than
inherited: the cap derivation below depends on it, and the mirror and textarea
must wrap identically or the panel sizes to the wrong number of lines.

### Focus state

Today focus is signalled four ways: background `transparent → secondary/50`,
border `white/10 → white/25`, the placeholder swap, and Send appearing. The
solid surface removes the background delta.

The remaining cue is the **zone edge brightening `hsl(0 0% 16%) → hsl(0 0% 24%)`**,
behind the placeholder swap and Send appearing, which the original spec already
identified as the signals that actually carry. One property animates; the whole
zone never changes value.

## The two derived constants

The type change is not free. Both of these were measured in a real browser at
390×844, not assumed.

### `--compose-max-h`

```
calc(3 * 1.25rem + 0.75rem)  =  72px     (3 * 20px line-height + py-1.5)
calc(3 * 21px   + 0.75rem)   =  75px     (3 * 21px line-height + py-1.5)
```

Verified: the mirror caps at exactly 75px and the textarea scrolls at 117px
rather than clipping.

### The spacer height

`h-11` (44px) is not an arbitrary pill of space — it is **the one-line panel
height**, and the panel is anchored to its bottom edge. If the panel's resting
height and the spacer disagree, the panel permanently overflows, the observer
reports a nonzero overlay at rest, and the terminal carries a constant shift.
That is a silent regression of the exact invariant the parent design exists to
protect.

At 15px/21px text the resting panel measures — note `py-1.5` appears at two
different levels here, on the mirror and again on the panel:

```
row height   = 21px line-height + 12px (mirror's own py-1.5)   = 33px
panel height = 6px + 33px + 6px (panel's py-1.5) + 1px border  = 46px
```

So the spacer moves `h-11` (44px) → 46px, and the pair must be kept in lockstep.

(The textarea's `min-h-8` = 32px stops binding once the row is 33px. It is
harmless, but it is no longer what sets the collapsed height — the mirror is.)

**Test:** assert that the resting panel height equals the spacer height. This is
the assertion that would have caught the drift, and it is cheap — the existing
`MockResizeObserver` seam already drives panel heights by hand.

### Height impact

Keeping `py-1.5` rather than widening the panel padding holds the growth to
**+2px of resting chrome** (44 → 46), entirely from the larger text. Widening
the padding to 8px would look marginally more open but costs +6px. Given that
space usage was explicitly scoped out of this pass, the recommendation is to
keep `py-1.5` and accept +2px.

## Files

| File | Change |
|---|---|
| `web/src/components/Terminal/ComposeBar.tsx` | surface + edge, button fills, 15px/21px type, `--compose-max-h` 72 → 75, spacer `h-11` → 46px, focus edge brightening |
| `web/src/components/Terminal/TerminalToolbar.tsx` | shared surface, transparent `border-t`, label colour, dot → underline |
| `web/src/components/Terminal/ComposeBar.test.tsx` | cap assertion updated to 75px; new resting-panel-equals-spacer assertion |

## Testing

Existing coverage that must keep passing unchanged: growth clamps at three lines
then scrolls, send clears / preserves the draft, placeholder and Send visibility
swap on focus and blur, draft text is never dimmed.

New:

- resting panel height equals the spacer height (the drift guard)
- `--compose-max-h` derivation matches `3 * line-height + padding` rather than a
  literal
- mirror and textarea still carry identical sizing and typography classes — the
  existing drift test, now covering `font-size` and `line-height` explicitly

Visual verification repeats the browser pass already established: emulated
iPhone at 390×844 with `pointer: coarse`, driving the growth range and reading
back panel/mirror/shift geometry plus the resize-frame count.
