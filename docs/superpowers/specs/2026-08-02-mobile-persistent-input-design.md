# Mobile persistent compose input

Date: 2026-08-02
Branch: `jeev/mobile-persistent-input`

## Problem

On mobile, composing text for an agent is a modal detour. The user taps a pen
button in the terminal toolbar, a full-screen overlay covers the terminal
(`ComposeInput`, `web/src/components/Terminal/TerminalToolbar.tsx:72-190`), they
type, and the overlay closes on send. While composing they cannot see the
terminal at all — which is precisely when they most want to, since the message
usually refers to what the agent just printed.

The toolbar also spends two of its eleven slots on buttons that exist only to
open modals (compose, attach), crowding the special keys on a narrow screen.

## Goals

- The compose input is always present in mobile terminal view — no toggle.
- The terminal stays visible and scrollable while typing.
- Vertical space spent on chrome is minimized; the terminal gets the rest.
- Input starts at one line, grows to three, scrolls beyond that.

## Non-goals

- Desktop is unchanged. The toolbar and compose modal already render only on
  mobile (`Terminal/index.tsx:184`), so this is a mobile-only redesign.
- No draft persistence across page reloads.
- No change to the wire protocol or to `injectCompose`'s paste semantics.

## Decisions

### Focus model: two live targets

Tapping the terminal focuses xterm and raises the keyboard, exactly as today.
Tapping the input focuses the input. Focus follows the last tap.

The alternative — making the compose bar the only text-entry path on mobile —
was rejected because it removes live per-keystroke typing into the pty, which is
needed for interactive prompts and TUIs that read raw keys.

### Layout: both bars permanent

```
┌──────────────────────┐
│  terminal (flex-1)   │  never resized while composing
├──────────────────────┤
│ 📎 │ Message…    │ ▶ │  ComposeBar — one line in flow
├──────────────────────┤
│ esc ctrl ←→↑↓ tab ⏎  │  TerminalToolbar (9 keys, was 11)
└──────────────────────┘
```

Both bars stay mounted whether or not the keyboard is up. This costs ~80px
permanently but has zero layout shift, and keeps `esc` / `^C` one tap away while
watching output without raising the keyboard.

Hiding the toolbar when the keyboard is down was considered. It reclaims ~41px
while reading, but every keyboard toggle then adds a mount/unmount and an xterm
refit on top of the refit the keyboard already causes.

### Growth: overlay, not resize

The ComposeBar occupies a **fixed one-line height in flow**. Its panel is
absolutely positioned, anchored to the bottom of that row, and grows *upward* to
three lines, scrolling internally beyond that.

Growth therefore never changes the terminal container's height. `FitAddon` never
refits, no `SIGWINCH` reaches the pty, and xterm never reflows wrapped
scrollback. This matters because reflow moves the scroll position: resizing on
line two would yank the terminal out from under a user who had scrolled up to
re-read an error while composing.

**Scroll compensation.** The terminal container gets
`transform: translateY(-H)`, where `H` = current panel height − one line.
Transform does not affect layout, so siblings do not move and xterm does not
refit. The outer container is already `overflow-hidden`
(`Terminal/index.tsx:125`), so the top `H` px clips while the bottom rows rise
above the panel. The newest output stays visible; the oldest visible rows are
sacrificed, and remain reachable by scrolling. The transform animates on the
same duration as the panel growth so it reads as one motion.

`padding-bottom` on the scroll container was considered and rejected: xterm's
`Viewport` maps `scrollTop` to a row offset by dividing by cell height, so
padding desyncs the render offset.

Reserving three lines of height up front (one resize at keyboard-open, none
while typing) is the fallback if the transform proves fiddly against xterm's
internals. It permanently spends ~2 of ~17 visible rows on empty space.

### Send semantics

- `Return` inserts a newline. `▶` sends.
- Send calls the existing `sendText` (bracketed paste + delayed `Return`,
  `submit: true`), clears the input, and keeps focus and the keyboard up.
- `▶` is disabled when the input is empty or the socket is not connected. The
  draft is preserved on disconnect — silently dropping text into a closed socket
  is the failure mode to avoid.

### Attachments

The 📎 button moves into the input bar. It opens the existing `FilePicker`
scoped to `workingDirectory` and inserts shell-escaped paths at the cursor,
porting `ComposeInput.handleFilesPicked` verbatim including its space-separator
rules.

The toolbar's attach button — which typed paths straight into the pty via
`Workspace`'s picker — is removed. That flow is now "insert into the draft, edit,
send", which is strictly more capable.

### Focus affordance

Only the input bar changes appearance. The terminal gets no added chrome; xterm
already signals focus by drawing its cursor solid vs hollow.

