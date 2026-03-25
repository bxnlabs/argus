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
  hasUnsubmitted: boolean;
}

export function ReviewSubmitButton({
  pendingCount,
  generalComment,
  onGeneralCommentChange,
  onSubmit,
  hasUnsubmitted,
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
        // Save draft on close
        if (localComment !== generalComment) {
          onGeneralCommentChange(localComment);
        }
        setOpen(false);
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
          disabled={!hasUnsubmitted}
          className="gap-1.5"
        >
          <MessageSquare className="h-3.5 w-3.5" />
          {buttonLabel}
        </Button>
        <Sheet open={open} onOpenChange={(v) => {
          if (!v && localComment !== generalComment) {
            onGeneralCommentChange(localComment);
          }
          setOpen(v);
        }}>
          <SheetContent side="bottom" hideCloseButton className="top-0 flex flex-col px-4 pt-[env(safe-area-inset-top)] pb-[env(safe-area-inset-bottom)]">
            <div className="flex items-center justify-between py-3">
              <SheetTitle className="text-base font-semibold">Finish Review</SheetTitle>
              <Button
                size="sm"
                onClick={handleSubmit}
                disabled={!hasUnsubmitted}
              >
                Submit Review
              </Button>
            </div>
            <div className="flex-1">
              <label className="text-muted-foreground mb-1.5 block text-sm">
                Review Message
              </label>
              <textarea
                value={localComment}
                onChange={(e) => setLocalComment(e.target.value)}
                placeholder="Leave a comment"
                rows={6}
                className="bg-muted border-border w-full rounded-lg border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
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
        disabled={!hasUnsubmitted}
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
            placeholder="Leave a comment"
            rows={4}
            className="bg-background border-border w-full rounded-md border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <div className="mt-3 flex items-center justify-between">
            <Button
              size="sm"
              variant="ghost"
              onClick={() => {
                if (localComment !== generalComment) {
                  onGeneralCommentChange(localComment);
                }
                setOpen(false);
              }}
            >
              Cancel
            </Button>
            <Button
              size="sm"
              onClick={handleSubmit}
              disabled={!hasUnsubmitted}
            >
              Submit review
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
