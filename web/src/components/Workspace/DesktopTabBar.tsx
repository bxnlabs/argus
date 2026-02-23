import { Button } from "@/components/ui/button";
import {
  X,
  Unplug,
  Plus,
  Terminal as TerminalIcon,
  GitBranch,
  FilePenLine,
} from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { Session } from "@/types";
import type { TabData } from "@/lib/tabs";
import type { SidePanel } from "@/components/views/types";

interface DesktopTabBarProps {
  tabs: TabData[];
  activeTabId: string;
  sessions: Session[];
  hasAttachedSession: boolean;
  activePanel: SidePanel;
  onSetActivePanel: (panel: SidePanel) => void;
  isGitEnabled: boolean;
  isEditorEnabled: boolean;
  onTabSwitch: (tabId: string) => void;
  onTabClose: (tabId: string) => void;
  onTabAdd: () => void;
  onDetach: () => void;
}

export function DesktopTabBar({
  tabs,
  activeTabId,
  sessions,
  hasAttachedSession,
  activePanel,
  onSetActivePanel,
  isGitEnabled,
  isEditorEnabled,
  onTabSwitch,
  onTabClose,
  onTabAdd,
  onDetach,
}: DesktopTabBarProps) {
  const getTabName = (tab: TabData) => {
    if (tab.sessionId) {
      const s = sessions.find((sess) => sess.id === tab.sessionId);
      return s?.name || "Session";
    }
    return "New Tab";
  };

  const isTerminalActive = activePanel === null;
  const isGitActive = activePanel === "git";
  const isEditorActive = activePanel === "editor";

  return (
    <div
      className={cn(
        "bg-muted flex h-10 items-center gap-1 overflow-x-auto px-1"
      )}
    >
      {/* Tabs */}
      <div className="flex min-w-0 flex-1 items-end gap-0.5 self-stretch">
        {tabs.map((tab) => (
          <div
            key={tab.id}
            onClick={(e) => {
              e.stopPropagation();
              onTabSwitch(tab.id);
            }}
            className={cn(
              "group mt-auto flex cursor-pointer items-center gap-1.5 rounded-t-md px-3 py-2.5 text-xs transition-colors",
              tab.id === activeTabId
                ? "bg-background text-foreground"
                : "text-muted-foreground hover:text-foreground/80 hover:bg-accent/50"
            )}
          >
            <span className="max-w-[120px] truncate">{getTabName(tab)}</span>
            {tabs.length > 1 && (
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  onTabClose(tab.id);
                }}
                className="hover:text-foreground ml-1 opacity-0 group-hover:opacity-100"
              >
                <X className="h-3 w-3" />
              </button>
            )}
          </div>
        ))}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={(e) => {
                e.stopPropagation();
                onTabAdd();
              }}
              className="mx-1 h-6 w-6 self-end mb-1"
            >
              <Plus className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>New tab</TooltipContent>
        </Tooltip>
      </div>

      {/* Actions */}
      <div className="ml-auto flex items-center gap-0.5 px-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={(e) => {
                e.stopPropagation();
                onDetach();
              }}
              disabled={!hasAttachedSession}
              className="h-6 w-6"
            >
              <Unplug className="h-3 w-3" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Detach session</TooltipContent>
        </Tooltip>
      </div>

      {/* Divider + View Modes */}
      <div className="border-border flex items-center gap-0.5 border-l pl-1 pr-1">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={(e) => {
                e.stopPropagation();
                onSetActivePanel(null);
              }}
              className="h-6 w-6"
            >
              <TerminalIcon className={cn("h-3 w-3", isTerminalActive && "text-primary")} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Terminal</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={(e) => {
                e.stopPropagation();
                onSetActivePanel("git");
              }}
              disabled={!isGitEnabled}
              className="h-6 w-6"
            >
              <GitBranch className={cn("h-3 w-3", isGitActive && "text-primary")} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Git</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={(e) => {
                e.stopPropagation();
                onSetActivePanel("editor");
              }}
              disabled={!isEditorEnabled}
              className="h-6 w-6"
            >
              <FilePenLine className={cn("h-3 w-3", isEditorActive && "text-primary")} />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Editor</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
