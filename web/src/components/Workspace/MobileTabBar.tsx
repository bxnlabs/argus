import { useState, useRef, useCallback, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  PanelLeft,
  ChevronLeft,
  ChevronRight,
  Plus,
  SquarePen,
  X,
  Terminal as TerminalIcon,
  GitBranch,
  FilePenLine,
  MousePointer2,
  Unplug,
} from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import type { Session } from "@/types";
import type { TabData } from "@/lib/tabs";
import type { SidePanel } from "@/components/views/types";

interface MobileTabBarProps {
  tabs: TabData[];
  activeTabId: string;
  session: Session | null | undefined;
  sessions: Session[];
  activePanel: SidePanel;
  onSetActivePanel: (panel: SidePanel) => void;
  isGitEnabled: boolean;
  isEditorEnabled: boolean;
  selectMode?: boolean;
  onEnterSelectMode?: () => void;
  onExitSelectMode?: () => void;
  onMenuClick?: () => void;
  onTabSwitch: (tabId: string) => void;
  onTabClose?: (tabId: string) => void;
  onTabAdd?: () => void;
  onNewSession?: () => void;
  hasAttachedSession?: boolean;
  onDetach?: () => void;
}

export function MobileTabBar({
  tabs,
  activeTabId,
  session,
  sessions,
  activePanel,
  onSetActivePanel,
  isGitEnabled,
  isEditorEnabled,
  selectMode = false,
  onEnterSelectMode,
  onExitSelectMode,
  onMenuClick,
  onTabSwitch,
  onTabClose,
  onTabAdd,
  onNewSession,
  hasAttachedSession = false,
  onDetach,
}: MobileTabBarProps) {
  const currentIndex = tabs.findIndex((t) => t.id === activeTabId);

  const hasPrev = currentIndex > 0;
  const hasNext = currentIndex >= 0 && currentIndex < tabs.length - 1;

  // Debounce to prevent rapid clicking causing command interference
  const [isNavigating, setIsNavigating] = useState(false);
  const [isSheetOpen, setIsSheetOpen] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  const handleNavigate = useCallback(
    (tabId: string) => {
      if (isNavigating) return;

      setIsNavigating(true);
      onTabSwitch(tabId);

      // Allow next navigation after delay
      if (debounceRef.current) clearTimeout(debounceRef.current);
      debounceRef.current = setTimeout(() => {
        setIsNavigating(false);
      }, 500);
    },
    [isNavigating, onTabSwitch]
  );

  const handlePrev = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (hasPrev && !isNavigating) {
      handleNavigate(tabs[currentIndex - 1].id);
    }
  };

  const handleNext = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (hasNext && !isNavigating) {
      handleNavigate(tabs[currentIndex + 1].id);
    }
  };

  const getTabName = (tab: TabData) => {
    if (tab.sessionId) {
      const s = sessions.find((sess) => sess.id === tab.sessionId);
      return s?.name || "Session";
    }
    return "New Tab";
  };

  // Display name for the trigger button
  const displayName =
    session?.name ||
    (activeTabId && currentIndex >= 0
      ? getTabName(tabs[currentIndex])
      : "No session");

  const isTerminalActive = activePanel === null;
  const isGitActive = activePanel === "git";
  const isEditorActive = activePanel === "editor";
  const ActiveIcon = selectMode
    ? MousePointer2
    : isGitActive
      ? GitBranch
      : isEditorActive
        ? FilePenLine
        : TerminalIcon;

  return (
    <div
      className="bg-muted pt-[env(safe-area-inset-top)]"
      onClick={(e) => e.stopPropagation()}
      onTouchStart={(e) => e.stopPropagation()}
      onTouchEnd={(e) => e.stopPropagation()}
      onContextMenu={(e) => e.preventDefault()}
    >
      <div className="flex items-center gap-2 px-2 py-1.5">
        {/* Menu button */}
        {onMenuClick && (
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={(e) => {
              e.stopPropagation();
              onMenuClick();
            }}
            className="h-8 w-8 shrink-0"
          >
            <PanelLeft className="h-4 w-4" />
          </Button>
        )}

        {/* Tab navigation */}
        <div className="flex min-w-0 flex-1 items-center gap-1">
          <button
            type="button"
            onClick={handlePrev}
            disabled={!hasPrev || isNavigating}
            className="hover:bg-accent flex h-8 w-8 shrink-0 items-center justify-center rounded-md disabled:pointer-events-none disabled:opacity-50"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>

          {/* Tab selector sheet */}
          <Sheet open={isSheetOpen} onOpenChange={setIsSheetOpen}>
            <SheetTrigger asChild>
              <button
                type="button"
                className="hover:bg-accent active:bg-accent flex min-w-0 flex-1 items-center justify-center gap-1 rounded-md px-2 py-1"
              >
                <span className="truncate text-sm font-medium">
                  {displayName}
                </span>
              </button>
            </SheetTrigger>
            <SheetContent
              side="bottom"
              hideCloseButton
              dismissOnOverlayClick
              className="rounded-t-xl"
            >
              <SheetTitle className="sr-only">Tabs</SheetTitle>
              <SheetDescription className="sr-only">
                Switch between open tabs
              </SheetDescription>
              <div className="flex flex-col px-2 pt-1 pb-2">
                {/* Tab list */}
                <div className="max-h-[50vh] overflow-y-auto">
                  {tabs.map((tab) => {
                    const isActive = tab.id === activeTabId;

                    return (
                      <div
                        key={tab.id}
                        className={cn(
                          "flex items-center rounded-lg px-3 py-3",
                          isActive && "bg-accent"
                        )}
                      >
                        <button
                          onClick={() => {
                            onTabSwitch(tab.id);
                            setIsSheetOpen(false);
                          }}
                          className="min-w-0 flex-1 text-left"
                        >
                          <span className="block truncate text-base">
                            {getTabName(tab)}
                          </span>
                        </button>
                        {tabs.length > 1 && (
                          <button
                            aria-label={`Close ${getTabName(tab)}`}
                            onClick={() => onTabClose?.(tab.id)}
                            className="text-muted-foreground hover:text-foreground -mr-1 shrink-0 rounded-md p-1.5"
                          >
                            <X className="h-4 w-4" />
                          </button>
                        )}
                      </div>
                    );
                  })}
                </div>

                {/* Separator */}
                <div className="bg-border mx-1 my-2 h-px" />

                {/* Actions */}
                <button
                  onClick={() => {
                    onTabAdd?.();
                    setIsSheetOpen(false);
                  }}
                  className="hover:bg-accent/50 flex items-center gap-3 rounded-lg px-3 py-3 text-base transition-colors"
                >
                  <Plus className="h-5 w-5" />
                  <span>New Tab</span>
                </button>
                <button
                  onClick={() => {
                    onNewSession?.();
                    setIsSheetOpen(false);
                  }}
                  className="hover:bg-accent/50 flex items-center gap-3 rounded-lg px-3 py-3 text-base transition-colors"
                >
                  <SquarePen className="h-5 w-5" />
                  <span>New Session</span>
                </button>
              </div>
              {/* Safe area inset for iOS */}
              <div className="h-[env(safe-area-inset-bottom)]" />
            </SheetContent>
          </Sheet>

          <button
            type="button"
            onClick={handleNext}
            disabled={!hasNext || isNavigating}
            className="hover:bg-accent flex h-8 w-8 shrink-0 items-center justify-center rounded-md disabled:pointer-events-none disabled:opacity-50"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>

        {/* Detach + Upload + View mode switcher */}
        <div className="flex items-center gap-0.5">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={(e) => {
              e.stopPropagation();
              onDetach?.();
            }}
            disabled={!hasAttachedSession}
            className="h-8 w-8 shrink-0"
            aria-label="Detach session"
          >
            <Unplug className="h-4 w-4" />
          </Button>
          <div className="border-border h-4 border-l" />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                className="h-8 w-8 shrink-0"
                aria-label="Switch view mode"
              >
                <ActiveIcon className="h-4 w-4 text-primary" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                onClick={() => {
                  onSetActivePanel(null);
                  onExitSelectMode?.();
                }}
                className={cn(isTerminalActive && !selectMode && "text-primary")}
              >
                <TerminalIcon className="h-4 w-4" />
                <span>Terminal</span>
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => onSetActivePanel("git")}
                disabled={!isGitEnabled}
                className={cn(isGitActive && "text-primary")}
              >
                <GitBranch className="h-4 w-4" />
                <span>Git</span>
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => onSetActivePanel("editor")}
                disabled={!isEditorEnabled}
                className={cn(isEditorActive && "text-primary")}
              >
                <FilePenLine className="h-4 w-4" />
                <span>Editor</span>
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => onEnterSelectMode?.()}
                className={cn(selectMode && "text-primary")}
              >
                <MousePointer2 className="h-4 w-4" />
                <span>Select</span>
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
    </div>
  );
}
