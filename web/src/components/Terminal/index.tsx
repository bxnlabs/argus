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
  sessionId: string | null;
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
  /** Server-derived session slug, shown in the compose placeholder */
  sessionSlug?: string | null;
}

export const Terminal = forwardRef<TerminalHandle, TerminalProps>(
  function Terminal(
    { sessionId, onConnected, onDisconnected, onBeforeUnmount, initialScrollState, selectMode = false, onFilesDropped, workingDirectory, sessionSlug },
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
      sessionId,
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

    // How far the compose panel overflows its one-line spacer. The terminal is
    // shifted by this much with a transform rather than resized: transforms
    // don't affect layout, so FitAddon never refits and the PTY never sees a
    // SIGWINCH mid-typing (which would repaint the agent's TUI and reflow
    // wrapped scrollback out from under the user).
    const [composeOverlay, setComposeOverlay] = useState(0);
    // The card's edge spans two sibling components, so its focus state lives
    // here rather than inside either half.
    const [composeFocused, setComposeFocused] = useState(false);

    // Select mode only hides ComposeBar, but crossing the mobile breakpoint
    // mid-draft unmounts it — and an unmounting observer never reports 0. Gate
    // the shift on mount state, and clear the height too so a stale one can't
    // be applied for a frame on the way back in.
    const composeBarVisible = isMobile && !selectMode;
    const terminalShift = composeBarVisible ? composeOverlay : 0;

    useEffect(() => {
      if (!composeBarVisible) {
        setComposeOverlay(0);
        // ComposeBar stays mounted under display:none rather than
        // unmounting, so its own internal `focused` state is not reset by
        // this effect — only this parent copy is. In practice a browser
        // blurs a focused element when an ancestor becomes display:none, so
        // ComposeBar's onBlur fires and the two copies converge anyway. This
        // reset exists as a belt-and-braces guarantee for the case that blur
        // is ever not delivered (e.g. a future change swaps display:none for
        // something that doesn't trigger it): without it, the toolbar half
        // could be left bright with no compose bar visible to un-focus.
        setComposeFocused(false);
      }
    }, [composeBarVisible]);

    // Sending while scrolled up must bring the user back to the live output —
    // xterm deliberately holds its scroll position when new output arrives, so
    // the reply would otherwise land off-screen and the terminal would look
    // frozen. (Attached tmux sessions are additionally snapped back server-side
    // by the copy-mode cancel; but hooks/touch-scroll.ts scrolls xterm's local
    // scrollback directly without ever reaching tmux, so this client-side
    // scroll is the primary mechanism for attached sessions too, not merely a
    // raw-shell fallback.)
    // Reports back whether the text actually went out, so ComposeBar keeps the
    // draft when the socket closed between its last `connected` render and the
    // tap — the window where the send button is still enabled but the write
    // would go nowhere.
    const handleSend = useCallback(
      (text: string) => {
        if (!sendText(text)) return false;
        xtermRef.current?.scrollToBottom();
        return true;
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
            // FitAddon fits whole rows, so any height that isn't an exact
            // multiple of the cell height leaves a sub-row remainder. Bottom-
            // align the terminal so that remainder falls at the TOP, under the
            // header, instead of as a dead band between the last row and the
            // compose bar — which the compose bar's solid surface turns into a
            // visible misaligned stripe.
            "terminal-container flex min-h-0 w-full flex-1 flex-col justify-end overflow-hidden",
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
            Unlike the old mount-on-demand modal, both bars stay mounted across
            focus/blur, so ^C / esc are one tap away while watching output
            without raising the keyboard.

            Select mode hides both bars (per spec), but only by hiding this
            wrapper (display:none via the "hidden" class) — it never unmounts
            ComposeBar. That is what keeps an in-progress draft alive when the
            user taps into select mode to copy something out of the terminal
            and back out again. */}
        {isMobile && (
          <div className={composeBarVisible ? "contents" : "hidden"}>
            <ComposeBar
              onSend={handleSend}
              connected={isConnected}
              workingDirectory={workingDirectory}
              sessionSlug={sessionSlug}
              onOverlayHeightChange={setComposeOverlay}
              onFocusedChange={setComposeFocused}
            />
            <TerminalToolbar onKeyPress={sendInput} focused={composeFocused} />
          </div>
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
