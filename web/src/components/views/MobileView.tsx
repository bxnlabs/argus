import { useCallback, useEffect } from "react";
import { SessionList } from "@/components/SessionList";
import { NewSessionDialog } from "@/components/NewSessionDialog";
import { QuickSwitcher } from "@/components/QuickSwitcher";
import { Button } from "@/components/ui/button";
import { PanelLeftClose, SquarePen, Search } from "lucide-react";
import { MobileNodePanel } from "@/components/NodeRail/MobileNodePanel";
import { NodeStatus } from "@/components/NodeStatus";
import { SessionSummary } from "@/components/SessionSummary";
import type { ViewProps } from "./types";

export function MobileView({
  sessions,
  homeDir,
  sessionStatuses,
  sessionsLoaded,
  sessionsError,
  sessionsErrorMessage,
  onRetrySessions,
  sidebarOpen,
  setSidebarOpen,
  railOpen,
  setRailOpen,
  activeTab,
  showNewSessionDialog,
  setShowNewSessionDialog,
  onCloseNewSessionDialog,
  showQuickSwitcher,
  setShowQuickSwitcher,
  attachToSession,
  onCreateSession,
  onDeleteSession,
  onRenameSession,
  onCloneSession,
  onChangeProfile,
  onViewInfo,
  onTogglePin,
  onMarkRead,
  onMarkUnread,
  renderWorkspace,
}: ViewProps) {
  const handleAttachSession = useCallback(
    (id: string) => {
      const session = sessions.find((s) => s.id === id);
      if (session) attachToSession(session);
      setSidebarOpen(false);
    },
    [sessions, attachToSession, setSidebarOpen],
  );

  // The node panel overlays the drawer, so it should never outlive it: reset
  // railOpen when the drawer closes, otherwise reopening the drawer would pop
  // the panel straight back over it (and a desktop→mobile switch could surface
  // it over a closed drawer).
  useEffect(() => {
    if (!sidebarOpen && railOpen) setRailOpen(false);
  }, [sidebarOpen, railOpen, setRailOpen]);

  return (
    <main className="bg-background flex h-app flex-col overflow-hidden">
      {/* Sidebar overlay */}
      {sidebarOpen && (
        <>
          <div
            className="fixed inset-0 z-40 bg-black/60 backdrop-blur-sm"
            onClick={() => setSidebarOpen(false)}
          />
          <div className="bg-sidebar-background fixed inset-y-0 left-0 z-50 w-72 shadow-2xl">
            <div className="flex h-full flex-row pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
              <div className="flex min-w-0 flex-1 flex-col">
                {/* Header: branding + close, then the node status snippet */}
                <div className="px-3 py-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <NodeStatus
                        railOpen={railOpen}
                        onToggleRail={() => setRailOpen(!railOpen)}
                      />
                      <h2 className="text-2xl font-bold tracking-wide">argus</h2>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => setSidebarOpen(false)}
                    >
                      <PanelLeftClose className="h-4 w-4" />
                    </Button>
                  </div>
                  {/* Rollup of the sessions listed below, matching the desktop
                      sidebar header. */}
                  <div className="mt-1.5 pl-10">
                    <SessionSummary
                      sessions={sessions}
                      sessionStatuses={sessionStatuses}
                      sessionsLoaded={sessionsLoaded}
                    />
                  </div>
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
                  <div className="min-h-0 flex-1 overflow-hidden">
                    <SessionList
                      sessions={sessions}
                      homeDir={homeDir}
                      activeSessionId={activeTab?.sessionId || undefined}
                      sessionStatuses={sessionStatuses}
                      isLoading={!sessionsLoaded && !sessionsError}
                      isError={sessionsError}
                      errorMessage={sessionsErrorMessage}
                      onRetry={onRetrySessions}
                      onAttachSession={handleAttachSession}
                      onDeleteSession={onDeleteSession}
                      onRenameSession={onRenameSession}
                      onCloneSession={onCloneSession}
                      onChangeProfile={onChangeProfile}
                      onViewInfo={onViewInfo}
                      onTogglePin={onTogglePin}
                      onMarkRead={onMarkRead}
                      onMarkUnread={onMarkUnread}
                      onNewSession={() => {
                        setShowNewSessionDialog(true);
                        setSidebarOpen(false);
                      }}
                    />
                  </div>
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

      {/* Node switcher — slides over the drawer (Slack-style). Gated on the
          drawer being open so it never floats alone (e.g. after a node switch
          remounts the view with the drawer closed). */}
      <MobileNodePanel
        open={sidebarOpen && railOpen}
        onClose={() => setRailOpen(false)}
        onDismiss={() => setSidebarOpen(false)}
      />

      {/* Dialogs */}
      <NewSessionDialog
        open={showNewSessionDialog}
        onClose={onCloseNewSessionDialog}
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
