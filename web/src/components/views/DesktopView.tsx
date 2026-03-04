import { SessionList } from "@/components/SessionList";
import { NewSessionDialog } from "@/components/NewSessionDialog";
import { Button } from "@/components/ui/button";
import { PanelLeftClose, PanelLeft, SquarePen, Search } from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { QuickSwitcher } from "@/components/QuickSwitcher";
import { cn } from "@/lib/utils";
import type { ViewProps } from "./types";

export function DesktopView({
  sessions,
  homeDir,
  sessionStatuses,
  sidebarOpen,
  setSidebarOpen,
  activeTab,
  showNewSessionDialog,
  setShowNewSessionDialog,
  showQuickSwitcher,
  setShowQuickSwitcher,
  attachToSession,
  onCreateSession,
  onDeleteSession,
  onRenameSession,
  renderWorkspace,
}: ViewProps) {
  return (
    <div className="bg-background flex h-app overflow-hidden">
      {/* Sidebar — always visible, toggles between expanded (w-72) and collapsed (w-14) */}
      <div
        className={cn(
          "border-border bg-sidebar-background flex flex-shrink-0 flex-col border-r overflow-hidden transition-all duration-200",
          sidebarOpen ? "w-72" : "w-14"
        )}
      >
        {/* Header row: branding + toggle */}
        <div
          className={cn(
            "flex items-center px-3 py-3",
            sidebarOpen ? "justify-between" : "justify-center"
          )}
        >
          {sidebarOpen && (
            <h2 className="pl-1 text-2xl font-bold tracking-wide">argus</h2>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => setSidebarOpen(!sidebarOpen)}
              >
                {sidebarOpen ? (
                  <PanelLeftClose className="h-4 w-4" />
                ) : (
                  <PanelLeft className="h-4 w-4" />
                )}
              </Button>
            </TooltipTrigger>
            {!sidebarOpen && (
              <TooltipContent side="right">Expand sidebar</TooltipContent>
            )}
          </Tooltip>
        </div>

        {/* Nav items */}
        <nav className="mt-4 flex flex-col gap-0.5 px-2">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={() => setShowNewSessionDialog(true)}
                className={cn(
                  "hover:bg-accent/50 flex items-center gap-3 rounded-md px-2 py-2 text-base transition-colors",
                  !sidebarOpen && "justify-center"
                )}
              >
                <SquarePen className="h-5 w-5 flex-shrink-0" />
                {sidebarOpen && <span>New Session</span>}
              </button>
            </TooltipTrigger>
            {!sidebarOpen && (
              <TooltipContent side="right">New Session</TooltipContent>
            )}
          </Tooltip>

          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={() => setShowQuickSwitcher(true)}
                className={cn(
                  "hover:bg-accent/50 flex items-center gap-3 rounded-md px-2 py-2 text-base transition-colors",
                  !sidebarOpen && "justify-center"
                )}
              >
                <Search className="h-5 w-5 flex-shrink-0" />
                {sidebarOpen && <span>Search</span>}
              </button>
            </TooltipTrigger>
            {!sidebarOpen && (
              <TooltipContent side="right">Search</TooltipContent>
            )}
          </Tooltip>
        </nav>

        {/* Sessions section — expanded only */}
        {sidebarOpen && (
          <div className="mt-10 flex min-h-0 flex-1 flex-col overflow-hidden">
            <div className="text-muted-foreground px-4 pb-1 text-xs font-medium">
              Sessions
            </div>
            <div className="min-h-0 flex-1 overflow-hidden">
              <SessionList
                sessions={sessions}
                homeDir={homeDir}
                activeSessionId={activeTab?.sessionId || undefined}
                sessionStatuses={sessionStatuses}
                onAttachSession={(id) => {
                  const session = sessions.find((s) => s.id === id);
                  if (session) attachToSession(session);
                }}
                onDeleteSession={onDeleteSession}
                onRenameSession={onRenameSession}
                onNewSession={() => setShowNewSessionDialog(true)}
              />
            </div>
          </div>
        )}
      </div>

      {/* Main content */}
      <div className="flex min-w-0 flex-1 flex-col">
        <div className="min-h-0 flex-1">
          {renderWorkspace()}
        </div>
      </div>

      {/* Dialogs */}
      <NewSessionDialog
        open={showNewSessionDialog}
        onClose={() => setShowNewSessionDialog(false)}
        onCreateSession={onCreateSession}
      />
      <QuickSwitcher
        sessions={sessions}
        homeDir={homeDir}
        open={showQuickSwitcher}
        onOpenChange={setShowQuickSwitcher}
        currentSessionId={activeTab?.sessionId ?? undefined}
        onSelectSession={(sessionId) => {
          const session = sessions.find((s) => s.id === sessionId);
          if (session) attachToSession(session);
        }}
      />
    </div>
  );
}
