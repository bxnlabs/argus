# Floating Icons Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Move view mode and attachment icons from `RightSidebar` into the Terminal pane as floating icons in an invisible ghost column (desktop only).

**Architecture:** Add new props to Terminal for view mode state. On desktop, wrap the xterm div and a ghost column in a flex row so FitAddon naturally respects the reduced width. Remove RightSidebar entirely.

**Tech Stack:** React, TypeScript, Tailwind CSS, Lucide icons, Radix Tooltip

---

### Task 1: Add floating icon buttons to Terminal component

**Files:**
- Modify: `web/src/components/Terminal/index.tsx`

**Step 1: Add imports for icons, Button, and Tooltip**

Add these imports at the top of the file, after the existing imports:

```tsx
import {
  Terminal as TerminalIcon,
  GitBranch,
  FilePenLine,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import type { SidePanel } from "@/components/views/types";
```

**Step 2: Add new props to TerminalProps interface**

Add these four props to the `TerminalProps` interface (after `workingDirectory`):

```tsx
  /** Active side panel (null = terminal view) */
  activePanel: SidePanel;
  /** Callback to switch the active panel */
  onSetActivePanel: (panel: SidePanel) => void;
  /** Whether the git panel is available */
  isGitEnabled: boolean;
  /** Whether the editor panel is available */
  isEditorEnabled: boolean;
```

**Step 3: Destructure new props in the component function**

Update the destructuring in the `forwardRef` function to include the four new props:

```tsx
{ sessionName, onConnected, onDisconnected, onBeforeUnmount, initialScrollState, selectMode = false, onFilesDropped, onAttachments, workingDirectory, activePanel, onSetActivePanel, isGitEnabled, isEditorEnabled },
```

**Step 4: Add derived active-state booleans**

Add these after the `isConnected` declaration (around line 86):

```tsx
    const isTerminalActive = activePanel === null;
    const isGitActive = activePanel === "git";
    const isEditorActive = activePanel === "editor";
```

**Step 5: Wrap xterm div and ghost column in a flex row (desktop only)**

Replace the current xterm `<div ref={terminalRef} .../>` block (lines 153-163) with a conditional layout. On desktop (`!isMobile`), wrap in a flex row with the ghost column. On mobile, render the xterm div exactly as before.

Replace:
```tsx
        {/* Terminal container - NO padding! FitAddon reads offsetHeight which includes padding */}
        <div
          ref={terminalRef}
          className={cn(
            "terminal-container min-h-0 w-full flex-1 overflow-hidden",
            selectMode && "ring-primary ring-2 ring-inset"
          )}
          onClick={handleContainerClick}
          onTouchStart={selectMode ? (e) => e.stopPropagation() : undefined}
          onTouchEnd={selectMode ? (e) => e.stopPropagation() : undefined}
        />
```

With:
```tsx
        {/* Terminal + ghost column (desktop) or plain terminal (mobile) */}
        {!isMobile ? (
          <div className="flex min-h-0 flex-1">
            {/* Terminal container - NO padding! FitAddon reads offsetHeight which includes padding */}
            <div
              ref={terminalRef}
              className="terminal-container min-h-0 flex-1 overflow-hidden"
              onClick={handleContainerClick}
            />
            {/* Ghost column — reserves space so xterm reflows around it */}
            <div className="flex w-14 shrink-0 flex-col items-center gap-1 pt-1">
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-lg"
                    onClick={() => onSetActivePanel(null)}
                    className={cn("h-12 w-12", isTerminalActive && "bg-accent")}
                    aria-label="Terminal"
                  >
                    <TerminalIcon className={cn("h-6 w-6", isTerminalActive && "text-primary")} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="left">Terminal</TooltipContent>
              </Tooltip>

              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-lg"
                    disabled={!isGitEnabled}
                    onClick={() => onSetActivePanel("git")}
                    className={cn("h-12 w-12", isGitActive && "bg-accent")}
                    aria-label="Git"
                  >
                    <GitBranch className={cn("h-6 w-6", isGitActive && "text-primary")} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="left">Git</TooltipContent>
              </Tooltip>

              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="icon-lg"
                    disabled={!isEditorEnabled}
                    onClick={() => onSetActivePanel("editor")}
                    className={cn("h-12 w-12", isEditorActive && "bg-accent")}
                    aria-label="Editor"
                  >
                    <FilePenLine className={cn("h-6 w-6", isEditorActive && "text-primary")} />
                  </Button>
                </TooltipTrigger>
                <TooltipContent side="left">Editor</TooltipContent>
              </Tooltip>

              {onAttachments && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-lg"
                      onClick={onAttachments}
                      className="h-12 w-12"
                      aria-label="Attachments"
                    >
                      <Paperclip className="h-6 w-6" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent side="left">Attachments</TooltipContent>
                </Tooltip>
              )}
            </div>
          </div>
        ) : (
          /* Mobile: plain terminal, no ghost column */
          <div
            ref={terminalRef}
            className={cn(
              "terminal-container min-h-0 w-full flex-1 overflow-hidden",
              selectMode && "ring-primary ring-2 ring-inset"
            )}
            onClick={handleContainerClick}
            onTouchStart={selectMode ? (e) => e.stopPropagation() : undefined}
            onTouchEnd={selectMode ? (e) => e.stopPropagation() : undefined}
          />
        )}
```

