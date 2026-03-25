import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  Sheet,
  SheetContent,
  SheetTitle,
} from "@/components/ui/sheet";
import type { DiffLine } from "@/lib/diff-parser";

interface MobileCommentSheetProps {
  activeComment: { file: string; from: number; to: number } | null;
  activeLines: DiffLine[];
  onAddComment: (body: string) => void;
  onCancel: () => void;
}

export function MobileCommentSheet({
  activeComment,
  activeLines,
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
      <SheetContent side="bottom" hideCloseButton className="top-0 flex flex-col px-4 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
        <div className="flex items-center justify-between py-3">
          <div className="min-w-0 flex-1">
            <SheetTitle className="text-base font-semibold">Add Comment</SheetTitle>
            <p className="text-muted-foreground mt-0.5 truncate text-xs">
              {activeComment?.file} &middot; {lineLabel}
            </p>
          </div>
          <div className="flex items-center gap-2">
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

        {activeLines.length > 0 && (
          <div className="border-border mb-3 overflow-x-auto rounded-md border font-mono text-xs">
            {activeLines.map((line, i) => {
              const marker = line.type === "addition" ? "+" : line.type === "deletion" ? "-" : " ";
              return (
                <div
                  key={i}
                  className={cn(
                    "flex whitespace-pre",
                    line.type === "addition" && "bg-green-500/10 text-green-400",
                    line.type === "deletion" && "bg-red-500/10 text-red-400",
                    line.type === "context" && "text-foreground",
                  )}
                >
                  <span className="text-muted-foreground w-8 shrink-0 select-none px-1.5 py-0.5 text-right">
                    {line.newLineNumber ?? ""}
                  </span>
                  <span className="w-4 shrink-0 select-none py-0.5 text-center">{marker}</span>
                  <span className="py-0.5 pr-2">{line.content || " "}</span>
                </div>
              );
            })}
          </div>
        )}

        <textarea
          ref={textareaRef}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Leave a comment..."
          rows={4}
          className="bg-muted border-border w-full flex-1 resize-none rounded-lg border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
        />
      </SheetContent>
    </Sheet>
  );
}
