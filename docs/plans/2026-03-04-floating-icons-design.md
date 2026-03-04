# Floating Icons Design

## Goal

Move the view mode icons (Terminal, Git, Editor) and attachments icon out of the
`RightSidebar` component and into the Terminal pane itself as floating icons in
an invisible "ghost" column. Desktop only — mobile layout is unchanged.

## Current State

- `RightSidebar` renders as a `border-l` column to the right of the content area
  in `Workspace/index.tsx`, containing view mode buttons, a divider, and an
  attachments button.
- Terminal component (`Terminal/index.tsx`) has no awareness of view mode state.
- Mobile uses `MobileTabBar` dropdown + `TerminalToolbar` for these controls.

## Design

### Layout

Inside `Terminal/index.tsx`, on desktop (`!isMobile`), wrap the xterm container
and a new ghost column in a flex row:

```
Terminal container (flex-col, 100% w/h)
├── SearchBar
├── Flex-row wrapper (flex-1, min-h-0)
│   ├── xterm div (flex-1, min-h-0)       ← FitAddon measures this
│   └── Ghost column (w-14, no bg/border, flex-col, items-center, pt-1, gap-1)
│       ├── Terminal icon button (h-12 w-12, icon h-6 w-6)
│       ├── Git icon button (h-12 w-12, icon h-6 w-6)
│       ├── Editor icon button (h-12 w-12, icon h-6 w-6)
│       └── Paperclip icon button (h-12 w-12, icon h-6 w-6)
├── TerminalToolbar (mobile only)
└── Connection overlays
```

On mobile, no flex-row wrapper — xterm div renders directly as before.

### Ghost Column

- Width: `w-14` (56px) — fits h-12 buttons with breathing room.
- No background, no border — completely invisible container.
- Icons are top-aligned (`pt-1`), no vertical centering.
- No divider between view mode icons and attachments — all four icons flow with
  `gap-1`.

### Icon Styling

- Buttons: `h-12 w-12` (up from `h-10 w-10`).
- Icons: `h-6 w-6` (up from `h-5 w-5`).
- Active state: subtle background fill (`bg-accent`) + `text-primary` icon color.
  Replaces the blue pill indicator.
- Disabled states unchanged: grayed out with reduced opacity.
- Tooltips remain, positioned `side="left"`.

### Props

New props on `Terminal` (always passed, not optional):

- `activePanel: SidePanel`
- `onSetActivePanel: (panel: SidePanel) => void`
- `isGitEnabled: boolean`
- `isEditorEnabled: boolean`

Terminal uses its existing `useViewport()` / `isMobile` check to decide whether
to render the ghost column. Mobile path ignores these props.

### Overlays

All existing overlays (drag-drop, connection states, select mode) use
`absolute inset-0 z-*` and naturally cover the ghost column — no changes needed.

### xterm FitAddon

FitAddon measures from the xterm div's offsetWidth/offsetHeight. Since the xterm
div is `flex-1` inside the new flex row, and the ghost column is fixed `w-14`,
FitAddon automatically gets the reduced width. No manual resize math.

## Files Changed

1. **`web/src/components/Terminal/index.tsx`** — Add props, wrap xterm + ghost
   column in flex row on desktop, render floating icon buttons.
2. **`web/src/components/Workspace/index.tsx`** — Remove `RightSidebar` render,
   pass view mode props to `Terminal`.
3. **`web/src/components/Workspace/RightSidebar.tsx`** — Delete (no longer used).

## What Stays the Same

- Mobile layout completely untouched.
- `MobileTabBar` keeps its view mode dropdown.
- `TerminalToolbar` keeps its attachments button.
- Keyboard shortcuts (Cmd+Shift+G, Cmd+Shift+E) unaffected.
