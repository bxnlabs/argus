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
  /** Mirrors ComposeBar's focus so both halves of the card brighten together. */
  focused?: boolean;
}

interface PopoverState {
  buttonIndex: number;
  options: { label: string; key: string }[];
  triggerRect: DOMRect;
}

export const TerminalToolbar = memo(function TerminalToolbar({
  onKeyPress,
  focused = false,
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
        className={cn(
          // The card's bottom half — see the matching comment on ComposeBar's
          // panel for why the card is two boxes rather than one.
          "scrollbar-none bg-input flex items-center min-[500px]:justify-center overflow-x-auto rounded-b-lg border-x border-t border-b mx-2 mb-1.5 transition-colors",
          focused ? "border-[hsl(0_0%_30%)]" : "border-[hsl(0_0%_20%)]",
          // The seam where the two halves meet is the row divider, at --border
          // (14%) — quieter than the card's own edge so it separates without
          // competing. It stays a border rather than becoming a child element
          // so the row height is unchanged and the divider costs nothing.
          //
          // MUST come last. twMerge treats `border-t-<color>` and
          // `border-<color>` as conflicting groups, so a generic border colour
          // appearing after this line would silently delete the divider.
          "border-t-[hsl(0_0%_14%)]",
        )}
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
                "text-[hsl(0_0%_72%)] active:bg-white/10",
                // focus-visible only: onMouseDown preventDefault above means a
                // tap never focuses these, so the ring comes from keyboard or
                // programmatic/AT focus — never from the touch path.
                // ring-inset because the toolbar scrolls horizontally, which
                // makes overflow-y compute to auto and would clip an outset
                // ring at the row's top and bottom edges. Full opacity, not
                // /50, which blends to ~2:1 against this surface.
                "focus-visible:ring-ring outline-none focus-visible:ring-2 focus-visible:ring-inset"
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
