import { memo, useCallback, useEffect, useRef, useState } from "react";
import { Globe, PenLine, SendHorizontal, CornerDownLeft, Paperclip } from "lucide-react";
import { cn } from "@/lib/utils";
import { KeyPopover } from "./KeyPopover";
import { FilePicker } from "@/components/FilePicker";
import { shellEscape } from "@/lib/shell";

// ANSI escape sequences
const SPECIAL_KEYS = {
  UP: "\x1b[A",
  DOWN: "\x1b[B",
  LEFT: "\x1b[D",
  RIGHT: "\x1b[C",
  ESC: "\x1b",
  TAB: "\t",
  CTRL_C: "\x03",
  CTRL_D: "\x04",
  SHIFT_TAB: "\x1b[Z",
  CTRL_B: "\x02",
  CTRL_Z: "\x1a",
  CTRL_L: "\x0c",
  RETURN: "\r",
} as const;

type MenuOption = { label: string; key: string; className?: string };

type ToolbarButton =
  | { label: string; key: string; icon?: React.ComponentType<{ className?: string }>; menu?: never }
  | { label: string; icon?: React.ComponentType<{ className?: string }>; menu: MenuOption[]; key?: never };

const TOOLBAR_BUTTONS: ToolbarButton[] = [
  {
    label: "more",
    icon: Globe,
    menu: [{ label: "\u21e7-tab", key: SPECIAL_KEYS.SHIFT_TAB }],
  },
  { label: "esc", key: SPECIAL_KEYS.ESC },
  {
    label: "ctrl",
    menu: [
      { label: "^C", key: SPECIAL_KEYS.CTRL_C, className: "text-red-400" },
      { label: "^D", key: SPECIAL_KEYS.CTRL_D },
      { label: "^B", key: SPECIAL_KEYS.CTRL_B },
      { label: "^Z", key: SPECIAL_KEYS.CTRL_Z },
      { label: "^L", key: SPECIAL_KEYS.CTRL_L },
    ],
  },
  { label: "\u2190", key: SPECIAL_KEYS.LEFT },
  { label: "\u2192", key: SPECIAL_KEYS.RIGHT },
  { label: "\u2191", key: SPECIAL_KEYS.UP },
  { label: "\u2193", key: SPECIAL_KEYS.DOWN },
  { label: "tab", key: SPECIAL_KEYS.TAB },
  { label: "return", key: SPECIAL_KEYS.RETURN, icon: CornerDownLeft },
];

interface TerminalToolbarProps {
  onKeyPress: (key: string) => void;
  onSendText: (text: string) => void;
  onAttachments?: () => void;
  visible?: boolean;
}

interface PopoverState {
  buttonIndex: number;
  options: { label: string; key: string }[];
  triggerRect: DOMRect;
}

// Compose modal for typing/pasting text with native keyboard features
function ComposeInput({
  open,
  onClose,
  onSend,
}: {
  open: boolean;
  onClose: () => void;
  onSend: (text: string) => void;
}) {
  const [text, setText] = useState("");
  const [showFilePicker, setShowFilePicker] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleSend = useCallback(() => {
    if (text.trim()) {
      onSend(text);
      setText("");
      onClose();
    }
  }, [text, onSend, onClose]);

  // Insert shell-escaped paths at the cursor position in the textarea
  const handleFilesPicked = useCallback(
    (paths: string[]) => {
      const insert = paths.map(shellEscape).join(" ");
      const ta = textareaRef.current;
      if (!ta) {
        setText((prev) => (prev ? prev + " " + insert : insert));
        return;
      }
      const start = ta.selectionStart;
      const end = ta.selectionEnd;
      const before = text.slice(0, start);
      const after = text.slice(end);
      // Add a space separator if there's adjacent text
      const needsLeadingSpace = before.length > 0 && !before.endsWith(" ");
      const needsTrailingSpace = after.length > 0 && !after.startsWith(" ");
      const full =
        before +
        (needsLeadingSpace ? " " : "") +
        insert +
        (needsTrailingSpace ? " " : "") +
        after;
      setText(full);
      // Restore cursor after the inserted text
      const cursorPos =
        before.length + (needsLeadingSpace ? 1 : 0) + insert.length;
      requestAnimationFrame(() => {
        ta.focus();
        ta.setSelectionRange(cursorPos, cursorPos);
      });
    },
    [text],
  );

  // Focus textarea on open, reset on close
  useEffect(() => {
    if (open) {
      textareaRef.current?.focus();
    } else {
      setText("");
      setShowFilePicker(false);
    }
  }, [open]);

  // Re-focus textarea when file picker closes
  useEffect(() => {
    if (!showFilePicker && open) {
      requestAnimationFrame(() => textareaRef.current?.focus());
    }
  }, [showFilePicker, open]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-x-0 top-0 z-50 flex items-end justify-center bg-black/50 pb-2"
      style={{ height: "var(--app-height, 100vh)" }}
      onClick={() => { if (!showFilePicker) onClose(); }}
    >
      <div
        className="bg-background flex w-[90%] max-w-md flex-col rounded-xl border border-white/20"
        onClick={(e) => e.stopPropagation()}
      >
        <textarea
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Type or paste..."
          autoFocus
          className="min-h-[4lh] max-h-60 w-full resize-none bg-transparent px-3 pt-3 pb-3 text-sm focus:outline-none"
        />
        <div className="flex items-center justify-between px-2.5 pb-2.5">
          <button
            onClick={() => setShowFilePicker(true)}
            className="text-secondary-foreground flex h-8 w-8 items-center justify-center rounded-full border border-white/20"
          >
            <Paperclip className="h-4 w-4" />
          </button>
          <button
            onClick={handleSend}
            disabled={!text.trim()}
            className="text-secondary-foreground flex h-8 w-8 items-center justify-center rounded-full border border-white/20 disabled:opacity-30"
          >
            <SendHorizontal className="h-4 w-4" />
          </button>
        </div>
      </div>
      <FilePicker
        open={showFilePicker}
        onOpenChange={setShowFilePicker}
        onPick={handleFilesPicked}
      />
    </div>
  );
}

