import type { Session, SessionStatusInfo, CreateSessionParams } from "@/types";
import type { TabData } from "@/lib/tabs";

export type { SessionStatusInfo } from "@/types";

export type SidePanel = "git" | "editor" | null;

export interface ViewProps {
  sessions: Session[];
  homeDir: string;
  sessionStatuses: Record<string, SessionStatusInfo>;

  // Whether the sessions fetch has landed, so the list can tell an empty list
  // apart from an unanswered one and hold placeholder rows instead of claiming
  // "no sessions". Failures don't appear here; they go to a toast (useSessions).
  sessionsLoaded: boolean;

  // Sidebar
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;

  // Node rail (hidden by default; toggled by the NodeStatus snippet)
  railOpen: boolean;
  setRailOpen: (open: boolean) => void;

  // Active tab info
  activeTab: TabData | null;

  // Dialogs
  showNewSessionDialog: boolean;
  setShowNewSessionDialog: (show: boolean) => void;
  // Dismissal path for the New Session dialog. Distinct from
  // setShowNewSessionDialog(false): App uses it to hand an in-flight create
  // off to a toast when the user closes the dialog before it finishes.
  onCloseNewSessionDialog: () => void;
  showQuickSwitcher: boolean;
  setShowQuickSwitcher: (show: boolean) => void;

  // Session operations
  attachToSession: (session: Session) => void;
  onCreateSession: (params: CreateSessionParams) => void | Promise<void>;
  onDeleteSession: (sessionId: string, deleteBranch?: boolean) => void;
  onRenameSession: (sessionId: string, newName: string) => void;
  onCloneSession: (sessionId: string) => void;
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
  onTogglePin: (sessionId: string, pinned: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;

  // Content
  renderWorkspace: () => React.ReactNode;
}
