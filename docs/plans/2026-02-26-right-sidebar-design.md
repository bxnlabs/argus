# Desktop Right Sidebar — Design

## Overview

Move view mode icons (Terminal, Git, Editor) and the attachment icon (Paperclip) from their current locations into a static, icon-only right sidebar on the desktop layout.

## Layout

```
┌──────────┬──────────────────┬────┐
│          │   Tab Bar        │ >_ │ Terminal
│  Left    ├──────────────────┤ /  │ Git
│  Side    │                  │ ✎  │ Editor
│  bar     │   Terminal /     │────│
│          │   Content        │ 📎 │ Attachments
│          │                  │    │
│          │                  │    │
└──────────┴──────────────────┴────┘
```

- Full-height column alongside the entire workspace (tab bar + content)
- ~w-10 width, icons only, no collapse behavior
- Left border to separate from content

## Icons (top to bottom)

1. Terminal (TerminalIcon) — sets activePanel to null
2. Git (GitBranch) — sets activePanel to "git"
3. Editor (FilePenLine) — sets activePanel to "editor"
4. Horizontal divider
5. Attachments (Paperclip) — opens file picker

Active view mode highlighted with `text-primary`.

## Changes

1. **New component: `RightSidebar`** — icon-only column with view modes + attachments
2. **`DesktopView.tsx`** — add right sidebar as third column in flex row
3. **`DesktopTabBar.tsx`** — remove "Divider + View Modes" section (keep Detach button)
4. **`Terminal/index.tsx`** — remove floating Paperclip button (desktop only)
5. **Props threading** — wire activePanel, onSetActivePanel, isGitEnabled, isEditorEnabled, onAttachments through DesktopView

## Scope

- Desktop only — no changes to MobileView, MobileTabBar, or TerminalToolbar
