import { memo, useCallback, useEffect, useRef, useState } from "react";
import type { CSSProperties } from "react";
import { SendHorizontal, Paperclip } from "lucide-react";
import { cn, truncateRight } from "@/lib/utils";
import { FilePicker } from "@/components/FilePicker";
import { useActiveNode } from "@/hooks/useActiveNode";
import { loadDraft, saveDraft } from "@/lib/composeDrafts";
import { insertPaths } from "./insertPaths";

// A textarea placeholder cannot ellipsize itself, so without a cap a long name
// clips mid-word at the input edge and reads as a bug.
//
// A character count cannot express "what fits" in a proportional font — at the
// same length, real slugs span 279-318px against the 276px the input offers at
// 390px wide alongside both action glyphs. 22 is the measured length at which
// every slug in a sample of real `argus session ls` names fits; the plan's
// original 28 overflowed by 6px on `review-mike-dp-host-self-registration`,
// the very name it was chosen to accommodate. Measured in Chrome at 390x844.
//
// The cap still holds after the card was centred (which cost the input 4px)
// because the prefix shrank by more than that in the same edit: "Send to #"
// measures ~9px narrower than the "Message #" the cap was fitted against,
// leading with an S rather than the widest glyph in the alphabet.
//
// So this bounds the common case rather than guaranteeing the general one: a
// slug of unusually wide glyphs can still clip. Fixing that needs render-time
// text measurement, which is not worth its machinery for a placeholder.
const MAX_SLUG_CHARS = 22;

function composePlaceholder(slug?: string | null): string {
  // No slug means no session behind this tab — a raw shell. There is no
  // channel to name, so the destination stays generic rather than borrowing
  // the "#" that stands for a specific session everywhere else.
  if (!slug) return "Send to session";
  return `Send to #${truncateRight(slug, MAX_SLUG_CHARS)}`;
}

interface ComposeBarProps {
  /**
   * Identity the draft is stored under, within the active node's scope: the id
   * of the tab this bar belongs to.
   *
   * The draft belongs to the BOX, and there is exactly one box per tab — so it
   * is keyed by the tab and lives and dies with it. Attaching a different
   * session to the tab does not disturb it: the tab is still open and the text
   * is still the user's. And because no two bars mounted in one document can
   * share a key, none of them can go stale behind another's write.
   */
  draftKey: string;
  /** Returns false when the text never reached the socket — keep the draft. */
  onSend: (text: string) => boolean;
  /** Send is disabled while the socket is down; the draft is kept. */
  connected: boolean;
  /** Session working directory, used to anchor file search. */
  workingDirectory?: string | null;
  /**
   * Server-derived session slug, rendered Slack-style as the destination.
   * Absent on a raw-shell tab, which has no session.
   */
  sessionSlug?: string | null;
  /**
   * Reports how far the panel overflows its one-line spacer, so the terminal
   * can be shifted up by the same amount.
   */
  onOverlayHeightChange?: (height: number) => void;
  /**
   * Focus is signalled by the whole card's edge, but the card's bottom half
   * is TerminalToolbar — a sibling component. This lifts the state so both
   * halves brighten together.
   */
  onFocusedChange?: (focused: boolean) => void;
}

