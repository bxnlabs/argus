import type { RefObject } from "react";
import type { Terminal as XTerm } from "@xterm/xterm";
import type { SearchAddon } from "@xterm/addon-search";

export interface TerminalScrollState {
  scrollTop: number;
  cursorY: number;
  baseY: number;
}

export interface UseTerminalConnectionProps {
  terminalRef: RefObject<HTMLDivElement | null>;
  /** Session ID to connect to. When null, terminal shows placeholder. */
  sessionName: string | null;
  onConnected?: () => void;
  onDisconnected?: () => void;
  onBeforeUnmount?: (scrollState: TerminalScrollState) => void;
  initialScrollState?: TerminalScrollState;
  isMobile?: boolean;
  selectMode?: boolean;
}

export interface UseTerminalConnectionReturn {
  connectionState: "connecting" | "connected" | "disconnected" | "reconnecting" | "session_ended";
  xtermRef: RefObject<XTerm | null>;
  searchAddonRef: RefObject<SearchAddon | null>;

  sendInput: (data: string) => void;
  sendText: (text: string) => void;
  focus: () => void;
  getScrollState: () => TerminalScrollState | null;
  restoreScrollState: (state: TerminalScrollState) => void;
  reconnect: () => void;
}
