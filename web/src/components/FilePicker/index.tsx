import { useState, useCallback, useRef, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { Paperclip, Upload, Clipboard, Loader2 } from "lucide-react";
import { useFileUpload } from "@/data/files/mutations";
import { useViewport } from "@/hooks/useViewport";
import { toast } from "sonner";
import { FileBrowser } from "../FileBrowser";

interface FilePickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPick: (paths: string[]) => void;
  searchPath?: string;
}

// ─── Mobile Bottom Bar ──────────────────────────────────────────────────────

function MobileBottomBar({
  onUpload,
  isPending,
  fileInputRef,
  pasteMode,
  onSetPasteMode,
  onPaste,
}: {
  onUpload: (files: File[]) => void;
  isPending: boolean;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  pasteMode: boolean;
  onSetPasteMode: (on: boolean) => void;
  onPaste: (files: File[]) => void;
}) {
  const pasteRef = useRef<HTMLDivElement>(null);

  // Attach selectstart listener to prevent text selection on long-press
  // while keeping the element visible so iOS shows the paste context menu.
  // We do NOT auto-focus — that causes viewport bounce on mobile. The
  // long-press gesture will focus the element naturally.
  useEffect(() => {
    if (!pasteMode) return;
    const el = pasteRef.current;
    if (!el) return;
    const prevent = (e: Event) => e.preventDefault();
    el.addEventListener("selectstart", prevent);
    return () => el.removeEventListener("selectstart", prevent);
  }, [pasteMode]);

  const handlePasteEvent = useCallback(
    (e: React.ClipboardEvent<HTMLDivElement>) => {
      e.preventDefault();
      const items = Array.from(e.clipboardData.items);
      const files: File[] = [];

      for (const item of items) {
        if (item.kind === "file") {
          const file = item.getAsFile();
          if (file) {
            if (file.name === "image.png" || file.name === "") {
              const ext = file.type.split("/")[1] || "png";
              files.push(
                new window.File([file], `clipboard-image.${ext}`, {
                  type: file.type,
                }),
              );
            } else {
              files.push(file);
            }
          }
        }
      }

      if (files.length > 0) {
        onPaste(files);
      } else {
        toast.error("No files found in clipboard");
      }
    },
    [onPaste],
  );

  return (
    <div className="border-border flex shrink-0 gap-2 border-t px-3 py-3">
      <button
        onClick={() => fileInputRef.current?.click()}
        disabled={isPending}
        className={cn(
          "bg-muted text-muted-foreground flex flex-1 items-center justify-center gap-2 rounded-lg py-3 text-sm transition-colors active:bg-accent",
          isPending && "pointer-events-none opacity-50",
        )}
      >
        {isPending ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <Upload className="h-4 w-4" />
        )}
        Upload
      </button>

      {/* Paste button — morphs into a contentEditable target on tap.
          selectstart listener prevents text selection on long-press while
          keeping the element visible so iOS shows the paste context menu.
          inputMode="none" suppresses the virtual keyboard. */}
      <div
        ref={pasteMode ? pasteRef : undefined}
        contentEditable={pasteMode}
        suppressContentEditableWarning
        inputMode={pasteMode ? "none" : undefined}
        className={cn(
          "flex flex-1 items-center justify-center gap-2 rounded-lg py-3 text-sm transition-colors duration-150",
          pasteMode
            ? "bg-primary/10 text-primary"
            : "bg-muted text-muted-foreground active:bg-accent",
          isPending && "pointer-events-none opacity-50",
        )}
        role="button"
        tabIndex={0}
        onClick={() => onSetPasteMode(!pasteMode)}
        onMouseDown={pasteMode ? (e) => e.preventDefault() : undefined}
        onPaste={pasteMode ? handlePasteEvent : undefined}
        onKeyDown={pasteMode ? (e) => e.preventDefault() : (e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onSetPasteMode(true);
          }
        }}
        onInput={pasteMode ? (e) => { (e.target as HTMLElement).textContent = ""; } : undefined}
        aria-label={pasteMode ? "Long-press to paste" : "Paste from clipboard"}
        style={pasteMode ? { caretColor: "transparent" } : undefined}
      >
        <Clipboard className={cn("h-4 w-4 transition-colors duration-150", pasteMode && "text-primary")} />
        {pasteMode ? "Long-press to paste" : "Paste"}
      </div>
    </div>
  );
}

