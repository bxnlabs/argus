import { memo, useState } from "react";
import { Globe, CornerDownLeft } from "lucide-react";
import { cn } from "@/lib/utils";
import { KeyPopover } from "./KeyPopover";

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
  CTRL_Y: "\x19",
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
    menu: [{ label: "\u21e7-tab", key: SPECIAL_KEYS.SHIFT_TAB }, { label: "^Y", key: SPECIAL_KEYS.CTRL_Y }],
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
}

interface PopoverState {
  buttonIndex: number;
  options: { label: string; key: string }[];
  triggerRect: DOMRect;
}

export const TerminalToolbar = memo(function TerminalToolbar({
  onKeyPress,
}: TerminalToolbarProps) {
  const [popover, setPopover] = useState<PopoverState | null>(null);

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
      <div
        data-testid="terminal-toolbar"
        // Shares ComposeBar's surface so the two rows read as one input zone.
        // The border-t stays but goes transparent: the shared surface already
        // groups them, and keeping the 1px preserves the row's height.
        className="scrollbar-none flex items-center min-[500px]:justify-center overflow-x-auto border-t border-transparent bg-[hsl(0_0%_8%)]"
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
                "text-[hsl(0_0%_72%)] active:bg-white/10"
              )}
            >
              {btn.icon ? (
                <btn.icon className="h-4 w-4" />
              ) : (
                btn.label
              )}
              {/* Underline rather than the old 1x1px corner dot, which
                  rendered as roughly one physical pixel and read as dust
                  instead of "this key opens a menu". */}
              {isMenu && (
                <span className="absolute bottom-1 left-1/2 h-[1.5px] w-2.5 -translate-x-1/2 rounded-[1px] bg-[hsl(0_0%_45%)]" />
              )}
            </button>
          );
        })}
      </div>
    </>
  );
});
