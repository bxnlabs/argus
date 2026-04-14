import { useState, useEffect, useCallback, useRef } from "react";
import { Toaster, toast } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { TabProvider, useTabs } from "@/contexts/TabContext";
import { Workspace } from "@/components/Workspace";
import { useNotifications } from "@/hooks/useNotifications";
import { useViewport } from "@/hooks/useViewport";
import { useViewportHeight } from "@/hooks/useViewportHeight";
import { useSessions } from "@/hooks/useSessions";
import { useSessionStatuses } from "@/hooks/useSessionStatuses";
import { useCreateSession } from "@/data/sessions/queries";
import { DesktopView } from "@/components/views/DesktopView";
import { MobileView } from "@/components/views/MobileView";
import { useGitCheckQuery } from "@/data/git";
import type { Session, CreateSessionParams } from "@/types";
import type { SidePanel } from "@/components/views/types";

function HomeContent() {
  // UI State
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
  const [showQuickSwitcher, setShowQuickSwitcher] = useState(false);
  const [activePanel, setActivePanel] = useState<SidePanel>(null);

  // Tab context
  const { tabs, activeTab, attachSession, detachSessionById } = useTabs();
  const { isMobile, isHydrated } = useViewport();

  // Data hooks
  const { sessions, homeDir, isLoaded: sessionsLoaded, deleteSession, renameSession } = useSessions();
  const createSessionMutation = useCreateSession();
  const createMutateRef = useRef(createSessionMutation.mutateAsync);
  createMutateRef.current = createSessionMutation.mutateAsync;

  const focusedSession = activeTab?.sessionId
    ? sessions.find((s) => s.id === activeTab.sessionId)
    : null;
  const activeWorkingDirectory = focusedSession?.working_directory ?? null;
  const { data: isGitRepo = false } = useGitCheckQuery(activeWorkingDirectory);

  // Detach tabs whose session no longer exists (e.g. stale localStorage after restart).
  // Runs once after the first successful sessions fetch to avoid racing with
  // newly created sessions that haven't appeared in the list yet.
  const staleCleaned = useRef(false);
  useEffect(() => {
    if (!sessionsLoaded || staleCleaned.current) return;
    staleCleaned.current = true;
    const sessionIds = new Set(sessions.map((s) => s.id));
    for (const tab of tabs) {
      if (tab.sessionId && !sessionIds.has(tab.sessionId)) {
        detachSessionById(tab.sessionId);
      }
    }
  }, [sessionsLoaded, sessions, tabs, detachSessionById]);

  // Set CSS variable for viewport height (handles mobile keyboard)
  useViewportHeight();

  // Notifications
  const { checkStateChanges } = useNotifications();

  // Session statuses
  const { sessionStatuses } = useSessionStatuses({
    sessions,
    activeSessionId: activeTab?.sessionId,
    checkStateChanges,
  });

  // Attach session to active tab — just updates tab state.
  // The Terminal component handles WebSocket connection based on the session ID.
  const attachToSession = useCallback(
    (session: Session) => {
      attachSession(session.id);

      // Acknowledge unread state when selecting a session
      const status = sessionStatuses[session.id];
      if (status?.unreadSince) {
        fetch(`${import.meta.env.VITE_NODE_URL || ""}/node/api/sessions/${encodeURIComponent(session.id)}/acknowledge`, {
          method: "POST",
        }).catch(() => {});
      }
    },
    [attachSession, sessionStatuses]
  );

  // Open sidebar on desktop at startup (mobile starts collapsed)
  useEffect(() => {
    if (isHydrated && !isMobile) setSidebarOpen(true);
  }, [isHydrated]); // eslint-disable-line react-hooks/exhaustive-deps

  // Keyboard shortcut: Cmd+K to open quick switcher
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setShowQuickSwitcher(true);
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === "G") {
        e.preventDefault();
        setActivePanel((prev) => (prev === "git" ? null : "git"));
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === "E") {
        e.preventDefault();
        setActivePanel((prev) => (prev === "editor" ? null : "editor"));
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  // Session selection handler
  const handleSelectSession = useCallback(
    (sessionId: string) => {
      const session = sessions.find((s) => s.id === sessionId);
      if (session) attachToSession(session);
    },
    [sessions, attachToSession]
  );

  // Create session handler
  const handleCreateSession = useCallback(
    async (params: CreateSessionParams) => {
      setShowNewSessionDialog(false);

      try {
        const result = await createMutateRef.current({
          name: params.name,
          source: params.source,
          provider_type: params.provider_type,
          auto_approve: params.auto_approve || false,
          profile: params.profile,
          branch: params.branch,
        });

        if (result.session) {
          // Attach to the newly created session — Terminal will auto-connect
          // via WebSocket to /node/ws/sessions/{id}. Session list refreshes
          // automatically via TanStack Query's onSuccess invalidation.
          attachToSession(result.session);
        }
      } catch (err) {
        console.error("Failed to create session:", err);
        toast.error("Failed to create session");
      }
    },
    [attachToSession]
  );

  // Delete session handler
  const handleDeleteSession = useCallback(
    async (sessionId: string, deleteBranch?: boolean) => {
      try {
        const result = await deleteSession(sessionId, deleteBranch);
        if (!result) return; // user cancelled
        detachSessionById(sessionId);
        if (deleteBranch && !result.branch_deleted) {
          toast.warning("Session deleted, but the branch could not be removed");
        }
      } catch (err) {
        console.error("Failed to delete session:", err);
        toast.error("Failed to delete session");
      }
    },
    [deleteSession, detachSessionById]
  );

  // Rename session handler
  const handleRenameSession = useCallback(
    async (sessionId: string, newName: string) => {
      try {
        await renameSession(sessionId, newName);
      } catch (err) {
        console.error("Failed to rename session:", err);
        toast.error("Failed to rename session");
      }
    },
    [renameSession]
  );

  // Render the main workspace
  const renderWorkspace = useCallback(
    () => (
      <Workspace
        sessions={sessions}
        activePanel={activePanel}
        setActivePanel={setActivePanel}
        activeWorkingDirectory={activeWorkingDirectory}
        isGitRepo={isGitRepo}
        onMenuClick={isMobile ? () => setSidebarOpen(true) : undefined}
        onSelectSession={handleSelectSession}
        onNewSession={() => setShowNewSessionDialog(true)}
      />
    ),
    [sessions, isMobile, handleSelectSession, activePanel, setActivePanel, activeWorkingDirectory, isGitRepo]
  );

  const viewProps = {
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
    onCreateSession: handleCreateSession,
    onDeleteSession: handleDeleteSession,
    onRenameSession: handleRenameSession,
    renderWorkspace,
  };

  if (isMobile) {
    return <MobileView {...viewProps} />;
  }

  return <DesktopView {...viewProps} />;
}

export function App() {
  return (
    <TooltipProvider>
      <TabProvider>
        <HomeContent />
        <Toaster theme="dark" position="bottom-center" />
      </TabProvider>
    </TooltipProvider>
  );
}
