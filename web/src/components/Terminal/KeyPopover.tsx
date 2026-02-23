import { createPortal } from "react-dom";
import { cn } from "@/lib/utils";

interface KeyOption {
  label: string;
  key: string;
  className?: string;
}

interface KeyPopoverProps {
  options: KeyOption[];
  triggerRect: DOMRect;
  onSelect: (key: string) => void;
  onClose: () => void;
}

export function KeyPopover({
  options,
  triggerRect,
  onSelect,
  onClose,
}: KeyPopoverProps) {
  const GAP = 8;

  // Horizontal center on trigger, clamped to screen edges
  const popoverWidth = options.length * 52 + 8;
  const screenPad = 8;
  let left = triggerRect.left + triggerRect.width / 2 - popoverWidth / 2;
  left = Math.max(
    screenPad,
    Math.min(left, window.innerWidth - popoverWidth - screenPad)
  );

  const bottom = window.innerHeight - triggerRect.top + GAP;

  // Arrow position relative to popover
  const arrowLeft = Math.min(
    Math.max(12, triggerRect.left + triggerRect.width / 2 - left),
    popoverWidth - 12
  );

  return createPortal(
    <>
      {/* Invisible backdrop -- tap to dismiss */}
      <div
        className="fixed inset-0"
        style={{ zIndex: 44 }}
        onMouseDown={(e) => e.preventDefault()}
        onClick={(e) => {
          e.stopPropagation();
          onClose();
        }}
      />

      {/* Popover */}
      <div
        className="animate-in zoom-in-95 fade-in-0 pointer-events-auto fixed duration-100"
        style={{
          left,
          bottom,
          zIndex: 45,
          transformOrigin: "bottom center",
        }}
      >
        <div className="bg-secondary flex items-center gap-0.5 rounded-lg border border-white/10 p-1 shadow-lg">
          {options.map((opt) => (
            <button
              key={opt.key}
              type="button"
              onMouseDown={(e) => e.preventDefault()}
              onClick={(e) => {
                e.stopPropagation();
                onSelect(opt.key);
              }}
              className={cn(
                "flex-shrink-0 rounded-md px-3.5 py-2 text-sm font-medium select-none",
                "text-secondary-foreground active:text-foreground active:bg-white/20",
                opt.className
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
        {/* Arrow */}
        <div
          className="absolute -bottom-1.5 h-0 w-0"
          style={{
            left: arrowLeft,
            marginLeft: -6,
            borderLeft: "6px solid transparent",
            borderRight: "6px solid transparent",
            borderTop: "6px solid var(--secondary)",
          }}
        />
      </div>
    </>,
    document.body
  );
}
