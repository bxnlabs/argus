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
  // Consecutive context losses where the renderer never survived long enough to
  // prove healthy. An unhealthy GPU loses every fresh context almost at once; a
  // normal background eviction loses one context that recovers on foreground.
  let webglLosses = 0;
  let webglStableTimer: ReturnType<typeof setTimeout> | null = null;
  // A context that survives this long is treated as healthy and clears the loss
  // streak.
  const WEBGL_STABLE_MS = 10000;
  // xterm fires onContextLoss only ~3s after the real loss — it waits for a
  // possible 'webglcontextrestored' first (see WebglRenderer). The health timer
  // must add this delay; otherwise a context that dies late in the window is
  // reported only after the reset already cleared the streak, letting an
  // unhealthy context loop forever.
  const WEBGL_LOSS_REPORT_DELAY_MS = 3000;
  const WEBGL_MAX_LOSSES = 2; // give up after this many rapid losses

  const loadWebgl = () => {
    if (webglGivenUp) return;
    let pendingAddon: WebglAddon | null = null;
    try {
      const addon = new WebglAddon();
      pendingAddon = addon;
      addon.onContextLoss(() => {
        addon.dispose(); // xterm reverts to its DOM renderer
        webglAddon = null;
        webglDropped = true;
        term.refresh(0, term.rows - 1); // repaint current buffer via DOM renderer
        // The context died before proving healthy — cancel its pending stability
        // reset so this loss counts toward the streak.
        if (webglStableTimer) {
          clearTimeout(webglStableTimer);
          webglStableTimer = null;
        }
        // Repeated rapid losses mean the context is unhealthy (e.g. GPU reset) —
        // stop retrying to avoid a dispose/recreate loop.
        if (++webglLosses >= WEBGL_MAX_LOSSES) {
          webglGivenUp = true;
        } else if (document.visibilityState === "visible") {
          // Loss surfaced while foregrounded — re-acquire immediately.
          loadWebgl();
        }
      });
      term.loadAddon(addon);
      webglAddon = addon;
      webglDropped = false;
      // If this renderer survives the window (plus xterm's loss-reporting delay,
      // so a late loss can still cancel this), treat it as healthy and clear the
      // loss streak.
      if (webglStableTimer) clearTimeout(webglStableTimer);
      webglStableTimer = setTimeout(() => {
        webglLosses = 0;
        webglStableTimer = null;
      }, WEBGL_STABLE_MS + WEBGL_LOSS_REPORT_DELAY_MS);
    } catch {
      // WebGL2 unavailable, or activation threw after xterm registered the addon
      // — dispose it and fall back to xterm's default DOM renderer.
      pendingAddon?.dispose();
      webglAddon = null;
      // A throw while re-acquiring after a context loss is another unhealthy
      // attempt — count it toward the streak so we eventually stop retrying on
      // every foreground instead of looping forever. (The initial load runs
      // with webglDropped === false, so a one-off "no WebGL2" startup failure
      // never trips this.)
      if (webglDropped && ++webglLosses >= WEBGL_MAX_LOSSES) {
        webglGivenUp = true;
      }
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
    if (webglStableTimer) clearTimeout(webglStableTimer);
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
