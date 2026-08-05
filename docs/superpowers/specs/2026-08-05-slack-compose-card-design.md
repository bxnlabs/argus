# Slack-style compose card

Date: 2026-08-05
Branch: `jeev/mobile-persistent-input`
Follows: `2026-08-04-compose-bar-visual-refresh-design.md`

## Problem

The compose zone reads as chrome bolted to the bottom of the terminal: two
full-bleed rows separated from the screen edges by nothing, distinguished from
the terminal only by a single top hairline. It works, and after the 2026-08-04
refresh it has a defensible hierarchy, but it does not read as a *place you
type* the way a messaging composer does.

The reference is Slack's mobile composer: a rounded card inset from the screen
edges, a quiet placeholder naming the destination (`Message #channel`), and a
row of ghost glyph actions beneath the text with send pinned right.

Two specific gaps against that reference:

1. **No card.** The zone has one edge, at the top. Slack's has four, plus
   margin on all sides, which is what makes it read as a surface you address
   rather than a strip of UI.
2. **The placeholder says nothing and says it loudly.** `Tap to compose` /
   `Message…` names no destination and renders at the browser's default
   placeholder value, which is more prominent than Slack's.

## Goals

- The input zone reads as one rounded card, inset from the screen edges.
- The placeholder names the session, Slack-style, and recedes.
- Actions adopt Slack's ghost-glyph treatment.

## Non-goals

- **No change to the growth architecture.** The spacer / absolutely-positioned
  panel / terminal-transform design from the parent spec stays exactly as it
  is, and growing the draft still never refits the terminal.
- Desktop is untouched — all of this is inside the `isMobile` branch.
- The compose row and the key toolbar stay as two rows. Merging them into a
  single Slack-style action row was considered and rejected: the toolbar
  carries ten keys in a horizontal scroller, and folding attach and send into
  it would leave `return` (a raw CR to the PTY) sitting next to send (the
  composed draft), two different meanings of "enter" one thumb-width apart.

## Decisions

### The card is two flush halves, not one box

The compose panel is absolutely positioned; the toolbar is in normal flow. They
cannot share a border box. So the card is assembled from two halves that meet
on a shared edge:

| | classes |
|---|---|
| Panel | `border-x border-t rounded-t-xl`, `inset-x-3` (was `inset-x-0`) |
| Toolbar | `border-x border-b rounded-b-xl`, `mx-3 mb-2`, `border-t` at `hsl(0 0% 14%)` |

The panel sits exactly on the spacer's bottom edge and the toolbar begins
immediately after the spacer in flow, so the two halves are flush and the seam
is a single hairline — which is the row divider. The parent spec had made that
`border-t` transparent to preserve the row's height while letting the shared
surface do the grouping; inside a card the two rows need separating again, so
it takes a value. It stays a border rather than becoming a child element, so
the row height is unchanged and the divider costs nothing. `hsl(0 0% 14%)` is
`--border`, deliberately quieter than the card's own 20% edge: it separates
without competing. When the draft grows, the
panel's top corners and side borders rise with it, so the card grows upward the
way Slack's does.

Border: `hsl(0 0% 20%)` at rest, `hsl(0 0% 30%)` on focus. The rest value is up
from the parent spec's 16% because a four-sided card needs a more present edge
than a single top rule did. Surface stays `bg-input` (8%) on `--background`
(4%).

Focus now has to reach both halves. `ComposeBar` gains an `onFocusedChange`
callback and `TerminalToolbar` a `focused` prop, lifted through
`Terminal/index.tsx` — the same shape as the existing `onOverlayHeightChange`,
and `Terminal` already re-renders on that.

### Geometry: +9px, and why the spacer is untouched

Added: the toolbar's `border-b` (1px) and `mb-2` (8px). Nothing is added at the
top — the parent spec's `mt-2` becomes the card's top margin.

The bottom margin is load-bearing, not decoration. A card flush against the
bottom of the viewport has three visible edges and reads as a panel, not a
card. Whether the 9px costs a terminal row is `containerHeight mod cellHeight`,
the same arithmetic the parent spec documents for its 8px gap; it is measured
in the browser pass, not assumed.

**The spacer formula does not change.** The `ResizeObserver` reads
`contentRect.height`, which is the content box — it excludes border and
padding — so adding `border-x` contributes nothing vertically. The observer
also compares against a captured baseline, so any constant offset cancels
regardless. The existing drift guard still pins 46px.

Insetting the toolbar costs 24px of key-row width in the horizontal scroller.
Accepted.

### Placeholder: `Message #<slug>`

One string in both focus states, at `hsl(0 0% 50%)`.

That value is derived. Placeholder text is text, so it needs 4.5:1. Against the
8% card surface:

```
hsl(0 0% 45%)  ->  3.86:1   fails
hsl(0 0% 50%)  ->  4.6:1    passes
--muted-foreground (65%) -> ~7.4:1   more prominent than Slack's
```

50% is the dimmest gray that clears the bar — "less prominent" taken as far as
it can honestly go.

When there is no session (a raw-shell tab, `sessionId === null`), the
placeholder is plain `Message`.

Slugs longer than 28 characters truncate with an ellipsis. A textarea
placeholder cannot ellipsize on its own, and clipping mid-word at the input
edge looks like a bug; `review-mike-dp-host-self-registration` is a real
session name at 36 characters.

### The slug is computed server-side

Session names are not slugified — `slugify` today runs only when deriving a
worktree branch name. Rather than porting the rule to TypeScript and letting
two implementations drift, the API carries it.

`slugify` moves out of `internal/git/worktree` into a new leaf package
`internal/slug` as `slug.Make`. The worktree manager keeps its single call site
(`manager.go:214`), now via `slug.Make`; `worktreeDirName` stays where it is.
The existing `TestSlugify` table moves with the function.

