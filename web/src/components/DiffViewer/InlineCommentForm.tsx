import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { isMac } from "@/lib/device";

interface InlineCommentFormProps {
  onSubmit: (body: string) => void;
  onCancel: () => void;
  initialBody?: string;
  submitLabel?: string;
}

export function InlineCommentForm({ onSubmit, onCancel, initialBody = "", submitLabel = "Comment" }: InlineCommentFormProps) {
  const [body, setBody] = useState(initialBody);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (body.trim()) onSubmit(body.trim());
    }
    if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  };

  return (
    <>
      <textarea
        ref={textareaRef}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Write a comment..."
        rows={3}
        className="bg-background/60 border-border placeholder:text-muted-foreground/50 text-foreground w-full resize-y rounded border px-3 py-2 text-sm leading-relaxed focus:border-primary/60 focus:outline-none focus:ring-1 focus:ring-primary/30"
      />
      <div className="mt-2 flex items-center justify-end gap-1.5">
          <Button size="sm" variant="ghost" onClick={onCancel}>
            Cancel
          </Button>
          <Button
            size="sm"
            onClick={() => body.trim() && onSubmit(body.trim())}
            disabled={!body.trim()}
            className="gap-1.5"
          >
            {submitLabel}
            <kbd className="text-[10px] opacity-60">
              {isMac() ? "⌘ ↵" : "Ctrl ↵"}
            </kbd>
          </Button>
      </div>
    </>
  );
}
