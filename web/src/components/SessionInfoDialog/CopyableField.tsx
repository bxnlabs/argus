import { useState, useRef, useEffect, type ReactNode } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { copyToClipboard } from "@/lib/clipboard";

interface CopyableFieldProps {
  label: string;
  // What the user sees (e.g. a tilde-contracted path).
  displayValue: string;
  // What gets copied; defaults to displayValue (e.g. the full absolute path).
  copyValue?: string;
  // Optional non-copyable adornment shown after the value (e.g. a type badge).
  badge?: ReactNode;
}

export function CopyableField({
  label,
  displayValue,
  copyValue,
  badge,
}: CopyableFieldProps) {
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // The clipboard fallback (non-secure contexts, e.g. argus over plain HTTP)
  // selects a hidden textarea. Anchor it inside this component so it stays
  // within the dialog's focus trap; otherwise the trap steals focus mid-copy
  // and nothing lands on the clipboard. See copyToClipboard.
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const handleCopy = async () => {
    const ok = await copyToClipboard(
      copyValue ?? displayValue,
      rootRef.current ?? undefined,
    );
    if (ok) {
      setCopied(true);
      toast.success("Copied");
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
      timeoutRef.current = setTimeout(() => setCopied(false), 1200);
    } else {
      toast.error("Copy failed");
    }
  };

  const Icon = copied ? Check : Copy;
  const copyButton = (
    <button
      type="button"
      onClick={handleCopy}
      aria-label={`Copy ${label}`}
      className="text-muted-foreground hover:text-foreground flex-shrink-0"
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  );

  return (
    <div ref={rootRef} className="space-y-1">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="bg-muted/50 flex items-center gap-2 rounded-md border px-2 py-1.5">
        <span className="min-w-0 flex-1 break-all font-mono text-xs">
          {displayValue}
        </span>
        {badge}
        {copyButton}
      </div>
    </div>
  );
}
