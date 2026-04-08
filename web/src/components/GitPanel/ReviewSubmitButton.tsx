import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { MessageSquare, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";
import { useViewport } from "@/hooks/useViewport";
import {
  Sheet,
  SheetContent,
  SheetTitle,
} from "@/components/ui/sheet";

interface ReviewSubmitButtonProps {
  pendingCount: number;
  generalComment: string;
  onGeneralCommentChange: (body: string) => void;
  onSubmit: (generalCommentBody: string) => void;
}

export function ReviewSubmitButton({
  pendingCount,
  generalComment,
  onGeneralCommentChange,
  onSubmit,
}: ReviewSubmitButtonProps) {
  const { isMobile } = useViewport();
  const [open, setOpen] = useState(false);
  const [localComment, setLocalComment] = useState(generalComment);
  const popoverRef = useRef<HTMLDivElement>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    setLocalComment(generalComment);
  }, [generalComment]);

  // Close popover on outside click (desktop only)
  useEffect(() => {
    if (!open || isMobile) return;
    const handler = (e: MouseEvent) => {
      if (
        popoverRef.current &&
        !popoverRef.current.contains(e.target as Node) &&
        buttonRef.current &&
        !buttonRef.current.contains(e.target as Node)
      ) {
        setOpen(false);
        // Save draft on close
        if (localComment !== generalComment) {
          onGeneralCommentChange(localComment);
        }
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [open, isMobile, localComment, generalComment, onGeneralCommentChange]);

  const handleSubmit = () => {
    onSubmit(localComment);
    setOpen(false);
  };

  const buttonLabel = pendingCount > 0
    ? `Finish review (${pendingCount})`
    : "Finish review";

  // Mobile: use bottom sheet
  if (isMobile) {
    return (
      <>
        <Button
          size="sm"
          onClick={() => setOpen(true)}
          className="gap-1.5"
        >
          <MessageSquare className="h-3.5 w-3.5" />
          {buttonLabel}
        </Button>
        <Sheet open={open} onOpenChange={(v) => {
          setOpen(v);
          if (!v && localComment !== generalComment) {
            onGeneralCommentChange(localComment);
          }
        }}>
          <SheetContent side="bottom" hideCloseButton className="top-0 flex flex-col px-4 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
            <div className="flex items-center justify-between py-3">
              <SheetTitle className="text-base font-semibold">Finish Review</SheetTitle>
              <div className="flex items-center gap-2">
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setOpen(false);
                    if (localComment !== generalComment) {
                      onGeneralCommentChange(localComment);
                    }
                  }}
                >
                  Cancel
                </Button>
                <Button
                  size="sm"
                  onClick={handleSubmit}
                  disabled={pendingCount === 0 && !localComment.trim()}
                >
                  Submit Review
                </Button>
              </div>
            </div>
            <div>
              <label className="text-muted-foreground mb-1.5 block text-sm">
                Review Message
              </label>
              <textarea
                value={localComment}
                onChange={(e) => setLocalComment(e.target.value)}
                placeholder="Leave a comment..."
                rows={4}
                className="bg-background/60 border-border placeholder:text-muted-foreground/50 text-foreground w-full resize-y rounded border px-3 py-2 text-sm leading-relaxed focus:border-primary/60 focus:outline-none focus:ring-1 focus:ring-primary/30"
              />
            </div>
          </SheetContent>
        </Sheet>
      </>
    );
  }

  // Desktop: popover dropdown
  return (
    <div className="relative">
      <Button
        ref={buttonRef}
        size="sm"
        onClick={() => setOpen(!open)}
        className="gap-1.5"
      >
        {buttonLabel}
        <ChevronDown className="h-3 w-3" />
      </Button>

      {open && (
        <div
          ref={popoverRef}
          className="bg-popover border-border absolute right-0 top-full z-50 mt-1 w-[28rem] rounded-lg border p-4 shadow-lg"
        >
          <p className="mb-2 text-sm font-medium">Finish your review</p>
          <textarea
            value={localComment}
            onChange={(e) => setLocalComment(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                if (pendingCount > 0 || localComment.trim()) handleSubmit();
              }
            }}
            placeholder="Leave a comment"
            rows={4}
            className="bg-background border-border w-full rounded-md border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <div className="mt-3 flex items-center justify-between">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                setOpen(false);
                if (localComment !== generalComment) {
                  onGeneralCommentChange(localComment);
                }
              }}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={pendingCount === 0 && !localComment.trim()}
              className="gap-1.5"
            >
              Submit review
              <kbd className="text-[10px] opacity-60">
                {/Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘" : "Ctrl"}-Enter
              </kbd>
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
