import { useRef, useCallback, useEffect, memo, useState } from "react";
import { useTabs } from "@/contexts/TabContext";
import { useViewport } from "@/hooks/useViewport";
import type { Session } from "@/types";
import type { SidePanel } from "@/components/views/types";
import { cn } from "@/lib/utils";
import { shellEscape } from "@/lib/shell";
import { MobileTabBar } from "./MobileTabBar";
import { DesktopTabBar } from "./DesktopTabBar";
import { RightSidebar } from "./RightSidebar";
import { GitPanel } from "@/components/GitPanel";
import { FileExplorer } from "@/components/FileExplorer";
import { Terminal } from "@/components/Terminal";
import { FilePicker } from "@/components/FilePicker";
import { useFileUpload } from "@/data/files/mutations";
import { toast } from "sonner";
import type { TerminalHandle } from "@/components/Terminal";

interface WorkspaceProps {
  sessions: Session[];
  activePanel: SidePanel;
  setActivePanel: (panel: SidePanel) => void;
  activeWorkingDirectory: string | null;
  isGitRepo: boolean;
  onMenuClick?: () => void;
  onSelectSession?: (sessionId: string) => void;
  onNewSession?: () => void;
}

export const Workspace = memo(function Workspace({
  sessions,
  activePanel,
  setActivePanel,
  activeWorkingDirectory,
  isGitRepo,
  onMenuClick,
  onSelectSession,
  onNewSession,
}: WorkspaceProps) {
  const { isMobile } = useViewport();
  const {
    tabs,
    activeTabId,
    activeTab,
    addTab,
    closeTab,
    switchTab,
    detachSession,
  } = useTabs();

  const [selectMode, setSelectMode] = useState(false);
  const [showFilePicker, setShowFilePicker] = useState(false);
  const terminalRefs = useRef<Map<string, TerminalHandle>>(new Map());
  const { mutateAsync: uploadFiles } = useFileUpload();

  // Get sendInput for the active terminal
  const sendInputToActiveTerminal = useCallback(
    (data: string) => {
      const handle = terminalRefs.current.get(activeTabId);
      handle?.sendInput(data);
    },
    [activeTabId]
  );

  // Handle picked files — type paths into terminal (no trailing return)
  const handleFilesPicked = useCallback(
    (paths: string[]) => {
      sendInputToActiveTerminal(paths.map(shellEscape).join(" "));
    },
    [sendInputToActiveTerminal]
  );

  // Handle drag-dropped files — upload then type paths (no trailing return)
  const handleFilesDropped = useCallback(
    async (files: File[]) => {
      try {
        const result = await uploadFiles(files);
        for (const file of result.files) {
          toast.success(`Uploaded ${file.name}`);
        }
        const paths = result.files.map((f) => f.path);
        sendInputToActiveTerminal(paths.map(shellEscape).join(" "));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Upload failed");
      }
    },
    [uploadFiles, sendInputToActiveTerminal]
  );

  const session = activeTab
    ? sessions.find((s) => s.id === activeTab.sessionId)
    : null;

  // Auto-switch to terminal when working directory is lost (e.g., detach, empty tab)
  useEffect(() => {
    if ((activePanel === "git" || activePanel === "editor") && !activeWorkingDirectory) {
      setActivePanel(null);
    }
  }, [activePanel, activeWorkingDirectory, setActivePanel]);

  // Auto-clear select mode when switching away from terminal
  useEffect(() => {
    if (activePanel !== null) {
      setSelectMode(false);
    }
  }, [activePanel]);

  // Restore terminal focus when FilePicker closes
  const prevShowFilePicker = useRef(showFilePicker);
  useEffect(() => {
    if (prevShowFilePicker.current && !showFilePicker) {
      terminalRefs.current.get(activeTabId)?.focus();
    }
    prevShowFilePicker.current = showFilePicker;
  }, [showFilePicker, activeTabId]);

  // Resolve session ID for a tab's WebSocket connection
  const getTabSessionId = useCallback(
    (tab: (typeof tabs)[0]): string | null => {
      return tab.sessionId || null;
    },
    []
  );

  // Mobile swipe gesture for tab switching
  const touchStartX = useRef<number | null>(null);
  const currentTabIndex = tabs.findIndex((t) => t.id === activeTabId);
  const SWIPE_THRESHOLD = 120;

  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    touchStartX.current = e.touches[0].clientX;
  }, []);

  const handleTouchEnd = useCallback(
    (e: React.TouchEvent) => {
      if (touchStartX.current === null) return;

      const diff = e.changedTouches[0].clientX - touchStartX.current;
      touchStartX.current = null;

      if (Math.abs(diff) <= SWIPE_THRESHOLD) return;

      const nextIndex = diff > 0 ? currentTabIndex - 1 : currentTabIndex + 1;
      if (nextIndex >= 0 && nextIndex < tabs.length) {
        switchTab(tabs[nextIndex].id);
      }
    },
    [currentTabIndex, tabs, switchTab]
  );

  return (
    <div
      className={cn(
        "flex h-full w-full overflow-hidden",
        !isMobile && "shadow-lg shadow-black/10 dark:shadow-black/30"
      )}
    >
      {/* Main column: tab bar + content */}
      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
      {/* Tab Bar */}
      {isMobile ? (
        <MobileTabBar
          tabs={tabs}
          activeTabId={activeTabId}
          session={session}
          sessions={sessions}
          activePanel={activePanel}
          onSetActivePanel={setActivePanel}
          isGitEnabled={isGitRepo}
          isEditorEnabled={!!activeWorkingDirectory}
          selectMode={selectMode}
          onEnterSelectMode={() => {
            setActivePanel(null);
            setSelectMode(true);
          }}
          onExitSelectMode={() => setSelectMode(false)}
          onMenuClick={onMenuClick}
          onTabSwitch={switchTab}
          onTabClose={closeTab}
          onTabAdd={addTab}
          onNewSession={onNewSession}
          hasAttachedSession={!!activeTab?.sessionId}
          onDetach={detachSession}
        />
      ) : (
        <DesktopTabBar
          tabs={tabs}
          activeTabId={activeTabId}
          sessions={sessions}
          hasAttachedSession={!!activeTab?.sessionId}
          onTabSwitch={switchTab}
          onTabClose={closeTab}
          onTabAdd={addTab}
          onDetach={detachSession}
        />
      )}

      {/* Content */}
      <div
        className="relative min-h-0 w-full flex-1 pl-1"
        onTouchStart={isMobile ? handleTouchStart : undefined}
        onTouchEnd={isMobile ? handleTouchEnd : undefined}
      >
        {activePanel === "git" && (
          activeWorkingDirectory ? (
            <GitPanel workingDirectory={activeWorkingDirectory} />
          ) : (
            <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
              Attach a session to view git status
            </div>
          )
        )}
        {activePanel === "editor" && (
          activeWorkingDirectory ? (
            <FileExplorer workingDirectory={activeWorkingDirectory} />
          ) : (
            <div className="flex h-full items-center justify-center text-muted-foreground text-sm">
              Attach a session to browse files
            </div>
          )
        )}
        {/* Always render terminals so xterm instances stay alive across panel switches */}
        <div className={activePanel !== null ? "hidden" : "contents"}>
          {tabs.map((tab) => {
            const isActive = tab.id === activeTab?.id;

            return (
              <div
                key={tab.id}
                className={isActive ? "h-full w-full" : "hidden"}
              >
                <Terminal
                  ref={(handle) => {
                    if (handle) {
                      terminalRefs.current.set(tab.id, handle);
                    } else {
                      terminalRefs.current.delete(tab.id);
                    }
                  }}
                  sessionName={getTabSessionId(tab)}
                  selectMode={isActive ? selectMode : false}
                  onFilesDropped={handleFilesDropped}
                  onAttachments={() => setShowFilePicker(true)}
                  workingDirectory={activeWorkingDirectory}
                />
              </div>
            );
          })}
        </div>
      </div>
      </div>

      {/* Right sidebar — desktop only */}
      {!isMobile && (
        <RightSidebar
          activePanel={activePanel}
          onSetActivePanel={setActivePanel}
          isGitEnabled={isGitRepo}
          isEditorEnabled={!!activeWorkingDirectory}
          onAttachments={() => setShowFilePicker(true)}
        />
      )}

      <FilePicker
        open={showFilePicker}
        onOpenChange={setShowFilePicker}
        onPick={handleFilesPicked}
        searchPath={activeWorkingDirectory ?? undefined}
      />
    </div>
  );
});
