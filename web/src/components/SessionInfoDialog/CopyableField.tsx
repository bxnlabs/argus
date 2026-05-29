import { useState, useRef, useEffect } from "react";
import { Check, Copy } from "lucide-react";
import { toast } from "sonner";
import { copyToClipboard } from "@/lib/clipboard";

interface CopyableFieldProps {
  label: string;
  // What the user sees (e.g. a tilde-contracted path).
  displayValue: string;
  // What gets copied; defaults to displayValue (e.g. the full absolute path).
  copyValue?: string;
  // Inline layout (label + value on one row) instead of a boxed field.
  inline?: boolean;
}

export function CopyableField({
  label,
  displayValue,
  copyValue,
  inline = false,
}: CopyableFieldProps) {
  const [copied, setCopied] = useState(false);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, []);

  const handleCopy = async () => {
    const ok = await copyToClipboard(copyValue ?? displayValue);
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

  if (inline) {
    return (
      <div className="flex items-center gap-2 text-sm">
        <span className="text-muted-foreground w-20 flex-shrink-0">
          {label}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-xs">
          {displayValue}
        </span>
        {copyButton}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <div className="text-muted-foreground text-xs">{label}</div>
      <div className="bg-muted/50 flex items-center gap-2 rounded-md border px-2 py-1.5">
        <span className="min-w-0 flex-1 break-all font-mono text-xs">
          {displayValue}
        </span>
        {copyButton}
      </div>
    </div>
  );
}
