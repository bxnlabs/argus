import type { Session, SessionStatusInfo, CreateSessionParams } from "@/types";
import type { TabData } from "@/lib/tabs";

export type { SessionStatusInfo } from "@/types";

export type SidePanel = "git" | "editor" | null;

export interface ViewProps {
  sessions: Session[];
  homeDir: string;
  sessionStatuses: Record<string, SessionStatusInfo>;

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
  showQuickSwitcher: boolean;
  setShowQuickSwitcher: (show: boolean) => void;

  // Session operations
  attachToSession: (session: Session) => void;
  onCreateSession: (params: CreateSessionParams) => void;
  onDeleteSession: (sessionId: string, deleteBranch?: boolean) => void;
  onRenameSession: (sessionId: string, newName: string) => void;
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
  onTogglePin: (sessionId: string, pinned: boolean) => void;
  onMarkRead: (sessionId: string) => void;
  onMarkUnread: (sessionId: string) => void;

  // Content
  renderWorkspace: () => React.ReactNode;
}
