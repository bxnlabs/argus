import { memo, useCallback, useEffect, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { SendHorizontal, Paperclip } from "lucide-react";
import { cn } from "@/lib/utils";
import { FilePicker } from "@/components/FilePicker";
import { insertPaths } from "./insertPaths";

interface ComposeBarProps {
  /** Returns false when the text never reached the socket — keep the draft. */
  onSend: (text: string) => boolean;
  /** Send is disabled while the socket is down; the draft is kept. */
  connected: boolean;
  /** Session working directory, used to anchor file search. */
  workingDirectory?: string | null;
  /**
   * Reports how far the panel overflows its one-line spacer, so the terminal
   * can be shifted up by the same amount.
   */
  onOverlayHeightChange?: (height: number) => void;
}

export const ComposeBar = memo(function ComposeBar({
  onSend,
  connected,
  workingDirectory,
  onOverlayHeightChange,
}: ComposeBarProps) {
  const [text, setText] = useState("");
  const [focused, setFocused] = useState(false);
  const [showFilePicker, setShowFilePicker] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  // Panel height at one line, measured rather than hardcoded so it can't drift
  // from the spacer's h-11.
  const collapsedHeightRef = useRef<number | null>(null);
  // Read through a ref so an unstable callback identity can't rebuild the
  // observer and recapture the baseline at an already-grown height.
  const onOverlayHeightChangeRef = useRef(onOverlayHeightChange);
  onOverlayHeightChangeRef.current = onOverlayHeightChange;

  useEffect(() => {
    const el = panelRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;

    const ro = new ResizeObserver((entries) => {
      const height = entries[0]?.contentRect.height ?? 0;
      // Skip missing and zero observations rather than recording them: the
      // first delivery seeds the permanent baseline, and an inactive tab
      // observes 0 inside its display:none ancestor. A zero baseline would
      // make every later observation read as pure overflow.
      if (!height) return;
      collapsedHeightRef.current ??= height;
      onOverlayHeightChangeRef.current?.(
        Math.max(0, height - collapsedHeightRef.current),
      );
    });

    ro.observe(el);
    // Empty deps: build the observer once, so the baseline stays "the first
    // height observed after mount".
    return () => ro.disconnect();
  }, []);

  const canSend = connected && text.trim().length > 0;
  // An empty, unfocused bar stays quiet chrome — but a draft must remain
  // sendable after the user taps away to the terminal to hit a special key.
  const showSend = focused || text.length > 0;

  const handleSend = useCallback(() => {
    if (!canSend) return;
    // `connected` is a render-time snapshot, so the socket can close between
    // the last connected render and this tap. Clearing on a write that went
    // nowhere is exactly the silent draft loss this bar exists to prevent.
    if (!onSend(text)) return;
    setText("");
    // Keep the keyboard up for the next message.
    textareaRef.current?.focus();
  }, [canSend, onSend, text]);

  const handleFilesPicked = useCallback(
    (paths: string[]) => {
      const ta = textareaRef.current;
      const start = ta?.selectionStart ?? text.length;
      const end = ta?.selectionEnd ?? text.length;
      const result = insertPaths(text, start, end, paths);
      setText(result.text);
      requestAnimationFrame(() => {
        ta?.focus();
        ta?.setSelectionRange(result.cursor, result.cursor);
      });
    },
    [text],
  );

  return (
    <>
      {/* Spacer: holds the one-line height in flow. The panel below is
          absolutely positioned against its bottom edge and grows UPWARD, so
          growing the input never changes the terminal's laid-out height and
          never triggers a FitAddon refit.

          --compose-row-h is the single source of truth for one line of compose
          text, and BOTH the spacer's height and the three-line cap derive from
          it. That coupling is not decorative: this spacer's height must equal
          the panel's resting height, or the panel permanently overflows it, the
          observer reports a nonzero overlay at rest, and the terminal carries a
          constant shift — silently breaking the very invariant the transform
          exists to protect. Deriving both from one variable means they cannot
          drift; a hardcoded 44px and a hardcoded 1.25rem line-height can.

          Spacer height = one row (line-height + the mirror's own py-1.5)
                        + the panel's py-1.5
                        + the panel's 1px border-t
                        = var + 0.75rem + 0.75rem + 1px. */}
      <div
        // mt-2 is deliberate breathing room between the terminal's last row and
        // the input zone. It replaces the sub-row remainder that used to land
        // here by accident — that gap varied with the container height (11px at
        // one size, 17px at another), which is what made it read as inconsistent.
        // 8px also happens to fit inside the remainder that was being wasted, so
        // it usually costs no terminal row; 12px crosses a row boundary and does.
        // Margin, not padding: FitAddon measures the terminal container, and
        // padding would be counted as usable row space.
        className="relative mt-2 flex-shrink-0"
        style={
          {
            "--compose-row-h": "21px",
            height: "calc(var(--compose-row-h) + 1.5rem + 1px)",
          } as CSSProperties
        }
      >
        <div
          ref={panelRef}
          className={cn(
            // One surface shared with TerminalToolbar, and one zone edge at its
            // top — the toolbar's own border-t is transparent so the two rows
            // read as a single input zone rather than three stacked hairlines.
            "absolute inset-x-0 bottom-0 flex items-end gap-1.5 border-t bg-[hsl(0_0%_8%)] px-2 py-1.5 transition-colors",
            // Focus is carried by the placeholder swap and the send button
            // appearing; this edge lift is the quiet third signal. Only the
            // border animates, so the zone never changes value underfoot.
            focused ? "border-[hsl(0_0%_24%)]" : "border-[hsl(0_0%_16%)]",
          )}
        >
          <button
            type="button"
            aria-label="Attach files"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setShowFilePicker(true)}
            className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-[hsl(0_0%_16%)] text-[hsl(0_0%_76%)]"
          >
            <Paperclip className="h-4 w-4" />
          </button>

          {/* CSS-only auto-grow: an invisible mirror in the same grid cell
              sizes the row to the content, so no JS measures anything.
              --compose-max-h caps growth at exactly three lines: 3 * one row
              (--compose-row-h, set on the spacer above) plus the shared
              vertical padding (py-1.5 => 0.75rem top+bottom total).
              It must be applied to the MIRROR itself, not just this
              wrapper — capping only the wrapper's own box still lets the
              grid row size to the mirror's full, uncapped content, so the
              textarea lays out taller than the wrapper and is merely
              clipped by overflow-hidden rather than made scrollable
              (scrollHeight === clientHeight in that case). The mirror and
              textarea must keep identical py-1.5 / text size / line-height,
              or the mirror mis-measures the textarea and the row sizes to
              the wrong number of lines. */}
          <div
            data-testid="compose-grow-wrapper"
            className="grid max-h-[var(--compose-max-h)] flex-1 overflow-hidden"
            style={
              {
                "--compose-max-h": "calc(3 * var(--compose-row-h) + 0.75rem)",
              } as CSSProperties
            }
          >
            <div
              data-testid="compose-mirror"
              aria-hidden="true"
              className="invisible max-h-[var(--compose-max-h)] [grid-area:1/1/2/2] py-1.5 text-[15px] leading-[var(--compose-row-h)] break-words whitespace-pre-wrap"
            >
              {text + " "}
            </div>
            <textarea
              ref={textareaRef}
              rows={1}
              value={text}
              onChange={(e) => setText(e.target.value)}
              onFocus={() => setFocused(true)}
              onBlur={() => setFocused(false)}
              placeholder={focused ? "Message…" : "Tap to compose"}
              className="w-full resize-none overflow-y-auto bg-transparent py-1.5 text-[15px] leading-[var(--compose-row-h)] [grid-area:1/1/2/2] focus:outline-none"
            />
          </div>

          {showSend && (
            <button
              type="button"
              aria-label="Send"
              disabled={!canSend}
              onMouseDown={(e) => e.preventDefault()}
              onClick={handleSend}
              // Disabled is a solid dim fill at full opacity, not a faded
              // outline: a ghost of a ghost reads as broken, where a filled but
              // dim control reads as "present, not yet available".
              className="bg-primary text-primary-foreground disabled:bg-[hsl(0_0%_18%)] disabled:text-[hsl(0_0%_48%)] flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full"
            >
              <SendHorizontal className="h-4 w-4" />
            </button>
          )}
        </div>
      </div>

      <FilePicker
        open={showFilePicker}
        onOpenChange={setShowFilePicker}
        onPick={handleFilesPicked}
        searchPath={workingDirectory ?? undefined}
      />
    </>
  );
});
