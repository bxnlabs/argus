import { useEffect, useRef, useState, useCallback } from "react";
import type { Terminal as XTerm } from "@xterm/xterm";
import type { FitAddon } from "@xterm/addon-fit";
import type { SearchAddon } from "@xterm/addon-search";
import { WS_RECONNECT_BASE_DELAY } from "../constants";

const textEncoder = new TextEncoder();
import type {
  TerminalScrollState,
  UseTerminalConnectionProps,
  UseTerminalConnectionReturn,
} from "./useTerminalConnection.types";
import { createTerminal, updateTerminalForMobile } from "./terminal-init";
import { setupTouchScroll } from "./touch-scroll";
import { createWebSocketConnection } from "./websocket-connection";
import { setupResizeHandlers } from "./resize-handlers";
import { useActiveNode } from "@/hooks/useActiveNode";

export type { TerminalScrollState } from "./useTerminalConnection.types";

export function useTerminalConnection({
  terminalRef,
  sessionName,
  onConnected,
  onDisconnected,
  onBeforeUnmount,
  initialScrollState,
  isMobile = false,
  selectMode = false,
}: UseTerminalConnectionProps): UseTerminalConnectionReturn {
  const { baseUrl } = useActiveNode();

  const [connectionState, setConnectionState] = useState<
    "connecting" | "connected" | "disconnected" | "reconnecting" | "session_ended"
  >("connecting");

  const wsRef = useRef<WebSocket | null>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const reconnectFnRef = useRef<(() => void) | null>(null);

  // Reconnection tracking
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelayRef = useRef<number>(WS_RECONNECT_BASE_DELAY);
  const intentionalCloseRef = useRef<boolean>(false);

  // Store callbacks and state in refs
  const callbacksRef = useRef({ onConnected, onDisconnected, onBeforeUnmount });
  callbacksRef.current = { onConnected, onDisconnected, onBeforeUnmount };
  const initialScrollStateRef = useRef(initialScrollState);
  const selectModeRef = useRef(selectMode);
  selectModeRef.current = selectMode;

  const sendInput = useCallback((data: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(textEncoder.encode(data));
    }
  }, []);

  // Returns whether the text actually reached the socket. Callers hold user
  // drafts: a send that silently no-ops on a closed socket must not read as
  // success, or the draft is cleared and the message is lost.
  const sendText = useCallback((text: string): boolean => {
    if (wsRef.current?.readyState !== WebSocket.OPEN) return false;
    wsRef.current.send(
      JSON.stringify({ version: 1, type: "text", text, submit: true })
    );
    return true;
  }, []);

  const focus = useCallback(() => xtermRef.current?.focus(), []);

  const getScrollState = useCallback((): TerminalScrollState | null => {
    if (!xtermRef.current || !terminalRef.current) return null;
    const buffer = xtermRef.current.buffer.active;
    const viewport = terminalRef.current.querySelector(
      ".xterm-viewport"
    ) as HTMLElement;
    return {
      scrollTop: viewport?.scrollTop ?? 0,
      cursorY: buffer.cursorY,
      baseY: buffer.baseY,
    };
  }, [terminalRef]);

  const restoreScrollState = useCallback(
    (state: TerminalScrollState) => {
      const viewport = terminalRef.current?.querySelector(
        ".xterm-viewport"
      ) as HTMLElement;
      if (viewport) {
        requestAnimationFrame(() => {
          viewport.scrollTop = state.scrollTop;
        });
      }
    },
    [terminalRef]
  );

  const reconnect = useCallback(() => {
    reconnectFnRef.current?.();
  }, []);

  // Main setup effect — depends on sessionName so it reconnects when session changes.
  // sessionName=null spawns a raw shell; sessionName="..." attaches to session by ID.
  useEffect(() => {
    if (!terminalRef.current) return;

    // Show "Connecting..." overlay immediately when session changes
    setConnectionState("connecting");

    // Reset intentional close flag (may be true from previous cleanup)
    intentionalCloseRef.current = false;
    let cleanupTouchScroll: (() => void) | null = null;
    let cleanupResizeHandlers: (() => void) | null = null;
    let cleanupWebSocket: (() => void) | null = null;
    let cleanupTerminal: (() => void) | null = null;

    // Initialize terminal (no theme parameter -- hardcoded dark)
    const { term, fitAddon, searchAddon, cleanup } = createTerminal(
      terminalRef.current,
      isMobile
    );
    xtermRef.current = term;
    fitAddonRef.current = fitAddon;
    searchAddonRef.current = searchAddon;
    cleanupTerminal = cleanup;

    // Setup touch scroll
    cleanupTouchScroll = setupTouchScroll({ term, selectModeRef, wsRef });

    // Setup WebSocket — pass sessionName for session-specific connection
    let scrollRestoreTimer: ReturnType<typeof setTimeout> | null = null;
    const wsManager = createWebSocketConnection(
      term,
      baseUrl,
      sessionName,
      {
        onConnected: () => {
          callbacksRef.current.onConnected?.();
          // Restore scroll state after connection
          if (initialScrollStateRef.current && terminalRef.current) {
            scrollRestoreTimer = setTimeout(() => {
              scrollRestoreTimer = null;
              const viewport = terminalRef.current?.querySelector(
                ".xterm-viewport"
              ) as HTMLElement;
              if (viewport)
                viewport.scrollTop = initialScrollStateRef.current!.scrollTop;
            }, 200);
          }
        },
        onDisconnected: () => callbacksRef.current.onDisconnected?.(),
        onConnectionStateChange: setConnectionState,
      },
      wsRef,
      reconnectTimeoutRef,
      reconnectDelayRef,
      intentionalCloseRef
    );
    cleanupWebSocket = wsManager.cleanup;
    reconnectFnRef.current = wsManager.reconnect;

    // Setup resize handlers
    cleanupResizeHandlers = setupResizeHandlers({
      term,
      fitAddon,
      containerRef: terminalRef,
      isMobile,
      sendResize: wsManager.sendResize,
    });

    return () => {
      intentionalCloseRef.current = true;
      if (scrollRestoreTimer) clearTimeout(scrollRestoreTimer);

      // Save scroll state before unmount
      const termInstance = xtermRef.current;
      if (termInstance && callbacksRef.current.onBeforeUnmount && terminalRef.current) {
        const buffer = termInstance.buffer.active;
        const viewport = terminalRef.current.querySelector(
          ".xterm-viewport"
        ) as HTMLElement;
        callbacksRef.current.onBeforeUnmount({
          scrollTop: viewport?.scrollTop ?? 0,
          cursorY: buffer.cursorY,
          baseY: buffer.baseY,
        });
      }

      // Cleanup in reverse order
      cleanupResizeHandlers?.();
      cleanupWebSocket?.();
      cleanupTouchScroll?.();
      cleanupTerminal?.();

      // Reset refs
      reconnectDelayRef.current = WS_RECONNECT_BASE_DELAY;

      if (wsRef.current) wsRef.current = null;
      if (xtermRef.current) {
        try {
          xtermRef.current.dispose();
        } catch {
          /* ignore */
        }
        xtermRef.current = null;
      }
      fitAddonRef.current = null;
      searchAddonRef.current = null;
    };
  }, [isMobile, terminalRef, sessionName, baseUrl]);

  // Handle isMobile changes dynamically
  useEffect(() => {
    const term = xtermRef.current;
    const fitAddon = fitAddonRef.current;
    if (!term || !fitAddon) return;

    updateTerminalForMobile(term, fitAddon, isMobile, (cols, rows) => {
      if (wsRef.current?.readyState === WebSocket.OPEN) {
        wsRef.current.send(
          JSON.stringify({ version: 1, type: "resize", cols, rows })
        );
      }
    });
  }, [isMobile]);

  return {
    connectionState,
    xtermRef,
    searchAddonRef,

    sendInput,
    sendText,
    focus,
    getScrollState,
    restoreScrollState,
    reconnect,
  };
}
