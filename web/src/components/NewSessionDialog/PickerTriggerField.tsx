import { forwardRef } from "react";
import { cn } from "@/lib/utils";

interface PickerTriggerFieldProps {
  label: string;
  value: string;
  placeholder: string;
  onOpen: () => void;
  optional?: boolean;
  open?: boolean;
}

export const PickerTriggerField = forwardRef<
  HTMLButtonElement,
  PickerTriggerFieldProps
>(function PickerTriggerField(
  { label, value, placeholder, onOpen, optional, open },
  ref,
) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">
        {label}
        {optional && (
          <span className="text-muted-foreground font-normal"> (optional)</span>
        )}
      </label>
      <button
        ref={ref}
        type="button"
        onClick={onOpen}
        aria-haspopup="dialog"
        aria-expanded={open ?? false}
        className={cn(
          "border-input bg-transparent flex h-9 w-full items-center rounded-md border px-3 py-1 text-left text-sm shadow-xs transition-[color,box-shadow] outline-none",
          "focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
          value ? "text-foreground" : "text-muted-foreground",
        )}
      >
        <span className="truncate">{value || placeholder}</span>
      </button>
    </div>
  );
});