**Step 6: Verify the build compiles**

Run: `cd web && npx tsc --noEmit 2>&1 | head -30`
Expected: Type errors in `Workspace/index.tsx` because Terminal now requires the new props (this is expected — we fix it in Task 2).

**Step 7: Commit**

```bash
git add web/src/components/Terminal/index.tsx
git commit -m "feat: add floating icon ghost column to Terminal component [BXN-37]"
```

---

### Task 2: Wire up new Terminal props in Workspace and remove RightSidebar

**Files:**
- Modify: `web/src/components/Workspace/index.tsx`
- Delete: `web/src/components/Workspace/RightSidebar.tsx`

**Step 1: Remove RightSidebar import from Workspace**

In `web/src/components/Workspace/index.tsx`, delete line 10:

```tsx
import { RightSidebar } from "./RightSidebar";
```

**Step 2: Pass new props to every Terminal instance**

In the `tabs.map(...)` block (around line 232), add the four new props to the `<Terminal>` JSX:

```tsx
                  <Terminal
                    ref={(handle) => {
                      if (handle) {
                        terminalRefs.current.set(tab.id, handle);
                      } else {
                        terminalRefs.current.delete(tab.id);
                      }
                    }}
                    sessionName={getTabSessionId(tab)}
                    selectMode={isActive ? selectMode : false}
                    onFilesDropped={handleFilesDropped}
                    onAttachments={() => setShowFilePicker(true)}
                    workingDirectory={activeWorkingDirectory}
                    activePanel={activePanel}
                    onSetActivePanel={setActivePanel}
                    isGitEnabled={isGitRepo}
                    isEditorEnabled={!!activeWorkingDirectory}
                  />
```

**Step 3: Remove the RightSidebar JSX block**

Delete the entire `{!isMobile && (<RightSidebar ... />)}` block (lines 253-262).

**Step 4: Delete RightSidebar.tsx**

```bash
rm web/src/components/Workspace/RightSidebar.tsx
```

**Step 5: Verify the build compiles cleanly**

Run: `cd web && npx tsc --noEmit 2>&1 | head -30`
Expected: No errors.

**Step 6: Verify dev server renders**

Run: `cd web && npm run dev` (if not already running), open in browser.
Expected: Desktop view shows floating icons inside the terminal pane with no border/sidebar. Mobile view unchanged.

**Step 7: Commit**

```bash
git add web/src/components/Workspace/index.tsx
git rm web/src/components/Workspace/RightSidebar.tsx
git commit -m "feat: remove RightSidebar, wire floating icons through Workspace [BXN-37]"
```

---

### Task 3: Verify and polish

**Files:**
- Possibly modify: `web/src/components/Terminal/index.tsx`

**Step 1: Test view mode switching**

In the browser (desktop), click each floating icon:
- Terminal icon: should show terminal content, icon gets `bg-accent` + `text-primary`
- Git icon: should show git panel (if session attached to git repo), icon highlighted
- Editor icon: should show file explorer (if session has working directory), icon highlighted
- Paperclip icon: should open file picker dialog
- Disabled icons (no git repo / no working dir): should appear dimmed and not clickable

**Step 2: Test keyboard shortcuts still work**

- `Cmd+Shift+G`: should toggle git panel, floating icon state updates
- `Cmd+Shift+E`: should toggle editor panel, floating icon state updates

**Step 3: Test xterm reflow**

- Resize the window — terminal text should reflow within the narrower area (not clip under icons)
- The ghost column should remain at `w-14` regardless of window size

**Step 4: Test mobile is unaffected**

- Open in mobile viewport (or resize to < 768px)
- No ghost column should appear
- MobileTabBar dropdown and TerminalToolbar should work as before

**Step 5: Commit any polish fixes**

```bash
git add -u
git commit -m "fix: polish floating icons layout [BXN-37]"
```
