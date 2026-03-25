import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";

interface InlineCommentFormProps {
  onSubmit: (body: string) => void;
  onCancel: () => void;
}

export function InlineCommentForm({ onSubmit, onCancel }: InlineCommentFormProps) {
  const [body, setBody] = useState("");
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
    <div className="border-border bg-muted/20 font-sans border-t p-3">
      <textarea
        ref={textareaRef}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Leave a comment..."
        rows={3}
        className="bg-background border-border w-full max-w-full resize-y rounded-md border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
      <div className="mt-2 flex items-center gap-2">
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          size="sm"
          onClick={() => body.trim() && onSubmit(body.trim())}
          disabled={!body.trim()}
        >
          Add comment
        </Button>
        <span className="text-muted-foreground ml-auto text-xs">
          {/Mac|iPhone|iPad/.test(navigator.userAgent) ? "\u2318" : "Ctrl"}+Enter to add
        </span>
      </div>
    </div>
  );
}
