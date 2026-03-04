import { SessionList } from "@/components/SessionList";
import { NewSessionDialog } from "@/components/NewSessionDialog";
import { QuickSwitcher } from "@/components/QuickSwitcher";
import { Button } from "@/components/ui/button";
import { PanelLeftClose, SquarePen, Search } from "lucide-react";
import type { ViewProps } from "./types";

export function MobileView({
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
    <main className="bg-background flex h-app flex-col overflow-hidden">
      {/* Sidebar overlay */}
      {sidebarOpen && (
        <>
          <div
            className="fixed inset-0 z-40 bg-black/50"
            onClick={() => setSidebarOpen(false)}
          />
          <div className="bg-sidebar-background fixed inset-y-0 left-0 z-50 w-72 shadow-2xl">
            <div className="flex h-full flex-col pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
              {/* Header */}
              <div className="flex items-center justify-between px-3 py-3">
                <h2 className="pl-1 text-2xl font-bold tracking-wide">
                  argus
                </h2>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setSidebarOpen(false)}
                >
                  <PanelLeftClose className="h-4 w-4" />
                </Button>
              </div>

              {/* Nav items */}
              <nav className="mt-4 flex flex-col gap-0.5 px-2">
                <button
                  onClick={() => {
                    setSidebarOpen(false);
                    setShowNewSessionDialog(true);
                  }}
                  className="hover:bg-accent/50 flex items-center gap-3 rounded-md px-2 py-2 text-base transition-colors"
                >
                  <SquarePen className="h-5 w-5 flex-shrink-0" />
                  <span>New Session</span>
                </button>

                <button
                  onClick={() => {
                    setSidebarOpen(false);
                    setShowQuickSwitcher(true);
                  }}
                  className="hover:bg-accent/50 flex items-center gap-3 rounded-md px-2 py-2 text-base transition-colors"
                >
                  <Search className="h-5 w-5 flex-shrink-0" />
                  <span>Search</span>
                </button>
              </nav>

              {/* Sessions */}
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
                      setSidebarOpen(false);
                    }}
                    onDeleteSession={onDeleteSession}
                    onRenameSession={onRenameSession}
                    onNewSession={() => {
                      setShowNewSessionDialog(true);
                      setSidebarOpen(false);
                    }}
                  />
                </div>
              </div>
            </div>
          </div>
        </>
      )}

      {/* Terminal fills the screen */}
      <div className="isolate min-h-0 w-full flex-1">
        {renderWorkspace()}
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
    </main>
  );
}