export const ComposeBar = memo(function ComposeBar({
  draftKey,
  onSend,
  connected,
  workingDirectory,
  sessionSlug,
  onOverlayHeightChange,
  onFocusedChange,
}: ComposeBarProps) {
  const { scope } = useActiveNode();
  // Hydrate from the persisted draft rather than "": the bar can be remounted
  // by anything from a node blip to a reload, and the text is the user's, not
  // the mount's.
  const [text, setText] = useState(() => loadDraft(scope, draftKey));
  const [focused, setFocused] = useState(false);
  const [showFilePicker, setShowFilePicker] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const panelRef = useRef<HTMLDivElement>(null);
  const spacerRef = useRef<HTMLDivElement>(null);
  // Read through a ref so an unstable callback identity can't rebuild the
  // observer mid-life.
  const onOverlayHeightChangeRef = useRef(onOverlayHeightChange);
  onOverlayHeightChangeRef.current = onOverlayHeightChange;

  // Every write to the draft goes through here, so persistence cannot be
  // forgotten by a new call site (the textarea and the file picker are both
  // writers).
  const applyText = useCallback(
    (next: string) => {
      setText(next);
      saveDraft(scope, draftKey, next);
    },
    [scope, draftKey],
  );

  useEffect(() => {
    const el = panelRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;

    const ro = new ResizeObserver(() => {
      const panel = panelRef.current;
      const spacer = spacerRef.current;
      if (!panel || !spacer) return;
      // Measure the overflow against the SPACER, not against the first height
      // this observer happened to see. The spacer's height is the panel's
      // resting height by construction — both derive from --compose-row-h, and
      // ComposeBar.test.tsx pins that they cannot drift — so the subtraction is
      // exact on every delivery and needs no captured baseline.
      //
      // A captured baseline was correct only while the bar always mounted
      // empty. Now that a draft is restored in useState's initializer, the
      // FIRST observation can already be a two- or three-line panel, which such
      // a baseline would record as "resting" — leaving the terminal unshifted
      // and the card overlapping live output for the whole life of that mount.
      //
      // offsetHeight on both sides, so both are border-box: the panel's own
      // padding and border are part of the height the spacer has to hold. (The
      // observer's contentRect, which this used to read, excludes them — fine
      // when comparing observations to each other, wrong against the spacer.)
      // Inside a display:none ancestor both measure 0 and this reports 0, which
      // is the truth for a hidden tab rather than a poisoned baseline.
      onOverlayHeightChangeRef.current?.(
        Math.max(0, panel.offsetHeight - spacer.offsetHeight),
      );
    });

    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const canSend = connected && text.trim().length > 0;

  const handleSend = useCallback(() => {
    if (!canSend) return;
    // `connected` is a render-time snapshot, so the socket can close between
    // the last connected render and this tap. Clearing on a write that went
    // nowhere is exactly the silent draft loss this bar exists to prevent.
    if (!onSend(text)) return;
    setText("");
    // A send is the moment the text stops being a draft, so the stored copy
    // goes with it — an already-sent message must not come back on the next
    // mount.
    saveDraft(scope, draftKey, "");
    // Keep the keyboard up for the next message.
    textareaRef.current?.focus();
  }, [canSend, onSend, text, scope, draftKey]);

  const handleFocusChange = useCallback(
    (next: boolean) => {
      setFocused(next);
      onFocusedChange?.(next);
    },
    [onFocusedChange],
  );

  const handleFilesPicked = useCallback(
    (paths: string[]) => {
      const ta = textareaRef.current;
      const start = ta?.selectionStart ?? text.length;
      const end = ta?.selectionEnd ?? text.length;
      const result = insertPaths(text, start, end, paths);
      applyText(result.text);
      requestAnimationFrame(() => {
        ta?.focus();
        ta?.setSelectionRange(result.cursor, result.cursor);
      });
    },
    [text, applyText],
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
                        + the panel's py-2
                        + the panel's 1px border-t
                        = var + 0.75rem + 1rem + 1px. */}
      <div
        ref={spacerRef}
        // mt-1.5 is the card's top margin. The 2026-08-04 refresh used 8px
        // here to turn an accidental sub-row remainder into a deliberate gap;
        // a bordered card states that boundary outright, so the gap no longer
        // has to carry the separation alone and pays 2px toward the card's
        // own margins. Margin, not padding: FitAddon measures the terminal
        // container, and padding would be counted as usable row space.
        className="relative mt-1.5 flex-shrink-0"
        style={
          {
            "--compose-row-h": "21px",
            height: "calc(var(--compose-row-h) + 1.75rem + 1px)",
          } as CSSProperties
        }
      >
        <div
          ref={panelRef}
          className={cn(
            // The card's top half. The panel is absolutely positioned and the
            // toolbar is in flow, so they cannot share one border box — each
            // renders half, and because the panel sits exactly on the spacer's
            // bottom edge while the toolbar begins immediately after it, the
            // two meet flush and the seam becomes the row divider.
            //
            // py-2: at the py-1 this shipped with, the input row stood 41px
            // against the toolbar's 41px and read as squished — the draft text
            // sat as close to the card's top edge as to the row divider. The
            // extra 8px gives the text some room of its own and makes the top
            // half the taller of the two, as in Slack. This is the PANEL's
            // padding, not the mirror's — the draft text keeps its own 6px,
            // which is why --compose-max-h is unchanged. The spacer height
            // above moves with this; they must not drift.
            "absolute inset-x-2 bottom-0 flex items-end gap-1.5 rounded-t-lg border-x border-t bg-input px-2 py-2 transition-colors",
            // Only the border animates, so the zone never changes value
            // underfoot. TerminalToolbar carries the same two values on the
            // bottom half via its `focused` prop.
            focused ? "border-[hsl(0_0%_30%)]" : "border-[hsl(0_0%_20%)]",
          )}
        >
          <button
            type="button"
            aria-label="Attach files"
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => setShowFilePicker(true)}
            // focus-visible only: onMouseDown preventDefault above means a tap
            // never focuses this button, so the ring comes from keyboard or
            // programmatic/AT focus — never from the touch path.
            // Full-opacity ring, not /50: blended at 50% the ring is ~2:1
            // against the card's bg-input surface, under the 3:1 non-text
            // contrast bar. (These buttons are ghost glyphs with no fill of
            // their own — the surface behind the ring is the card's.)
            className="focus-visible:ring-ring flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-[hsl(0_0%_60%)] outline-none focus-visible:ring-2"
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
              the wrong number of lines.

              The mirror also needs min-w-0. As a grid item its automatic
              minimum size is min-width:auto => its min-content width, and
              per CSS Text 3 the wrap opportunities break-words introduces do
              NOT count toward min-content. So one unbreakable token — a
              pasted URL — floored this single column at that token's full
              width (406px inside a 280px wrapper, measured in Chrome at
              390x844); the w-full textarea sized to the column and wrapped
              its text to a line box wider than the card, which
              overflow-hidden then clipped. The textarea needs no floor of
              its own — overflow-y-auto already zeroes its automatic minimum
              size — but the mirror's overflow must stay visible so its
              content can size the row, so it opts out by hand. */}
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
              className="invisible max-h-[var(--compose-max-h)] min-w-0 [grid-area:1/1/2/2] py-1.5 text-[15px] leading-[var(--compose-row-h)] break-words whitespace-pre-wrap"
            >
              {text + " "}
            </div>
            <textarea
              ref={textareaRef}
              rows={1}
              value={text}
              onChange={(e) => applyText(e.target.value)}
              onFocus={() => handleFocusChange(true)}
              onBlur={() => handleFocusChange(false)}
              placeholder={composePlaceholder(sessionSlug)}
              className="w-full resize-none overflow-y-auto bg-transparent py-1.5 text-[15px] leading-[var(--compose-row-h)] placeholder:text-[hsl(0_0%_50%)] [grid-area:1/1/2/2] focus:outline-none"
            />
          </div>

          <button
            type="button"
            aria-label="Send"
            disabled={!canSend}
            onMouseDown={(e) => e.preventDefault()}
            onClick={handleSend}
            // Always mounted, as in Slack. Disabled is a dimmer glyph rather
            // than a hidden control: at ghost weight it is quiet enough at
            // rest that hiding it bought nothing, and a visible-but-dim
            // control reads as "present, not yet available".
            // 45% is 3.86:1 on this surface — under the 4.5:1 text bar the
            // placeholder must clear, but over the 3:1 bar for glyphs.
            className="text-primary disabled:text-[hsl(0_0%_45%)] focus-visible:ring-ring flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full outline-none focus-visible:ring-2"
          >
            <SendHorizontal className="h-4 w-4" />
          </button>
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
