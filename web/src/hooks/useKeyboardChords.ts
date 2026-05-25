import { useEffect, useMemo, useRef, useState } from "react";
import { isMac } from "@/lib/device";

/**
 * A single selectable chord step.
 *
 * The leader-key model: a browser tab can't claim single-chord shortcuts
 * (Cmd+T/N/W etc. are reserved), so we arm a "command pending" state on a
 * leader combo and let the next unmodified key pick an action. Bindings form a
 * shallow tree: a key may fire a `run`, descend into `children`, or both.
 */
export type ChordBinding = {
  /** Human-readable label for the hint overlay. */
  label: string;
  /** Action fired when this key is selected. */
  run?: () => void;
  /** Optional sub-chords reachable after this key. */
  children?: Record<string, ChordBinding>;
};

export type ChordMap = Record<string, ChordBinding>;

/**
 * The bindings selectable right now, plus the breadcrumb of keys taken to get
 * there. Exposed so a hint overlay can render the current options. `null` when
 * idle.
 */
export type ChordPending = {
  /** Bindings selectable at this moment. */
  level: ChordMap;
  /** Keys taken to reach `level` ([] at the prefix, e.g. ["g"] in a sub-chord). */
  path: string[];
};

/** A pending chord auto-cancels after this much inactivity. */
const CHORD_TIMEOUT_MS = 1200;

/** The leader key (combined with Cmd on macOS / Ctrl elsewhere). */
const LEADER_KEY = ";";

/**
 * Keys that only ever appear as the user holding/releasing a modifier. While
 * pending we ignore these outright — the user may still be holding Cmd from the
 * leader, and a lone Shift/Ctrl press must neither cancel nor match.
 */
const BARE_MODIFIER_KEYS = new Set([
  "Shift",
  "Control",
  "Alt",
  "Meta",
  "CapsLock",
  "Fn",
  "FnLock",
  "Hyper",
  "Super",
  "AltGraph",
  "NumLock",
  "ScrollLock",
]);

/** Normalize a `KeyboardEvent.key` for matching against binding keys. */
function normalizeKey(key: string): string {
  // Single alphabetic chars are lowercased so Shift/CapsLock don't break letter
  // chords. Symbols ("=", "-", "?") and named keys ("ArrowLeft") stay verbatim.
  if (key.length === 1 && /[a-zA-Z]/.test(key)) return key.toLowerCase();
  return key;
}

/**
 * Walk `bindings` along `path` to find the level selectable at that position.
 * Returns `null` if the path no longer resolves against the latest bindings
 * (e.g. the caller's `useMemo` rebuilt them and a branch disappeared).
 */
function resolveLevel(bindings: ChordMap, path: string[]): ChordMap | null {
  let level: ChordMap = bindings;
  for (const key of path) {
    const next = level[key]?.children;
    if (!next) return null;
    level = next;
  }
  return level;
}

/**
 * Leader-key chord engine. Action-agnostic: it knows nothing about which
 * actions exist, only the `ChordMap` it's handed. Listens on `document` in the
 * capture phase so it intercepts keys before a focused terminal/editor sees
 * them — mirroring `Terminal/hooks/terminal-init.ts`.
 *
 * @param bindings  Chord tree. The caller rebuilds this (via `useMemo`) as app
 *   state changes; the latest version is always read through a ref so `run`
 *   callbacks never go stale and the listener never re-subscribes.
 * @param options.enabled  When `false` the hook attaches no listeners and is
 *   completely inert (used to disable chords on touch devices). Default `true`.
 * @returns `{ pending }` — the current selectable level + breadcrumb, or `null`
 *   when idle.
 */
export function useKeyboardChords(
  bindings: ChordMap,
  options?: { enabled?: boolean },
): { pending: ChordPending | null } {
  const enabled = options?.enabled ?? true;

  // Latest bindings, read by the handler so the document listener doesn't have
  // to re-subscribe (and so `run` is never stale).
  const bindingsRef = useRef(bindings);
  bindingsRef.current = bindings;

  // The pending position is tracked as a path: `null` (idle), `[]` (prefix), or
  // e.g. `["g"]` (sub). `pending.level` is derived from the LATEST bindings, so
  // the overlay and fired `run` both reflect current state.
  const [path, setPath] = useState<string[] | null>(null);
  const pathRef = useRef<string[] | null>(path);
  pathRef.current = path;

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!enabled) return;

    const clearTimer = () => {
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };

    const cancel = () => {
      clearTimer();
      pathRef.current = null;
      setPath(null);
    };

    // Move to a new pending position and (re)start the inactivity timer.
    const transition = (next: string[]) => {
      clearTimer();
      pathRef.current = next;
      setPath(next);
      timerRef.current = setTimeout(cancel, CHORD_TIMEOUT_MS);
    };

    const handleKeyDown = (event: KeyboardEvent) => {
      // Ignore OS key auto-repeat entirely: a held leader would otherwise land in
      // the pending branch, fail to match ";", and silently disarm the chord.
      if (event.repeat) return;

      // During IME composition browsers emit keydown with isComposing=true; while
      // pending such a key would be captured/prevented and cancel the chord.
      if (event.isComposing) return;

      const current = pathRef.current;

      // --- idle: react only to the leader combo, pass everything else through.
      if (current === null) {
        const leaderHeld = isMac() ? event.metaKey : event.ctrlKey;
        if (leaderHeld && event.key === LEADER_KEY) {
          event.preventDefault();
          event.stopImmediatePropagation();
          transition([]);
        }
        return;
      }

      // --- pending (prefix or sub).
      // Bare modifier presses are ignored: the user may still be holding Cmd
      // from the leader. Stay pending, don't touch the event.
      if (BARE_MODIFIER_KEYS.has(event.key)) return;

      // Any real follow-up is captured airtight so it can't leak to the
      // PTY/editor/input — regardless of whether it ends up matching.
      // stopImmediatePropagation (not just stopPropagation) ensures sibling
      // capture-phase listeners on the same target are also suppressed.
      event.preventDefault();
      event.stopImmediatePropagation();

      if (event.key === "Escape") {
        cancel();
        return;
      }

      // Resolve the current level against the LATEST bindings. The follow-up is
      // matched on `event.key` alone (modifier state ignored).
      const level = resolveLevel(bindingsRef.current, current);
      if (!level) {
        cancel();
        return;
      }

      const binding = level[normalizeKey(event.key)];
      if (!binding) {
        // Unmapped follow-up: cancel silently, don't act.
        cancel();
        return;
      }

      const next = [...current, normalizeKey(event.key)];

      if (binding.children) {
        // Descend (firing `run` first if present), staying pending.
        binding.run?.();
        transition(next);
      } else {
        // Leaf: fire and return to idle.
        binding.run?.();
        cancel();
      }
    };

    document.addEventListener("keydown", handleKeyDown, true);
    window.addEventListener("blur", cancel);

    return () => {
      document.removeEventListener("keydown", handleKeyDown, true);
      window.removeEventListener("blur", cancel);
      clearTimer();
    };
  }, [enabled]);

  const pending = useMemo<ChordPending | null>(() => {
    if (path === null) return null;
    const level = resolveLevel(bindings, path);
    if (!level) return null;
    return { level, path };
  }, [path, bindings]);

  return { pending };
}
