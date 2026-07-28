import {
  useState,
  useEffect,
  useLayoutEffect,
  useCallback,
  useRef,
  useMemo,
} from "react";
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
import { isMac, isTouchDevice, isPhoneSized } from "@/lib/device";
import { buildNodeSwitchBindings } from "@/lib/nodeShortcuts";
import { nodeScope } from "@/lib/nodeScope";
import { resolveCreateToastDecision } from "@/lib/createSessionToast";
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
  const { activeNode, nodes, setActiveNode } = useNodeContext();
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
    attachSessionToTab,
    detachSession,
    detachSessionById,
  } = useTabs();
  const { isMobile, isHydrated } = useViewport();

  // railOpen defaults open on desktop; on a desktop→mobile switch (e.g. a tablet
  // entering split-screen) close it so the node panel can't surface over the
  // drawer. Keyed on isMobile only — not railOpen — so it fires on the
  // transition, not when the user toggles the panel open on mobile. A layout
  // effect lands the close before paint, so the panel never flashes open for a
  // frame during the transition.
  useLayoutEffect(() => {
    if (isMobile) setRailOpen(false);
  }, [isMobile, setRailOpen]);

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

  // Handoff bookkeeping: the name to show in the toast, and the id of the
  // toast once one exists. Single-slot by design — it assumes at most one
  // create in flight, which holds only because NewSessionDialog serialises
  // submits, and does so with its own synchronous lock rather than `isCreating`
  // alone (which is delivered asynchronously). Allowing concurrent creates
  // means making both of these per-create too.
  const pendingCreateNameRef = useRef("");
  const createToastRef = useRef<string | number | null>(null);
  // Whether the create this component dispatched is still in flight. Set
  // synchronously around the call rather than read from `isCreating`, for the
  // same reason the dialog holds its submit lock in a ref: `isCreating` is
  // delivered asynchronously, so a close arriving before that snapshot reaches
  // React would skip the handoff toast and complete the create in silence.
  const createInFlightRef = useRef(false);

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

  // Acknowledge the automatic unread_since when a session is opened.
  // Acknowledge leaves the manual user_marked_unread_at intact, so a sticky
  // "Mark as unread" survives selection. Split out so a *skipped* attach does
  // not acknowledge a session the user never actually opened.
  const acknowledgeUnread = useCallback(
    (session: Session) => {
      const status = sessionStatuses[session.id];
      if (!status?.unreadSince) return;
      apiTextFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(session.id)}/acknowledge`, {
        method: "POST",
      }).catch(() => {});
    },
    [sessionStatuses, baseUrl],
  );

  // Attach session to active tab — just updates tab state.
  // The Terminal component handles WebSocket connection based on the session ID.
  const attachToSession = useCallback(
    (session: Session) => {
      attachSession(session.id);
      acknowledgeUnread(session);
    },
    [attachSession, acknowledgeUnread],
  );

  // Where background work should land: the tab that started it, and what that
  // tab held at the time. Held in a ref so the async handlers below stay stable
  // across tab switches, matching the mutate-ref pattern above.
  const attachTargetRef = useRef<{ tabId: string; sessionId: string | null }>({
    tabId: activeTabId,
    sessionId: null,
  });
  attachTargetRef.current = {
    tabId: activeTabId,
    sessionId: activeTab?.sessionId ?? null,
  };

  // Attach the result of background work to the tab that started it — but only
  // if that tab is still what it was. Returns whether it attached, so callers
  // can fall back to a toast rather than dropping the session into whatever tab
  // the user has moved to.
  const attachToTarget = useCallback(
    (
      target: { tabId: string; sessionId: string | null },
      session: Session,
    ): boolean => {
      const attached = attachSessionToTab(
        target.tabId,
        session.id,
        target.sessionId,
      );
      if (attached) acknowledgeUnread(session);
      return attached;
    },
    [attachSessionToTab, acknowledgeUnread],
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
      b: { label: "Toggle sidebar", run: () => setSidebarOpen((v) => !v) },
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
      ...buildNodeSwitchBindings(nodes, setActiveNode),
      "?": { label: "Show all shortcuts", run: () => setShowShortcutsHelp(true) },
    };
  }, [tabs, activeTabId, isGitRepo, activeWorkingDirectory, addTab, closeTab, switchTab, requestGitTab, activeTab, detachSession, nodes, setActiveNode]);

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

  // Resolves a handoff toast if the user dismissed the dialog mid-create, and
  // is also the fallback when the create could not be attached to the tab that
  // started it. With neither, success stays silent as it always has.
  const resolveCreateToast = useCallback(
    (
      outcome: "success" | "error",
      name: string,
      action?: { label: string; onClick: () => void },
    ) => {
      const id = createToastRef.current;
      createToastRef.current = null;
      const decision = resolveCreateToastDecision(outcome, name, id, action);
      if (!decision) return;
      if (decision.kind === "error") {
        toast.error(decision.message, decision.options);
      } else {
        toast.success(decision.message, decision.options);
      }
    },
    [],
  );

  // Create session handler
  const handleCreateSession = useCallback(
    async (params: CreateSessionParams) => {
      const name = params.name?.trim() || "session";
      pendingCreateNameRef.current = name;
      createInFlightRef.current = true;
      const target = attachTargetRef.current;

      try {
        const result = await createMutateRef.current({
          name: params.name,
          source: params.source,
          provider_type: params.provider_type,
          auto_approve: params.auto_approve || false,
          profile: params.profile,
          branch: params.branch,
        });

        // A handoff toast is the record that the user dismissed the dialog
        // mid-create. Whatever is on screen now is not this create's form —
        // most likely one reopened while the first create was still in
        // flight (its fieldset stays disabled until that settles, so it's
        // inert, not a queued second submit) — so closing it here would yank
        // it away on someone else's completion.
        if (createToastRef.current === null) setShowNewSessionDialog(false);

        let openAction: { label: string; onClick: () => void } | undefined;
        // The server may normalise or dedupe the requested name, so prefer
        // what it actually created — falling back to the request only if the
        // response somehow carries no session (the toast still needs a name).
        let displayName = name;
        if (result.session) {
          const session = result.session;
          displayName = session.name;
          // Attach to the tab that opened the dialog — Terminal will
          // auto-connect via WebSocket to /api/node/ws/sessions/{id}. Session
          // list refreshes automatically via TanStack Query's onSuccess
          // invalidation.
          if (!attachToTarget(target, session)) {
            openAction = {
              label: "Open",
              onClick: () => attachToSession(session),
            };
          }
        }
        resolveCreateToast("success", displayName, openAction);
      } catch (err) {
        console.error("Failed to create session:", err);
        resolveCreateToast("error", name);
      } finally {
        createInFlightRef.current = false;
      }
    },
    [attachToTarget, attachToSession, resolveCreateToast],
  );

  // Dismissing the dialog mid-create hands the work off to a toast, so a
  // multi-minute clone never traps the modal. A create that already settled
  // leaves the ref false, so the behaviour there is unchanged: silence on
  // success, an error toast on failure.
  const handleCloseNewSessionDialog = useCallback(() => {
    setShowNewSessionDialog(false);
    if (createInFlightRef.current && createToastRef.current === null) {
      createToastRef.current = toast.loading(
        `Creating ${pendingCreateNameRef.current}…`,
      );
    }
  }, []);

  // Clone session handler — creates a sibling session sharing the same context
  // (worktree, profile, provider) and attaches to it.
  const handleCloneSession = useCallback(
    async (sessionId: string) => {
      const target = attachTargetRef.current;
      try {
        const result = await cloneMutateRef.current({ sessionId });
        if (result.session) {
          const session = result.session;
          if (!attachToTarget(target, session)) {
            // The tab that started this is gone or in use. A clone can take
            // minutes, so say it finished rather than leaving the user to
            // notice a new sidebar row.
            toast.success(`Cloned ${session.name}`, {
              action: {
                label: "Open",
                onClick: () => attachToSession(session),
              },
              // Longer than sonner's ~4s default: this is the only recovery
              // affordance for the clone, and the user may be looking at
              // another tab when it appears.
              duration: 10000,
            });
          }
        }
      } catch (err) {
        console.error("Failed to clone session:", err);
        toast.error("Failed to clone session");
      }
    },
    [attachToTarget, attachToSession]
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
        // Only close if the dialog is still showing the session that just
        // finished. A restart takes seconds, so dismissing it and opening
        // another session's dialog meanwhile is ordinary — and closing that
        // one on this result would yank it out from under the user.
        setChangeProfileSessionId((current) =>
          current === sessionId ? null : current,
        );
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
    onCloseNewSessionDialog: handleCloseNewSessionDialog,
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
  // survives the workspace remount on node switch. Default it open on desktop
  // (toggle via the node switcher tile); on mobile it stays closed so the
  // overlay panel never pops open over the drawer. Mirrors useViewport's mobile
  // check so both agree on what "desktop" means.
  const [railOpen, setRailOpen] = useState(
    () => !(isTouchDevice && isPhoneSized()),
  );
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