// ─── Main Component ──────────────────────────────────────────────────────────

export function FilePicker({ open, onOpenChange, onPick, searchPath }: FilePickerProps) {
  const { isMobile } = useViewport();
  const uploadMutation = useFileUpload();
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [pasteMode, setPasteMode] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const dragCountRef = useRef(0);

  // Reset state on open
  useEffect(() => {
    if (open) {
      setPasteMode(false);
      setIsDragging(false);
      dragCountRef.current = 0;
    }
  }, [open]);

  const { mutateAsync: uploadFiles } = uploadMutation;
  const openRef = useRef(open);
  useEffect(() => { openRef.current = open; }, [open]);

  const uploadAndPick = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return;
      try {
        const result = await uploadFiles(files);
        // Bail if dialog was closed during upload
        if (!openRef.current) return;
        const paths = result.files.map((f) => f.path);
        for (const file of result.files) {
          toast.success(`Uploaded ${file.name}`);
        }
        onPick(paths);
        onOpenChange(false);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Upload failed");
      }
    },
    [uploadFiles, onPick, onOpenChange],
  );

  const handlePasteFiles = useCallback(
    (files: File[]) => {
      setPasteMode(false);
      uploadAndPick(files);
    },
    [uploadAndPick],
  );

  // Desktop drag-drop handlers (counter-based to handle nested elements)
  const handleDragEnter = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCountRef.current++;
    if (dragCountRef.current === 1) setIsDragging(true);
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    dragCountRef.current = Math.max(0, dragCountRef.current - 1);
    if (dragCountRef.current === 0) setIsDragging(false);
  }, []);

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      dragCountRef.current = 0;
      setIsDragging(false);
      const files = Array.from(e.dataTransfer.files);
      if (files.length > 0) uploadAndPick(files);
    },
    [uploadAndPick],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          "gap-0 overflow-hidden p-0",
          isMobile
            ? "top-[env(safe-area-inset-top)] left-0 right-0 h-[calc(var(--app-height)_-_env(safe-area-inset-top))] max-w-none translate-x-0 translate-y-0 rounded-none border-0"
            : "top-[50%] left-[50%] translate-x-[-50%] translate-y-[-50%] sm:max-w-md",
        )}
        showCloseButton={false}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>Attachments</DialogTitle>
        </DialogHeader>

        <div
          className={cn(
            "relative flex min-h-0 min-w-0 flex-col",
            isMobile && "h-full",
          )}
          onDragEnter={!isMobile ? handleDragEnter : undefined}
          onDragOver={!isMobile ? handleDragOver : undefined}
          onDragLeave={!isMobile ? handleDragLeave : undefined}
          onDrop={!isMobile ? handleDrop : undefined}
        >
          {/* Hidden file input */}
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(e) => {
              if (e.target.files) {
                uploadAndPick(Array.from(e.target.files));
              }
              e.target.value = "";
            }}
          />

          {/* File browser */}
          <FileBrowser
            open={open}
            onSelect={(path) => {
              onPick([path]);
              onOpenChange(false);
            }}
            onClose={() => onOpenChange(false)}
            mode="all"
            placeholder="Search files or type a path..."
            searchPath={searchPath}
            headerExtra={!isMobile ? (
              <button
                onClick={() => fileInputRef.current?.click()}
                disabled={uploadMutation.isPending}
                className={cn(
                  "text-muted-foreground hover:text-foreground flex-shrink-0 rounded-md p-1.5 transition-colors",
                  uploadMutation.isPending && "pointer-events-none opacity-50",
                )}
                aria-label="Upload files"
              >
                {uploadMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Upload className="h-4 w-4" />
                )}
              </button>
            ) : undefined}
          />

          {/* Mobile upload/paste bar — pinned to bottom */}
          {isMobile && (
            <MobileBottomBar
              onUpload={uploadAndPick}
              isPending={uploadMutation.isPending}
              fileInputRef={fileInputRef}
              pasteMode={pasteMode}
              onSetPasteMode={setPasteMode}
              onPaste={handlePasteFiles}
            />
          )}

          {/* Desktop drag-drop overlay */}
          {!isMobile && isDragging && (
            <div className="border-primary/50 bg-background/80 absolute inset-0 z-10 flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed backdrop-blur-sm">
              <Paperclip className="text-primary h-8 w-8" />
              <span className="text-foreground text-sm font-medium">Drop files to upload</span>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
