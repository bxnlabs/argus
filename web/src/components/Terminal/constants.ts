/**
 * Terminal constants and hardcoded dark theme configuration.
 * No theme switching -- single dark theme only.
 */

import type { ITheme } from "@xterm/xterm";

// Reconnection constants
export const WS_RECONNECT_BASE_DELAY = 1000; // 1 second
export const WS_RECONNECT_MAX_DELAY = 30000; // 30 seconds

// Hardcoded dark terminal theme
export const TERMINAL_THEME: ITheme = {
  background: "#0a0a0a",
  foreground: "#e4e4e7",
  cursor: "#e4e4e7",
  cursorAccent: "#0a0a0a",
  selectionBackground: "#3f3f46",
  selectionForeground: "#fafafa",
  selectionInactiveBackground: "#27272a",
  black: "#18181b",
  red: "#ef4444",
  green: "#22c55e",
  yellow: "#eab308",
  blue: "#3b82f6",
  magenta: "#a855f7",
  cyan: "#06b6d4",
  white: "#e4e4e7",
  brightBlack: "#52525b",
  brightRed: "#f87171",
  brightGreen: "#4ade80",
  brightYellow: "#facc15",
  brightBlue: "#60a5fa",
  brightMagenta: "#c084fc",
  brightCyan: "#22d3ee",
  brightWhite: "#fafafa",
};
