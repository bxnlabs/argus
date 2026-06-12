import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { Toaster, toast } from "sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { TabProvider, useTabs } from "@/contexts/TabContext";
import { NodeProvider, useNodeContext } from "@/contexts/NodeContext";
import { Workspace } from "@/components/Workspace";
import { useNotifications } from "@/hooks/useNotifications";
import { useViewport } from "@/hooks/useViewport";
import { useViewportHeight } from "@/hooks/useViewportHeight";
import { useSessions } from "@/hooks/useSessions";
import { useSessionStatuses } from "@/hooks/useSessionStatuses";
import { useCreateSession, useCloneSession } from "@/data/sessions/queries";
import { apiTextFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import { ChangeProfileDialog } from "@/components/ChangeProfileDialog";
import { SessionInfoDialog } from "@/components/SessionInfoDialog";
import { useKeyboardChords, type ChordMap } from "@/hooks/useKeyboardChords";
import { ShortcutHintOverlay } from "@/components/ShortcutHintOverlay";
import { DesktopView } from "@/components/views/DesktopView";
import { MobileView } from "@/components/views/MobileView";
import { NodeOffline } from "@/components/NodeOffline";
import { useGitCheckQuery } from "@/data/git";
import { isMac } from "@/lib/device";
import { nodeScope } from "@/lib/nodeScope";
import type { Session, CreateSessionParams } from "@/types";
import type { SidePanel } from "@/components/views/types";
import type { GitTab, GitTabRequest } from "@/components/GitPanel/GitPanelTabs";

// `railOpen` lives in AppInner (above the node-keyed TabProvider) so switching
// nodes resets the workspace without collapsing the node rail — it stays open
// until toggled from the switcher.
function HomeContent({
  railOpen,
  setRailOpen,
}: {
  railOpen: boolean;
  setRailOpen: (open: boolean) => void;
}) {
  const { baseUrl } = useActiveNode();
  const { activeNode } = useNodeContext();
  // UI State
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);
  const [showQuickSwitcher, setShowQuickSwitcher] = useState(false);
  const [showShortcutsHelp, setShowShortcutsHelp] = useState(false);
  const [activePanel, setActivePanel] = useState<SidePanel>(null);
  const [changeProfileSessionId, setChangeProfileSessionId] = useState<string | null>(null);
  const [infoSessionId, setInfoSessionId] = useState<string | null>(null);
  // Latest Git sub-tab intent. GitPanel re-mounts each time the panel opens, so
  // this must always reflect the most recent chord so it opens to the right tab.
  // Modeled as an event (bumping `seq`) rather than a bare value so repeating a
  // chord re-navigates even when the panel is already open on another tab.
  const [requestedGitTab, setRequestedGitTab] = useState<GitTabRequest>({
    tab: "changes",
    seq: 0,
  });
  const requestGitTab = useCallback((tab: GitTab) => {
    setRequestedGitTab((prev) => ({ tab, seq: prev.seq + 1 }));
  }, []);

  // Tab context
  const {
    tabs,
    activeTab,
    activeTabId,
    addTab,
    closeTab,
    switchTab,
    attachSession,
    detachSession,
    detachSessionById,
  } = useTabs();
  const { isMobile, isHydrated } = useViewport();

  // Data hooks
  const {
    sessions,
    homeDir,
    isLoaded: sessionsLoaded,
    deleteSession,
    renameSession,
    changeProfile,
    togglePin,
    markRead,
    markUnread,
  } = useSessions();
  const createSessionMutation = useCreateSession();
  const createMutateRef = useRef(createSessionMutation.mutateAsync);
  createMutateRef.current = createSessionMutation.mutateAsync;
  const cloneSessionMutation = useCloneSession();
  const cloneMutateRef = useRef(cloneSessionMutation.mutateAsync);
  cloneMutateRef.current = cloneSessionMutation.mutateAsync;

  const focusedSession = activeTab?.sessionId
    ? sessions.find((s) => s.id === activeTab.sessionId)
    : null;
  const activeWorkingDirectory = focusedSession?.working_directory ?? null;
  const { data: isGitRepo = false } = useGitCheckQuery(activeWorkingDirectory);

  // Derive the dialog targets from the live session list (by id) rather than
  // snapshotting the Session object, so the dialogs always reflect current data.
  const changeProfileSession = useMemo(
    () =>
      changeProfileSessionId
        ? (sessions.find((s) => s.id === changeProfileSessionId) ?? null)
        : null,
    [sessions, changeProfileSessionId],
  );
  const infoSession = useMemo(
    () =>
      infoSessionId
        ? (sessions.find((s) => s.id === infoSessionId) ?? null)
        : null,
    [sessions, infoSessionId],
  );

  // Close a dialog whose target session disappears (deleted elsewhere, or
  // restarted under a new id) so it never lingers on stale data.
  useEffect(() => {
    if (!sessionsLoaded) return;
    if (changeProfileSessionId && !changeProfileSession) setChangeProfileSessionId(null);
    if (infoSessionId && !infoSession) setInfoSessionId(null);
  }, [sessionsLoaded, changeProfileSessionId, changeProfileSession, infoSessionId, infoSession]);

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

      // Acknowledge the automatic unread_since when selecting a session.
      // Acknowledge leaves the manual user_marked_unread_at intact, so a sticky
      // "Mark as unread" survives selection.
      const status = sessionStatuses[session.id];
      if (status?.unreadSince) {
        apiTextFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(session.id)}/acknowledge`, {
          method: "POST",
        }).catch(() => {});
      }
    },
    [attachSession, sessionStatuses, baseUrl]
  );

  // Deep-link: auto-attach session from ?session= query param (e.g. from Slack notification)
  const deepLinkHandled = useRef(false);
  useEffect(() => {
    if (!sessionsLoaded || deepLinkHandled.current) return;

    const params = new URLSearchParams(window.location.search);
    const sessionId = params.get("session");
    if (!sessionId) return;

    deepLinkHandled.current = true;

    const session = sessions.find((s) => s.id === sessionId);
    if (session) {
      attachToSession(session);
      // Open sidebar on mobile so user can see the session
      if (isMobile) setSidebarOpen(true);
    }

    // Clear query param to avoid re-triggering on refresh
    const url = new URL(window.location.href);
    url.searchParams.delete("session");
    window.history.replaceState({}, "", url.pathname + url.search + url.hash);
  }, [sessionsLoaded, sessions, attachToSession, isMobile]);

  // Open sidebar on desktop at startup (mobile starts collapsed)
  useEffect(() => {
    if (isHydrated && !isMobile) setSidebarOpen(true);
  }, [isHydrated]); // eslint-disable-line react-hooks/exhaustive-deps

  // Keyboard shortcut: Cmd+K (mac) / Ctrl+K (else) to open quick switcher.
  // Bound to the platform-primary modifier ONLY (not both) so on macOS it
  // doesn't shadow readline's Ctrl+K (kill-to-end-of-line) in the terminal.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((isMac() ? e.metaKey : e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setShowQuickSwitcher(true);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  // Leader-key chord tree. `g`/`e` are conditionally included on the same
  // availability rules as the ViewModeRail buttons (Git only on a repo, Editor
  // only with a working dir) — which keeps unavailable actions out of the hint
  // overlay and dodges the open-then-auto-close flicker from Workspace's reset.
  const bindings: ChordMap = useMemo(() => {
    const switchRelative = (delta: number) => {
      const i = tabs.findIndex((t) => t.id === activeTabId);
      if (i === -1) return;
      switchTab(tabs[(i + delta + tabs.length) % tabs.length].id);
    };
    return {
      n: { label: "New session", run: () => setShowNewSessionDialog(true) },
      "=": { label: "New tab", run: () => addTab() },
      "-": { label: "Close tab", run: () => closeTab(activeTabId) },
      ArrowLeft: { label: "Previous tab", run: () => switchRelative(-1) },
      ArrowRight: { label: "Next tab", run: () => switchRelative(1) },
      t: { label: "Terminal", run: () => setActivePanel(null) },
      // Session-scoped shortcuts: only offered when the active tab actually has
      // a session attached, so the hint overlay never advertises a no-op (same
      // conditional-registration approach as `g`/`e` below). Detach mirrors the
      // tab-bar button; info opens the session-info dialog for the active tab.
      ...(activeTab?.sessionId
        ? {
            d: { label: "Detach session", run: () => detachSession() },
            i: {
              label: "Session info",
              run: () => setInfoSessionId(activeTab.sessionId),
            },
          }
        : {}),
      ...(isGitRepo
        ? {
            g: {
              label: "Git",
              run: () => {
                requestGitTab("changes");
                setActivePanel("git");
              },
              children: {
                h: {
                  label: "History",
                  run: () => {
                    requestGitTab("history");
                    setActivePanel("git");
                  },
                },
                c: {
                  label: "Compare",
                  run: () => {
                    requestGitTab("compare");
                    setActivePanel("git");
                  },
                },
              },
            },
          }
        : {}),
      ...(activeWorkingDirectory
        ? {
            e: {
              label: "Editor",
              run: () => setActivePanel("editor"),
            },
          }
        : {}),
      "?": { label: "Show all shortcuts", run: () => setShowShortcutsHelp(true) },
    };
  }, [tabs, activeTabId, isGitRepo, activeWorkingDirectory, addTab, closeTab, switchTab, requestGitTab, activeTab, detachSession]);

  // Chord engine — desktop only (touch devices have no leader key).
  const { pending } = useKeyboardChords(bindings, { enabled: !isMobile });

  const leaderLabel = isMac() ? "⌘ ;" : "Ctrl ;";
  const extraShortcuts = [
    { keys: isMac() ? "⌘ K" : "Ctrl K", label: "Search / quick-switch" },
    { keys: "Esc", label: "Cancel chord / return to terminal" },
  ];

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
          // via WebSocket to /api/node/ws/sessions/{id}. Session list refreshes
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

  // Clone session handler — creates a sibling session sharing the same context
  // (worktree, profile, provider) and attaches to it.
  const handleCloneSession = useCallback(
    async (sessionId: string) => {
      try {
        const result = await cloneMutateRef.current(sessionId);
        if (result.session) {
          attachToSession(result.session);
        }
      } catch (err) {
        console.error("Failed to clone session:", err);
        toast.error("Failed to clone session");
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

  // Change-profile handler — recreates the session with the new profile.
  const handleChangeProfileApply = useCallback(
    async (sessionId: string, profile: string | null) => {
      try {
        await changeProfile(sessionId, profile);
      } catch (err) {
        console.error("Failed to change profile:", err);
        toast.error("Failed to change profile");
      }
    },
    [changeProfile],
  );

  const handleTogglePin = useCallback(
    async (sessionId: string, pinned: boolean) => {
      try {
        await togglePin(sessionId, pinned);
      } catch (err) {
        console.error("Failed to update session:", err);
        toast.error("Failed to update session");
      }
    },
    [togglePin],
  );

  const handleMarkRead = useCallback(
    async (sessionId: string) => {
      try {
        await markRead(sessionId);
      } catch (err) {
        console.error("Failed to mark session read:", err);
        toast.error("Failed to mark session read");
      }
    },
    [markRead],
  );

  const handleMarkUnread = useCallback(
    async (sessionId: string) => {
      try {
        await markUnread(sessionId);
      } catch (err) {
        console.error("Failed to mark session unread:", err);
        toast.error("Failed to mark session unread");
      }
    },
    [markUnread],
  );

  // Render the main workspace
  const renderWorkspace = useCallback(
    () => {
      // An unreachable active node would otherwise render as an empty workspace,
      // indistinguishable from a node with no sessions. Surface it explicitly —
      // but only once its poll has SETTLED offline: `pending` is the first
      // "Connecting…" poll (true on cold start, including the local node), so
      // gating on !pending avoids flashing the offline screen before we know.
      if (activeNode && !activeNode.online && !activeNode.pending) {
        return (
          <NodeOffline
            name={activeNode.name}
            onMenuClick={isMobile ? () => setSidebarOpen(true) : undefined}
          />
        );
      }
      return (
        <Workspace
          sessions={sessions}
          activePanel={activePanel}
          setActivePanel={setActivePanel}
          activeWorkingDirectory={activeWorkingDirectory}
          isGitRepo={isGitRepo}
          requestedGitTab={requestedGitTab}
          onMenuClick={isMobile ? () => setSidebarOpen(true) : undefined}
          onSelectSession={handleSelectSession}
          onNewSession={() => setShowNewSessionDialog(true)}
        />
      );
    },
    [activeNode, sessions, isMobile, handleSelectSession, activePanel, setActivePanel, activeWorkingDirectory, isGitRepo, requestedGitTab]
  );

  const viewProps = {
    sessions,
    homeDir,
    sessionStatuses,
    sidebarOpen,
    setSidebarOpen,
    railOpen,
    setRailOpen,
    activeTab,
    showNewSessionDialog,
    setShowNewSessionDialog,
    showQuickSwitcher,
    setShowQuickSwitcher,
    attachToSession,
    onCreateSession: handleCreateSession,
    onDeleteSession: handleDeleteSession,
    onRenameSession: handleRenameSession,
    onCloneSession: handleCloneSession,
    onChangeProfile: (session: Session) => setChangeProfileSessionId(session.id),
    onViewInfo: (session: Session) => setInfoSessionId(session.id),
    onTogglePin: handleTogglePin,
    onMarkRead: handleMarkRead,
    onMarkUnread: handleMarkUnread,
    renderWorkspace,
  };

  if (isMobile) {
    return (
      <>
        <MobileView {...viewProps} />
        <ChangeProfileDialog
          session={changeProfileSession}
          onClose={() => setChangeProfileSessionId(null)}
          onApply={handleChangeProfileApply}
        />
        <SessionInfoDialog
          session={infoSession}
          status={infoSession ? sessionStatuses[infoSession.id]?.status : undefined}
          homeDir={homeDir}
          onClose={() => setInfoSessionId(null)}
        />
      </>
    );
  }

  return (
    <>
      <DesktopView {...viewProps} />
      <ShortcutHintOverlay
        pending={pending}
        bindings={bindings}
        leaderLabel={leaderLabel}
        helpOpen={showShortcutsHelp}
        onHelpOpenChange={setShowShortcutsHelp}
        extraShortcuts={extraShortcuts}
      />
      <ChangeProfileDialog
        session={changeProfileSession}
        onClose={() => setChangeProfileSessionId(null)}
        onApply={handleChangeProfileApply}
      />
      <SessionInfoDialog
        session={infoSession}
        status={infoSession ? sessionStatuses[infoSession.id]?.status : undefined}
        homeDir={homeDir}
        onClose={() => setInfoSessionId(null)}
      />
    </>
  );
}

export function App() {
  return (
    <TooltipProvider>
      <NodeProvider>
        <AppInner />
      </NodeProvider>
    </TooltipProvider>
  );
}

function AppInner() {
  const { activeNodeId, activeNode, isLoaded } = useNodeContext();
  // Node-rail visibility lives here, above the node-keyed TabProvider, so it
  // survives the workspace remount on node switch.
  const [railOpen, setRailOpen] = useState(false);
  // Wait for the registry to settle before mounting the workspace. Otherwise,
  // when a remote node is the persisted selection, HomeContent would mount with
  // the default (local) origin and fire node-scoped fetches against the wrong
  // node before the active origin is applied. The registry is same-origin and
  // fast, so this is a brief gate; on a registry error isLoaded still settles
  // and the selection falls back to the local node.
  if (!isLoaded) return <div className="bg-background h-app" />;
  // Scope tabs by id+origin so the workspace remounts AND persists under the
  // same key — editing a manual node's URL (same id) yields fresh tabs. Same
  // token as useActiveNode's cache scope (both via nodeScope).
  const scope = nodeScope(activeNodeId, activeNode?.url ?? "");
  return (
    <TabProvider key={scope} nodeScope={scope}>
      <HomeContent railOpen={railOpen} setRailOpen={setRailOpen} />
      <Toaster
        theme="dark"
        position="bottom-right"
        richColors
        toastOptions={{ className: "argus-toast" }}
      />
    </TabProvider>
  );
}
