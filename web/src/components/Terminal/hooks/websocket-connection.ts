import type { Terminal as XTerm } from "@xterm/xterm";
import { WS_RECONNECT_BASE_DELAY, WS_RECONNECT_MAX_DELAY } from "../constants";
import { nodeWsUrl } from "@/api/client";

export interface WebSocketCallbacks {
  onConnected?: () => void;
  onDisconnected?: () => void;
  onConnectionStateChange: (
    state: "connecting" | "connected" | "disconnected" | "reconnecting" | "session_ended"
  ) => void;
}

export interface WebSocketManager {
  ws: WebSocket;
  sendInput: (data: string) => void;
  sendText: (text: string) => void;
  sendResize: (cols: number, rows: number) => void;
  reconnect: () => void;
  cleanup: () => void;
}

const textEncoder = new TextEncoder();

export function createWebSocketConnection(
  term: XTerm,
  baseUrl: string,
  sessionId: string | null,
  callbacks: WebSocketCallbacks,
  wsRef: React.MutableRefObject<WebSocket | null>,
  reconnectTimeoutRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>,
  reconnectDelayRef: React.MutableRefObject<number>,
  intentionalCloseRef: React.MutableRefObject<boolean>
): WebSocketManager {
  // Captured once at setup so this socket — and its reconnects — always target
  // the node that owned it, even if the active node changes elsewhere.
  const wsUrl = nodeWsUrl(baseUrl, sessionId);
  const ws = new WebSocket(wsUrl);
  ws.binaryType = "arraybuffer";
  wsRef.current = ws;

  const sendResize = (cols: number, rows: number) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({ version: 1, type: "resize", cols, rows })
      );
    }
  };

  const sendInput = (data: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(textEncoder.encode(data));
    }
  };

  const sendText = (text: string) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({ version: 1, type: "text", text, submit: true })
      );
    }
  };

  // Force reconnect - kills any existing connection and creates fresh one
  // Note: savedHandlers is populated after handlers are defined below
  let savedHandlers: {
    onopen: typeof ws.onopen;
    onmessage: typeof ws.onmessage;
    onclose: typeof ws.onclose;
    onerror: typeof ws.onerror;
  };

  const forceReconnect = () => {
    if (intentionalCloseRef.current) return;

    // Clear any pending reconnect
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }

    // Force close existing socket regardless of state (handles hung sockets)
    const oldWs = wsRef.current;
    if (oldWs) {
      // Remove handlers to prevent callbacks
      oldWs.onopen = null;
      oldWs.onmessage = null;
      oldWs.onclose = null;
      oldWs.onerror = null;
      try {
        oldWs.close();
      } catch {
        /* ignore */
      }
      wsRef.current = null;
    }

    callbacks.onConnectionStateChange("reconnecting");
    reconnectDelayRef.current = WS_RECONNECT_BASE_DELAY;

    // Create fresh connection with saved handlers
    const newWs = new WebSocket(wsUrl);
    newWs.binaryType = "arraybuffer";
    wsRef.current = newWs;
    newWs.onopen = savedHandlers.onopen;
    newWs.onmessage = savedHandlers.onmessage;
    newWs.onclose = savedHandlers.onclose;
    newWs.onerror = savedHandlers.onerror;
  };

  // Soft reconnect - only if not already connected
  const attemptReconnect = () => {
    if (intentionalCloseRef.current) return;
    if (wsRef.current?.readyState === WebSocket.OPEN) return;
    forceReconnect();
  };

  ws.onopen = () => {
    callbacks.onConnectionStateChange("connected");
    reconnectDelayRef.current = WS_RECONNECT_BASE_DELAY;
    // Send hello with initial terminal size
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(
        JSON.stringify({
          version: 1,
          type: "hello",
          cols: term.cols,
          rows: term.rows,
        })
      );
    }
    callbacks.onConnected?.();
    term.focus();
  };

  ws.onmessage = (event) => {
    if (event.data instanceof ArrayBuffer) {
      // Binary frame = PTY output — write raw bytes to xterm
      const data = new Uint8Array(event.data);
      term.write(data);
    } else if (typeof event.data === "string") {
      // Text frame = JSON control message
      try {
        const msg = JSON.parse(event.data);
        if (msg.type === "exit") {
          callbacks.onConnectionStateChange("session_ended");
          term.write("\r\n\x1b[33m[Session ended]\x1b[0m\r\n");
        } else if (msg.type === "error") {
          term.write(
            `\r\n\x1b[31m[Error: ${msg.message || "unknown"}]\x1b[0m\r\n`
          );
        }
        // "connected" message is informational, no action needed
      } catch {
        // Ignore malformed text frames
      }
    }
  };

  ws.onclose = () => {
    callbacks.onDisconnected?.();

    if (intentionalCloseRef.current) {
      callbacks.onConnectionStateChange("disconnected");
      return;
    }

    callbacks.onConnectionStateChange("reconnecting");

    const currentDelay = reconnectDelayRef.current;
    reconnectDelayRef.current = Math.min(
      currentDelay * 2,
      WS_RECONNECT_MAX_DELAY
    );
    reconnectTimeoutRef.current = setTimeout(attemptReconnect, currentDelay);
  };

  ws.onerror = () => {
    // Errors are handled by onclose
  };

  // Save handlers now that they're defined (for reconnection)
  savedHandlers = {
    onopen: ws.onopen,
    onmessage: ws.onmessage,
    onclose: ws.onclose,
    onerror: ws.onerror,
  };

  // Handle terminal input — send as binary frames
  term.onData((data) => {
    sendInput(data);
  });

  // Handle Shift+Enter — send literal newline for multi-line input.
  // preventDefault() stops the browser from also inserting into xterm's
  // hidden textarea, which would fire a second onData and double the newline.
  term.attachCustomKeyEventHandler((event) => {
    if (event.type === "keydown" && event.key === "Enter" && event.shiftKey) {
      event.preventDefault();
      sendInput("\n");
      return false;
    }
    return true;
  });

  // Track when page was last hidden (for detecting long sleeps)
  let hiddenAt: number | null = null;

  // Handle visibility change for reconnection
  const handleVisibilityChange = () => {
    if (intentionalCloseRef.current) return;

    if (document.visibilityState === "hidden") {
      hiddenAt = Date.now();
      return;
    }

    // Page became visible
    if (document.visibilityState !== "visible") return;

    const wasHiddenFor = hiddenAt ? Date.now() - hiddenAt : 0;
    hiddenAt = null;

    // If hidden for more than 5 seconds, force reconnect (iOS Safari kills sockets)
    // This handles the "hung socket" problem where readyState says OPEN but it's dead
    if (wasHiddenFor > 5000) {
      forceReconnect();
      return;
    }

    // Otherwise only reconnect if actually disconnected
    const currentWs = wsRef.current;
    const isDisconnected =
      !currentWs ||
      currentWs.readyState === WebSocket.CLOSED ||
      currentWs.readyState === WebSocket.CLOSING;
    const isStaleConnection = currentWs?.readyState === WebSocket.CONNECTING;

    if (isDisconnected || isStaleConnection) {
      forceReconnect();
    }
  };
  document.addEventListener("visibilitychange", handleVisibilityChange);

  const cleanup = () => {
    document.removeEventListener("visibilitychange", handleVisibilityChange);
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    // Detach handlers before closing to prevent stale onclose callbacks.
    // When sessionId changes, React runs cleanup then setup synchronously,
    // but WebSocket onclose fires async. By that time, the new setup has
    // reset intentionalCloseRef to false, so the stale onclose would
    // incorrectly schedule reconnection against the new connection.
    ws.onopen = null;
    ws.onmessage = null;
    ws.onclose = null;
    ws.onerror = null;
    const currentWs = wsRef.current;
    if (currentWs && currentWs !== ws) {
      // Also detach handlers on any socket created by forceReconnect
      currentWs.onopen = null;
      currentWs.onmessage = null;
      currentWs.onclose = null;
      currentWs.onerror = null;
    }
    if (
      currentWs &&
      (currentWs.readyState === WebSocket.OPEN ||
        currentWs.readyState === WebSocket.CONNECTING)
    ) {
      currentWs.close(1000, "Component unmounting");
    }
  };

  return {
    ws,
    sendInput,
    sendText,
    sendResize,
    reconnect: forceReconnect,
    cleanup,
  };
}