| | terminal focused | input focused |
|---|---|---|
| border | `border-white/10` | `border-white/25` |
| background | `bg-transparent` | `bg-secondary/50` |
| placeholder | `"Tap to compose"`, muted | `"Message…"`, full opacity |
| send button | hidden | visible (disabled until there's text) |

The placeholder swap and the send button appearing carry the signal — a
`white/10 → white/25` border delta is not legible on a phone. Border and
background are polish.

Dimming applies to chrome only, never to draft text. A user who types two lines
and then taps the terminal to hit `esc` must still see their draft clearly.

### Draft scope

Draft lives in `ComposeBar` component state. `ComposeBar` lives inside
`Terminal`, which stays mounted per tab, so drafts are per-tab and survive tab
and panel switches. They are lost on reload. No persistence layer.

### Select mode

Both bars hide, via the existing `visible={!selectMode}` pattern.

## Sending while scrolled

Two routes, each with exactly one applicable fix. Neither route is rare.

| Route | Buffer | Scroll mechanism | Fix |
|---|---|---|---|
| `/ws/sessions/{id}` (attached) | alternate | wheel events → tmux copy-mode | server-side `send-keys -X cancel` |
| `/ws/terminal` (unattached tab) | normal | local `term.scrollLines()` | client-side `scrollToBottom()` |

### Attached sessions: exit tmux copy-mode

The web terminal is a real `tmux attach-session`
(`internal/node/terminal/handler.go:368`) and the seeded config sets `mouse on`
(`internal/shared/tmux.go:19`). tmux attaches on the alternate screen, so
`buffer.type === "alternate"` holds for every attached session, and mobile
touch-scroll always takes the wheel-event branch
(`web/src/components/Terminal/hooks/touch-scroll.ts:97-104`). With mouse on, a
wheel-up puts the pane into copy-mode.

`injectCompose` writes to `s.ptmx`, which is the pty of the tmux *client* — so
those bytes are client keystrokes. A pane in copy-mode routes them through the
copy-mode key table instead of to the pane's process. The message never reaches
the agent, and its characters fire copy-mode bindings on the way past (`q`
cancels, `/` opens search, `g`/`G` jump, digits become repeat counts, space
starts a selection); the trailing `\r` from `submit` copies the selection and
exits. Silent loss, plus a clobbered paste buffer.

Fix, in the `"text"` branch of `handler.go:141`, before `injectCompose`:

```go
// The PTY we write to is a tmux *client*. A pane in copy-mode routes those
// bytes through the copy-mode key table, so the paste never reaches the
// agent and its characters fire copy-mode bindings instead. Cancel first.
// No-op (and harmless error) when the pane isn't in a mode.
_ = exitPaneMode(s.sessionName)
```

`handleConnection` already receives `sessionName` (`handler.go:281`); it is
stored on the `session` struct. The call is guarded on a non-empty name so the
raw-shell route skips it. One `tmux send-keys -t <name> -X cancel` exec per send.

Side effect, and a desirable one: cancelling snaps the pane back to live output,
so sending visibly returns the user to the bottom — feedback that the message
landed, instead of the pane sitting in history while the agent replies
off-screen.

This is a pre-existing bug that also affects today's compose modal. It gets its
own commit on this branch, since the persistent input is what makes it routine.

### Unattached tabs: scroll to bottom

`getTabSessionId` returns `tab.sessionId || null`
(`web/src/components/Workspace/index.tsx:136`), and `nodeWsUrl` falls back to
`/ws/terminal` on an empty id (`web/src/api/client.ts:22`). So a tab with no
session attached gets a plain `$SHELL` with no tmux in the path — normal buffer,
real local xterm scrollback, and `touch-scroll.ts:101` takes the
`term.scrollLines()` branch.

Sending from a scrolled-up unattached tab delivers the text fine, but xterm
deliberately does not jump to the bottom when new output arrives while the user
is scrolled up. The reply lands off-screen and the terminal looks frozen.

Fix: call `term.scrollToBottom()` on send. On this route `exitPaneMode` is
skipped (empty session name), so this is not belt-and-braces — it is the entire
fix.

The other normal-buffer window, between attach and tmux emitting smcup, is real
but sub-second with no scrollback accumulated. Not designed for.

### Scope line

Both fixes apply to compose sends only, not to the toolbar's raw keys. Arrows,
`esc` and `^C` are legitimate copy-mode navigation and stay passthrough. A
compose send is the only unambiguous "talk to the agent" intent.

## Components and files

| File | Change |
|---|---|
| `web/src/components/Terminal/ComposeBar.tsx` | new — textarea, grow/clamp, 📎 + ▶, focus states, reports `H` upward |
| `web/src/components/Terminal/index.tsx` | render `ComposeBar`; apply `translateY(-H)`; `scrollToBottom()` on send; drop `onAttachments` |
| `web/src/components/Terminal/TerminalToolbar.tsx` | delete `ComposeInput`, the compose and attach buttons, and the `onSendText` / `onAttachments` / `workingDirectory` props (~130 lines lighter) |
| `web/src/components/Workspace/index.tsx` | stop passing `onAttachments` to `Terminal` (the desktop `ViewModeRail` keeps it) |
| `internal/node/terminal/handler.go` | store `sessionName` on `session`; add `exitPaneMode`; call it before `injectCompose` |

`ComposeBar` owns: draft state, growth measurement, the file picker, and the
focus-state styling. It reports only its overlay height `H` upward. `Terminal`
owns: translating the terminal container by `H`, and wiring send to `sendText` +
`scrollToBottom`.

The toolbar keeps its current ~41px height and padding. It is already under
Apple's 44px touch-target guidance, and it is the row `^C` lives on. Dropping two
buttons is what makes the remaining nine (~44px each ≈ 396px) nearly fit a 390px
screen without horizontal scrolling.

## Testing

`ComposeBar` (vitest):

- growth clamps at three lines, then scrolls internally
- send calls `onSend` with the raw text and clears the input
- `▶` disabled when empty; disabled when disconnected, with the draft preserved
- placeholder and send-button visibility swap on focus and blur
- draft text is not dimmed when the input loses focus
- path insertion at the cursor, including the leading/trailing space rules

`Terminal` (vitest):

- the terminal container's `transform` tracks the reported overlay height —
  the observable proxy for "the pty was never resized"
- send calls `scrollToBottom`

`handler.go` (go test):

- `exitPaneMode` is called before the paste write on a `text` message
- it is skipped when `sessionName` is empty

Follow the `sleep func(time.Duration)` injection precedent already in
`injectCompose` — `exitPaneMode` is injectable for the same reason.

tmux integration tests must stay isolated from the default socket; production
argus runs on this box with live sessions.
