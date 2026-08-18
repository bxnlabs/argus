import { Button } from "@/components/ui/button";
import { X, Plus } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { isMac } from "@/lib/device";
import type { Session } from "@/types";
import type { TabData } from "@/lib/tabs";

interface DesktopTabBarProps {
  tabs: TabData[];
  activeTabId: string;
  sessions: Session[];
  onTabSwitch: (tabId: string) => void;
  onTabClose: (tabId: string) => void;
  onTabAdd: () => void;
}

export function DesktopTabBar({
  tabs,
  activeTabId,
  sessions,
  onTabSwitch,
  onTabClose,
  onTabAdd,
}: DesktopTabBarProps) {
  const leader = isMac() ? "⌘ ;" : "Ctrl ;";

  const getTabName = (tab: TabData) => {
    if (tab.sessionId) {
      const s = sessions.find((sess) => sess.id === tab.sessionId);
      return s?.name || "Session";
    }
    return "New Tab";
  };

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
          <TooltipContent>New tab ({leader} =)</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
