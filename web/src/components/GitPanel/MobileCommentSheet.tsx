import { useState, useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  Sheet,
  SheetContent,
  SheetTitle,
} from "@/components/ui/sheet";
import type { DiffLine } from "@/lib/diff-parser";
import type { DiffPosition, ReviewComment } from "@/types";

interface MobileCommentSheetProps {
  activeComment: { file: string; position: DiffPosition } | null;
  activeLines: DiffLine[];
  onAddComment: (body: string) => void;
  onCancel: () => void;
  /** When set, the sheet opens in edit mode for this comment. */
  editingComment?: ReviewComment | null;
  onEditComment?: (id: string, body: string) => void;
  onCancelEdit?: () => void;
}

export function MobileCommentSheet({
  activeComment,
  activeLines,
  onAddComment,
  onCancel,
  editingComment,
  onEditComment,
  onCancelEdit,
}: MobileCommentSheetProps) {
  const [body, setBody] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const isEditing = !!editingComment;
  const isOpen = isEditing || activeComment !== null;

  // Reset body when a new comment target is selected (creation mode)
  useEffect(() => {
    if (activeComment && !editingComment) {
      setBody("");
    }
  }, [activeComment?.file, activeComment?.position?.side, activeComment?.position?.line, editingComment]);

  // Pre-populate body when entering edit mode
  useEffect(() => {
    if (editingComment) {
      setBody(editingComment.body);
    }
  }, [editingComment?.id]);

  // Focus textarea when sheet opens
  useEffect(() => {
    if (isOpen) {
      const t = setTimeout(() => textareaRef.current?.focus(), 100);
      return () => clearTimeout(t);
    }
  }, [isOpen]);

  const handleSubmit = () => {
    if (!body.trim()) return;
    if (isEditing && editingComment) {
      if (body.trim() !== editingComment.body) {
        onEditComment?.(editingComment.id, body.trim());
      }
      onCancelEdit?.();
    } else {
      onAddComment(body.trim());
      setBody("");
    }
  };

  const handleCancel = () => {
    if (isEditing) {
      onCancelEdit?.();
    } else {
      onCancel();
    }
  };

  const file = isEditing ? editingComment!.file : activeComment?.file;
  const lineLabel = isEditing
    ? `${editingComment!.line.to.side === "L" ? "Old" : "New"} Line ${editingComment!.line.to.line}`
    : activeComment
      ? `${activeComment.position.side === "L" ? "Old" : "New"} Line ${activeComment.position.line}`
      : "";

  return (
    <Sheet
      open={isOpen}
      onOpenChange={(open) => {
        if (!open) handleCancel();
      }}
    >
      <SheetContent side="bottom" hideCloseButton className="top-0 flex flex-col px-4 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
        <div className="flex items-center justify-between py-3">
          <div className="min-w-0 flex-1">
            <SheetTitle className="text-base font-semibold">
              {isEditing ? "Edit Comment" : "Add Comment"}
            </SheetTitle>
            <p className="text-muted-foreground mt-0.5 truncate text-xs">
              {file} &middot; {lineLabel}
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button size="sm" variant="ghost" onClick={handleCancel}>
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={!body.trim()}
            >
              {isEditing ? "Save" : "Comment"}
            </Button>
          </div>
        </div>

        {!isEditing && activeLines.length > 0 && (
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

        {isEditing && editingComment?.snippet && (
          <div className="border-border mb-3 overflow-x-auto rounded-md border font-mono text-xs">
            <div className="text-foreground flex whitespace-pre">
              <span className="text-muted-foreground w-8 shrink-0 select-none px-1.5 py-0.5 text-right">
                {editingComment.line.to.line}
              </span>
              <span className="w-4 shrink-0 select-none py-0.5 text-center"> </span>
              <span className="py-0.5 pr-2">{editingComment.snippet}</span>
            </div>
          </div>
        )}

        <textarea
          ref={textareaRef}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Leave a comment..."
          rows={4}
          className="bg-background/60 border-border placeholder:text-muted-foreground/50 text-foreground w-full resize-y rounded border px-3 py-2 text-sm leading-relaxed focus:border-primary/60 focus:outline-none focus:ring-1 focus:ring-primary/30"
        />
      </SheetContent>
    </Sheet>
  );
}
