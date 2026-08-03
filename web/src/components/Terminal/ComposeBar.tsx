import { memo, useCallback, useEffect, useRef, useState } from "react";
import { SendHorizontal, Paperclip } from "lucide-react";
import { cn } from "@/lib/utils";
import { FilePicker } from "@/components/FilePicker";
import { insertPaths } from "./insertPaths";

interface ComposeBarProps {
  onSend: (text: string) => void;
  /** Send is disabled while the socket is down; the draft is kept. */
  connected: boolean;
  /** Session working directory, used to anchor file search. */
  workingDirectory?: string | null;
  /**
   * Reports how far the panel currently overflows its one-line spacer, so the
   * terminal can be shifted up by the same amount. Wired in Task 4.
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
  // Height of the panel at one line, captured from the first observation. The
  // spacer's h-11 is only a pre-measurement fallback; everything downstream
  // uses the measured value, so no hardcoded pixel height can drift.
  const collapsedHeightRef = useRef<number | null>(null);
  // Read through a ref so an unstable onOverlayHeightChange identity can't
  // force the effect below to rebuild.
  const onOverlayHeightChangeRef = useRef(onOverlayHeightChange);
  onOverlayHeightChangeRef.current = onOverlayHeightChange;

  useEffect(() => {
    const el = panelRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;

    const ro = new ResizeObserver((entries) => {
      const height = entries[0]?.contentRect.height ?? 0;
      if (collapsedHeightRef.current === null) {
        collapsedHeightRef.current = height;
      }
      onOverlayHeightChangeRef.current?.(
        Math.max(0, height - (collapsedHeightRef.current ?? height)),
      );
    });

    ro.observe(el);
    return () => ro.disconnect();
    // Empty deps: the observer must be built exactly once. collapsedHeightRef
    // is defined as "the first height observed after mount" — rebuilding the
    // observer mid-life (e.g. because a parent passes an inline callback)
    // would recapture that baseline at an already-grown height and
    // under-report the overlay for the rest of the component's life.
  }, []);

  const canSend = connected && text.trim().length > 0;
  // An empty, unfocused bar stays quiet chrome — but a draft must remain
  // sendable after the user taps away to the terminal to hit a special key.
  const showSend = focused || text.length > 0;

  const handleSend = useCallback(() => {
    if (!canSend) return;
    onSend(text);
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
          never triggers a FitAddon refit. */}
      <div className="relative h-11 flex-shrink-0">
        <div
          ref={panelRef}
          className={cn(
            "absolute inset-x-0 bottom-0 flex items-end gap-1.5 border-t px-2 py-1.5 backdrop-blur transition-colors",
            focused
              ? "border-white/25 bg-secondary/50"
              : "border-white/10 bg-transparent",
          )}
        >
          <button
            type="button"
            aria-label="Attach files"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setShowFilePicker(true)}
            className="text-secondary-foreground flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-white/20"
          >
            <Paperclip className="h-4 w-4" />
          </button>

          {/* CSS-only auto-grow: an invisible mirror in the same grid cell
              sizes the row to the content, so no JS measures anything. The
              wrapper caps at three lines and the textarea scrolls inside it. */}
          <div className="grid max-h-[3lh] flex-1 overflow-hidden">
            <div
              data-testid="compose-mirror"
              aria-hidden="true"
              className="invisible [grid-area:1/1/2/2] py-1.5 text-sm break-words whitespace-pre-wrap"
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
              className="min-h-8 w-full resize-none overflow-y-auto bg-transparent py-1.5 text-sm [grid-area:1/1/2/2] focus:outline-none"
            />
          </div>

          {showSend && (
            <button
              type="button"
              aria-label="Send"
              disabled={!canSend}
              onMouseDown={(e) => e.preventDefault()}
              onClick={handleSend}
              className="text-secondary-foreground flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full border border-white/20 disabled:opacity-30"
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