export const TerminalToolbar = memo(function TerminalToolbar({
  onKeyPress,
  onSendText,
  onAttachments,
  visible = true,
}: TerminalToolbarProps) {
  const [showCompose, setShowCompose] = useState(false);
  const [popover, setPopover] = useState<PopoverState | null>(null);

  if (!visible) return null;

  return (
    <>
      {popover && (
        <KeyPopover
          options={popover.options}
          triggerRect={popover.triggerRect}
          onSelect={(key) => {
            onKeyPress(key);
          }}
          onClose={() => setPopover(null)}
        />
      )}
      <ComposeInput
        open={showCompose}
        onClose={() => setShowCompose(false)}
        onSend={onSendText}
      />
      <div
        data-testid="terminal-toolbar"
        className="bg-secondary/50 scrollbar-none flex items-center min-[500px]:justify-center overflow-x-auto border-t border-white/5 backdrop-blur"
      >
        {/* Special keys */}
        {TOOLBAR_BUTTONS.map((btn, index) => {
          const isMenu = !!btn.menu;

          return (
            <button
              type="button"
              key={btn.label}
              onMouseDown={(e) => e.preventDefault()}
              onClick={(e) => {
                e.stopPropagation();
                if (isMenu) {
                  // Toggle popover for menu buttons
                  if (popover?.buttonIndex === index) {
                    setPopover(null);
                  } else {
                    const rect = (
                      e.currentTarget as HTMLElement
                    ).getBoundingClientRect();
                    setPopover({
                      buttonIndex: index,
                      options: btn.menu!,
                      triggerRect: rect,
                    });
                  }
                } else {
                  onKeyPress(btn.key);
                }
              }}
              className={cn(
                "relative flex-shrink-0 select-none px-3.5 py-2.5 text-sm font-medium",
                "text-secondary-foreground active:bg-white/10"
              )}
            >
              {btn.icon ? (
                <btn.icon className="h-4 w-4" />
              ) : (
                btn.label
              )}
              {isMenu && (
                <span className="absolute top-0.5 right-0.5 h-1 w-1 rounded-full bg-white/30" />
              )}
            </button>
          );
        })}

        {/* Compose button */}
        <button
          type="button"
          onMouseDown={(e) => e.preventDefault()}
          onClick={(e) => {
            e.stopPropagation();
            setShowCompose((v) => !v);
          }}
          className={cn(
            "flex-shrink-0 select-none px-3.5 py-2.5 text-sm font-medium",
            showCompose
              ? "text-primary-foreground bg-white/10"
              : "text-secondary-foreground active:bg-white/10"
          )}
        >
          <PenLine className="h-4 w-4" />
        </button>

        {/* Attachments button */}
        {onAttachments && (
          <>
            <button
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={(e) => {
                e.stopPropagation();
                onAttachments();
              }}
              className="flex-shrink-0 select-none px-3.5 py-2.5 text-sm font-medium text-secondary-foreground active:bg-white/10"
              aria-label="Attach files"
            >
              <Paperclip className="h-4 w-4" />
            </button>
          </>
        )}

      </div>
    </>
  );
});