A leaf package rather than exporting `worktree.Slugify`: the database layer
needs the rule, and `internal/node/db` importing `internal/git/worktree` would
invert the layering. `internal/slug` imports only `regexp` and `strings`, so
nothing can cycle.

`db.Session` gains a derived field:

```go
// Slug is derived from Name, not a column. Populated by scanSession so
// every read path carries it; never written by CreateSession.
Slug string `json:"slug"`
```

`scanSession` is the single seam, and it holds because every response path
re-reads through it:

| path | |
|---|---|
| `Create` | inserts, then re-fetches via `GetSession` (`lifecycle.go:257`) |
| `Update` | returns `GetSession` (`lifecycle.go:647`) |
| `Get` / `List` | scan directly |

The one `db.Session{}` literal (`lifecycle.go:232`) is insert-only and never
marshalled. `sessionColumns` and `CreateSession` both name their columns
explicitly, so a field with no column is simply never written.

Deriving rather than storing also settles rename for free: `PATCH
/sessions/{id}` changes `Name`, and the next scan recomputes the slug. A stored
column would need recomputation on every rename plus a migration to backfill
existing rows.

Web: `Session` in `types.ts` gains `slug: string`; `ComposeBar` takes
`sessionSlug?: string | null`.

### Actions go ghost

| | now | after |
|---|---|---|
| Attach | filled circle `hsl(0 0% 16%)`, glyph 76% | bare glyph `hsl(0 0% 60%)` |
| Send, idle | hidden until focus or draft | always mounted, glyph `hsl(0 0% 45%)` |
| Send, live | blue filled circle | glyph at `--primary` |

Send's idle 45% is 3.86:1, which clears the 3:1 bar for non-text graphics —
the same value that fails for the placeholder, because the bar is different.

Send becomes permanent, as in Slack. That retires "Send appears" as a focus
cue, which the parent spec leaned on; with the placeholder swap also gone, the
card edge brightening carries focus alone. The `showSend` state and its
`focused || text.length > 0` derivation are deleted.

Both controls go ghost rather than following Slack literally — Slack's `+` is a
filled circle. A filled attach beside a ghost send would invert the "weight
follows importance" hierarchy the parent spec established. Live send goes
`--primary` rather than Slack's near-white to keep Argus's identity.

Key labels stay at `hsl(0 0% 72%)`. These are hit at speed; legibility beats
mood.

### `Terminal`'s `sessionName` prop is a session ID

It has always received `getTabSessionId(tab)`. With a genuinely name-derived
value (`sessionSlug`) arriving alongside it, the two would read as near
duplicates, so the misnomer is corrected here rather than compounded.

Renamed to `sessionId` through the chain: `Terminal/index.tsx`,
`useTerminalConnection.types.ts`, `useTerminalConnection.ts`,
`websocket-connection.ts`, the `Workspace` call site, and the comments at
`useTerminalConnection.ts:105-106` and `websocket-connection.ts:252` that
describe the parameter. No test references it, so the rename is mechanical.

Noted, not touched: `SessionStatusInfo.sessionName` in `types.ts` is never read
anywhere in the codebase. Dead field, unrelated chain.

## Files

| File | Change |
|---|---|
| `internal/slug/slug.go` | new — `Make`, moved from `worktree.slugify` |
| `internal/slug/slug_test.go` | new — the moved `TestSlugify` table |
| `internal/git/worktree/slugify.go` | drop `slugify`; keep `worktreeDirName` |
| `internal/git/worktree/manager.go` | call `slug.Make` |
| `internal/git/worktree/slugify_test.go` | drop `TestSlugify`; keep `TestWorktreeDirName` |
| `internal/node/db/models.go` | derived `Slug` field |
| `internal/node/db/sessions.go` | populate `Slug` in `scanSession` |
| `web/src/types.ts` | `Session.slug` |
| `web/src/components/Terminal/ComposeBar.tsx` | card half, `inset-x-3`, placeholder, ghost actions, `onFocusedChange`, drop `showSend` |
| `web/src/components/Terminal/TerminalToolbar.tsx` | card half, `mx-3 mb-2`, visible `border-t`, `focused` prop |
| `web/src/components/Terminal/index.tsx` | lift `focused`; `sessionName` → `sessionId`; pass `sessionSlug` |
| `web/src/components/Terminal/hooks/*` | `sessionName` → `sessionId` |
| `web/src/components/Workspace/index.tsx` | `sessionId` prop; resolve `sessionSlug` per tab |
| `web/src/components/Terminal/ComposeBar.test.tsx` | placeholder and send-visibility tests updated |

## Testing

Existing coverage that must keep passing unchanged: growth clamps at three
lines then scrolls, send clears the draft on success and preserves it on
failure, draft text is never dimmed, and the resting-panel-equals-spacer drift
guard.

Changed: send visibility no longer swaps on focus/blur, and the placeholder no
longer swaps.

New:

- `slug.Make` — the moved table
- `scanSession` populates `Slug` from `Name`
- create and rename round-trips both return a slug matching the name
- placeholder composition: slug pass-through, 28-character truncation, and the
  no-session `Message` fallback

Visual verification repeats the established browser pass: emulated iPhone at
390×844 with `pointer: coarse`, driving the growth range and reading back
panel/mirror/shift geometry plus the resize-frame count. Two things to look at
specifically that the unit tests cannot cover:

- the seam where the two card halves meet, at rest and at full growth — a
  subpixel gap or a doubled hairline would give away the construction
- the first and last key clipping against the card's rounded bottom corners in
  the horizontal scroller
