import {
  useRef,
  forwardRef,
  useImperativeHandle,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import "@xterm/xterm/css/xterm.css";
import { WifiOff, Paperclip } from "lucide-react";
import { useFileDrop } from "@/hooks/useFileDrop";
import { cn } from "@/lib/utils";
import { ComposeBar } from "./ComposeBar";
import { SearchBar } from "./SearchBar";
import { TerminalToolbar } from "./TerminalToolbar";
import { useTerminalConnection, useTerminalSearch } from "./hooks";
import type { TerminalScrollState } from "./hooks";
import { useViewport } from "@/hooks/useViewport";

export type { TerminalScrollState };

export interface TerminalHandle {
  sendInput: (data: string) => void;
  focus: () => void;
  getScrollState: () => TerminalScrollState | null;
  restoreScrollState: (state: TerminalScrollState) => void;
}

interface TerminalProps {
  /** Session ID — WebSocket connects to /api/node/ws/sessions/{id} */
  sessionName: string | null;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onBeforeUnmount?: (scrollState: TerminalScrollState) => void;
  initialScrollState?: TerminalScrollState;
  /** When true, shows a text overlay for touch-based text selection (mobile only) */
  selectMode?: boolean;
  /** Called when files are dropped onto the terminal (desktop drag-drop) */
  onFilesDropped?: (files: File[]) => void;
  /** Session working directory, used to anchor file search */
  workingDirectory?: string | null;
}

export const Terminal = forwardRef<TerminalHandle, TerminalProps>(
  function Terminal(
    { sessionName, onConnected, onDisconnected, onBeforeUnmount, initialScrollState, selectMode = false, onFilesDropped, workingDirectory },
    ref
  ) {
    const terminalRef = useRef<HTMLDivElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const { isMobile } = useViewport();

    const {
      connectionState,
      xtermRef,
      searchAddonRef,

      sendInput,
      sendText,
      focus,
      getScrollState,
      restoreScrollState,
      reconnect,
    } = useTerminalConnection({
      terminalRef,
      sessionName,
      onConnected,
      onDisconnected,
      onBeforeUnmount,
      initialScrollState,
      isMobile,
      selectMode,
    });

    const {
      searchVisible,
      searchQuery,
      setSearchQuery,
      searchInputRef,
      closeSearch,
      findNext,
      findPrevious,
    } = useTerminalSearch(searchAddonRef, xtermRef);

    // Drag-and-drop file upload — disabled when not connected
    const isConnected = connectionState === "connected";
    const { isDragging, dragHandlers } = useFileDrop(
      (files) => onFilesDropped?.(files),
      { disabled: !onFilesDropped || !isConnected }
    );


    // Extract terminal text for select mode overlay (snapshot on enter)
    const terminalText = useMemo(() => {
      if (!selectMode || !xtermRef.current) return "";

      const term = xtermRef.current;
      const buffer = term.buffer.active;
      const startRow = Math.max(0, buffer.baseY - 500);
      const endRow = buffer.baseY + term.rows;
      const lines: string[] = [];

      for (let i = startRow; i < endRow; i++) {
        const line = buffer.getLine(i);
        if (line) lines.push(line.translateToString(true));
      }

      return lines.join("\n");
      // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [selectMode]);

    // Expose imperative methods
    useImperativeHandle(ref, () => ({
      sendInput,
      focus,
      getScrollState,
      restoreScrollState,
    }));

    const handleContainerClick = useCallback(() => focus(), [focus]);

    // How far the compose panel currently overflows its one-line spacer. The
    // terminal is shifted up by this much with a transform rather than being
    // resized: transforms don't affect layout, so FitAddon never refits and the
    // PTY never sees a SIGWINCH mid-typing (which would repaint the agent's TUI
    // and reflow wrapped scrollback out from under the user).
    const [composeOverlay, setComposeOverlay] = useState(0);

    // ComposeBar is unmounted outside mobile compose mode, and its
    // ResizeObserver never reports 0 on the way out — so derive the shift
    // from whether the bar is actually mounted rather than trusting the last
    // height it reported. This also guards against a mid-draft viewport
    // change (foldable, tablet rotate, split-screen) crossing the mobile
    // breakpoint and leaving a stale nonzero composeOverlay with nothing
    // mounted to justify it.
    const composeBarVisible = isMobile && !selectMode;
    const terminalShift = composeBarVisible ? composeOverlay : 0;

    // composeOverlay itself must also be reset when the bar unmounts: without
    // this, toggling select mode off can apply a stale nonzero overlay in the
    // same commit as the remount (terminalShift briefly reads that stale
    // value before the fresh observer reports 0), producing a visible slide
    // as it animates back down. terminalShift above remains the source of
    // truth for the actual shift; this is just about not carrying a stale
    // value across a remount.
    useEffect(() => {
      if (!composeBarVisible) setComposeOverlay(0);
    }, [composeBarVisible]);

    // Sending while scrolled up must bring the user back to the live output —
    // xterm deliberately holds its scroll position when new output arrives, so
    // the reply would otherwise land off-screen and the terminal would look
    // frozen. (Attached tmux sessions are additionally snapped back server-side
    // by the copy-mode cancel; but hooks/touch-scroll.ts scrolls xterm's local
    // scrollback directly without ever reaching tmux, so this client-side
    // scroll is the primary mechanism for attached sessions too, not merely a
    // raw-shell fallback.)
    const handleSend = useCallback(
      (text: string) => {
        sendText(text);
        xtermRef.current?.scrollToBottom();
      },
      [sendText, xtermRef]
    );

    return (
      <div
        ref={containerRef}
        className="bg-background flex flex-col overflow-hidden"
        style={{
          position: "relative",
          width: "100%",
          height: "100%",
        }}
        {...dragHandlers}
      >
        {/* Drag-and-drop overlay */}
        {isDragging && (
          <div className="bg-background/80 pointer-events-none absolute inset-0 z-40 flex flex-col items-center justify-center gap-2 border-2 border-dashed border-primary/50 backdrop-blur-sm">
            <Paperclip className="text-primary h-8 w-8" />
            <span className="text-sm font-medium">Drop files to upload</span>
          </div>
        )}

        {/* Search Bar */}
        <SearchBar
          ref={searchInputRef}
          visible={searchVisible}
          query={searchQuery}
          onQueryChange={setSearchQuery}
          onFindNext={findNext}
          onFindPrevious={findPrevious}
          onClose={closeSearch}
        />

        {/* Terminal container - NO padding! FitAddon reads offsetHeight which includes padding */}
        <div
          ref={terminalRef}
          className={cn(
            "terminal-container min-h-0 w-full flex-1 overflow-hidden",
            selectMode && "ring-primary ring-2 ring-inset"
          )}
          style={{
            transform: terminalShift ? `translateY(-${terminalShift}px)` : undefined,
            transition: "transform 150ms ease-out",
          }}
          onClick={handleContainerClick}
          onTouchStart={selectMode ? (e) => e.stopPropagation() : undefined}
          onTouchEnd={selectMode ? (e) => e.stopPropagation() : undefined}
        />

        {/* Select mode overlay - shows terminal text in a selectable format */}
        {selectMode && (
          <div
            className="bg-background absolute inset-0 z-40 overflow-auto"
            onTouchStart={(e) => e.stopPropagation()}
            onTouchEnd={(e) => e.stopPropagation()}
          >
            <pre
              className="p-3 font-mono text-xs break-all whitespace-pre-wrap select-text"
              style={{
                userSelect: "text",
                WebkitUserSelect: "text",
              }}
            >
              {terminalText}
            </pre>
          </div>
        )}

        {/* Mobile: persistent compose input plus the special-keys toolbar.
            Unlike the old mount-on-demand modal, the compose input stays
            mounted across focus/blur so ^C / esc are always one tap away. */}
        {composeBarVisible && (
          <>
            <ComposeBar
              onSend={handleSend}
              connected={isConnected}
              workingDirectory={workingDirectory}
              onOverlayHeightChange={setComposeOverlay}
            />
            <TerminalToolbar onKeyPress={sendInput} />
          </>
        )}

        {/* Connection status overlays */}
        {connectionState === "connecting" && (
          <div className="bg-background absolute inset-0 z-20 flex flex-col items-center justify-center gap-3">
            <div className="bg-primary h-2 w-2 animate-pulse rounded-full" />
            <span className="text-muted-foreground text-sm">Connecting...</span>
          </div>
        )}

        {connectionState === "reconnecting" && (
          <div className="absolute top-4 right-4 flex items-center gap-2 rounded bg-amber-500/20 px-2 py-1 text-xs text-amber-400">
            <div className="h-2 w-2 animate-pulse rounded-full bg-amber-500" />
            Reconnecting...
          </div>
        )}

        {/* Session ended overlay - no reconnect button */}
        {connectionState === "session_ended" && (
          <div className="bg-background/80 absolute inset-0 z-30 flex flex-col items-center justify-center gap-3 backdrop-blur-sm">
            <span className="text-muted-foreground text-sm font-medium">
              Session ended
            </span>
          </div>
        )}

        {/* Disconnected overlay - shows tap to reconnect button */}
        {connectionState === "disconnected" && (
          <button
            onClick={reconnect}
            className="bg-background/80 active:bg-background/90 absolute inset-0 z-30 flex flex-col items-center justify-center gap-3 backdrop-blur-sm transition-all"
          >
            <WifiOff className="text-muted-foreground h-8 w-8" />
            <span className="text-foreground text-sm font-medium">
              Connection lost
            </span>
            <span className="bg-primary text-primary-foreground rounded-full px-4 py-2 text-sm font-medium">
              Tap to reconnect
            </span>
          </button>
        )}
      </div>
    );
  }
);
