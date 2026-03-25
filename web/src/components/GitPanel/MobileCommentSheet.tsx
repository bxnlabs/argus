import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetTitle,
} from "@/components/ui/sheet";

interface MobileCommentSheetProps {
  activeComment: { file: string; from: number; to: number } | null;
  onAddComment: (body: string) => void;
  onCancel: () => void;
}

export function MobileCommentSheet({
  activeComment,
  onAddComment,
  onCancel,
}: MobileCommentSheetProps) {
  const [body, setBody] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Reset body when a new comment target is selected
  useEffect(() => {
    if (activeComment) {
      setBody("");
    }
  }, [activeComment?.file, activeComment?.from, activeComment?.to]);

  // Focus textarea when sheet opens
  useEffect(() => {
    if (activeComment) {
      // Small delay to let the sheet animation start
      const t = setTimeout(() => textareaRef.current?.focus(), 100);
      return () => clearTimeout(t);
    }
  }, [activeComment]);

  const handleSubmit = () => {
    if (body.trim()) {
      onAddComment(body.trim());
      setBody("");
    }
  };

  const lineLabel = activeComment
    ? activeComment.from === activeComment.to
      ? `Line ${activeComment.from}`
      : `Lines ${activeComment.from}-${activeComment.to}`
    : "";

  return (
    <Sheet
      open={activeComment !== null}
      onOpenChange={(open) => {
        if (!open) onCancel();
      }}
    >
      <SheetContent side="bottom" hideCloseButton className="rounded-t-xl px-4 pb-8 pt-4">
        <SheetTitle className="text-base font-semibold">Add Comment</SheetTitle>
        <p className="text-muted-foreground mt-1 text-xs">
          {activeComment?.file} {lineLabel}
        </p>
        <div className="mt-3">
          <textarea
            ref={textareaRef}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Leave a comment..."
            rows={4}
            className="bg-muted border-border w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <div className="mt-3 flex items-center justify-end gap-2">
            <Button size="sm" variant="ghost" onClick={onCancel}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={!body.trim()}
            >
              Comment
            </Button>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}
