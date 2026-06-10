import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { WebglAddon } from "@xterm/addon-webgl";
import { TERMINAL_THEME } from "../constants";

const FONT_SIZE_MOBILE = 13;
const FONT_SIZE_DESKTOP = 12;
const LINE_HEIGHT = 1.2;

export interface TerminalInstance {
  term: XTerm;
  fitAddon: FitAddon;
  searchAddon: SearchAddon;
  cleanup: () => void;
}

export function createTerminal(
  container: HTMLElement,
  isMobile: boolean
): TerminalInstance {
  const fontSize = isMobile ? FONT_SIZE_MOBILE : FONT_SIZE_DESKTOP;

  const term = new XTerm({
    cursorBlink: true,
    fontSize,
    fontFamily:
      '"JetBrains Mono", "Fira Code", Menlo, Monaco, "Courier New", monospace',
    fontWeight: "400",
    fontWeightBold: "600",
    letterSpacing: 0,
    lineHeight: LINE_HEIGHT,
    scrollback: 15000,
    scrollSensitivity: isMobile ? 3 : 1,
    fastScrollSensitivity: 5,
    smoothScrollDuration: 100,
    cursorStyle: "bar",
    cursorWidth: 2,
    allowProposedApi: true,
    altClickMovesCursor: false,
    macOptionClickForcesSelection: true,
    minimumContrastRatio: 4.5,
    theme: TERMINAL_THEME,
  });

  const fitAddon = new FitAddon();
  const searchAddon = new SearchAddon();

  term.loadAddon(fitAddon);
  term.loadAddon(new WebLinksAddon());
  term.loadAddon(searchAddon);
  term.open(container);

  // --- WebGL renderer with context-loss recovery ---
  // Browsers drop a tab's GPU/WebGL context when it's backgrounded for a while
  // to reclaim memory. xterm's WebGL renderer can't repaint a lost context, so
  // the canvas goes blank white (the bug this guards against). On loss we
  // dispose the addon — xterm falls back to its DOM renderer so the terminal
  // stays visible — and re-acquire WebGL when the tab is foregrounded again,
  // restoring GPU acceleration.
  let webglAddon: WebglAddon | null = null;
  let webglDropped = false; // had WebGL, then lost the context
  let webglGivenUp = false; // repeated rapid losses — stop retrying
  let lastWebglLoadAt = 0;

  const loadWebgl = () => {
    if (webglGivenUp) return;
    try {
      const addon = new WebglAddon();
      addon.onContextLoss(() => {
        addon.dispose(); // xterm reverts to its DOM renderer
        webglAddon = null;
        webglDropped = true;
        term.refresh(0, term.rows - 1); // repaint current buffer via DOM renderer
        // A loss within a second of (re)loading means the context is unhealthy
        // (e.g. GPU reset) — stop retrying to avoid a dispose/recreate loop.
        if (Date.now() - lastWebglLoadAt < 1000) {
          webglGivenUp = true;
        } else if (document.visibilityState === "visible") {
          // Loss surfaced while foregrounded — re-acquire immediately.
          loadWebgl();
        }
      });
      term.loadAddon(addon);
      webglAddon = addon;
      webglDropped = false;
      lastWebglLoadAt = Date.now();
    } catch {
      // WebGL2 unavailable — fall back to xterm's default DOM renderer.
      webglAddon = null;
    }
  };

  loadWebgl();
  fitAddon.fit();

  // Re-acquire WebGL once the tab is visible again and the GPU context is back.
  const handleWebglVisibility = () => {
    if (
      document.visibilityState === "visible" &&
      webglDropped &&
      !webglAddon &&
      !webglGivenUp
    ) {
      loadWebgl();
      if (webglAddon) term.refresh(0, term.rows - 1);
    }
  };
  document.addEventListener("visibilitychange", handleWebglVisibility);

  // Helper to copy text to clipboard with fallback
  const copyToClipboard = (text: string) => {
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).catch(() => {
        execCommandCopy(text);
      });
    } else {
      execCommandCopy(text);
    }
  };

  const execCommandCopy = (text: string) => {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.style.position = "fixed";
    textarea.style.opacity = "0";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    document.body.removeChild(textarea);
  };

  // Handle Cmd+A and Cmd+C via document event listener
  const handleKeyDown = (event: KeyboardEvent) => {
    // Only handle when terminal is focused
    if (!container.contains(document.activeElement)) return;

    const key = event.key.toLowerCase();

    // Cmd+A (macOS) for select all — Ctrl+A passes through for readline/tmux
    if (event.metaKey && !event.ctrlKey && key === "a") {
      event.preventDefault();
      event.stopPropagation();
      term.selectAll();
      return;
    }

    // Cmd+C (macOS) / Ctrl+C for copy when text is selected
    if ((event.metaKey || event.ctrlKey) && key === "c") {
      const selection = term.getSelection();
      if (selection) {
        event.preventDefault();
        event.stopPropagation();
        copyToClipboard(selection);
      }
    }
  };

  // Use capture phase to intercept before browser default
  document.addEventListener("keydown", handleKeyDown, true);

  const cleanup = () => {
    document.removeEventListener("keydown", handleKeyDown, true);
    document.removeEventListener("visibilitychange", handleWebglVisibility);
  };

  return { term, fitAddon, searchAddon, cleanup };
}

export function updateTerminalForMobile(
  term: XTerm,
  fitAddon: FitAddon,
  isMobile: boolean,
  sendResize: (cols: number, rows: number) => void
): void {
  const newFontSize = isMobile ? FONT_SIZE_MOBILE : FONT_SIZE_DESKTOP;
  const newLineHeight = LINE_HEIGHT;

  if (term.options.fontSize !== newFontSize) {
    term.options.fontSize = newFontSize;
    term.options.lineHeight = newLineHeight;
    term.refresh(0, term.rows - 1);
    fitAddon.fit();
    sendResize(term.cols, term.rows);
  }
}
