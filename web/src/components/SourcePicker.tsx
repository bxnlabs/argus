import { useCallback, useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn, contractTilde } from "@/lib/utils";
import { useFilesQuery } from "@/data/files";
import { useViewport } from "@/hooks/useViewport";
import { FileBrowser } from "./FileBrowser";
import { RepoBrowser } from "./RepoBrowser";

type SourceTab = "local" | "remote";

interface SourcePickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (source: string, tab: SourceTab) => void;
  initialTab?: SourceTab;
  initialLocalPath?: string;
  initialRemoteQuery?: string;
}

export function SourcePicker({
  open,
  onOpenChange,
  onSelect,
  initialTab = "local",
  initialLocalPath = "~",
  initialRemoteQuery = "",
}: SourcePickerProps) {
  const { isMobile } = useViewport();
  const [tab, setTab] = useState<SourceTab>(initialTab);
  const [homePath, setHomePath] = useState("");

  const homeQuery = useFilesQuery("~", { enabled: !homePath });
  useEffect(() => {
    if (homeQuery.data?.path && !homePath) {
      setHomePath(homeQuery.data.path);
    }
  }, [homeQuery.data, homePath]);

  // Sync tab from props when opening
  useEffect(() => {
    if (open) {
      setTab(initialTab);
    }
  }, [open, initialTab]);

  const handleLocalSelect = useCallback(
    (absolutePath: string) => {
      const contracted = homePath
        ? contractTilde(absolutePath, homePath)
        : absolutePath;
      onSelect(contracted, "local");
      onOpenChange(false);
    },
    [homePath, onSelect, onOpenChange],
  );

  const handleRemoteSelect = useCallback(
    (repo: string) => {
      onSelect(repo, "remote");
      onOpenChange(false);
    },
    [onSelect, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          "gap-0 overflow-hidden p-0",
          isMobile
            ? "top-[env(safe-area-inset-top)] left-0 right-0 h-[calc(var(--app-height)_-_env(safe-area-inset-top))] flex flex-col max-w-none translate-x-0 translate-y-0 rounded-none border-0"
            : "top-[50%] left-[50%] translate-x-[-50%] translate-y-[-50%] sm:max-w-md",
        )}
        showCloseButton={false}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>Select Source</DialogTitle>
        </DialogHeader>

        {/* Tab bar */}
        <div className="border-border flex border-b">
          <button
            type="button"
            onClick={() => setTab("local")}
            className={cn(
              "flex-1 px-4 py-2 text-sm font-medium transition-colors",
              tab === "local"
                ? "border-b-2 border-foreground text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            Local
          </button>
          <button
            type="button"
            onClick={() => setTab("remote")}
            className={cn(
              "flex-1 px-4 py-2 text-sm font-medium transition-colors",
              tab === "remote"
                ? "border-b-2 border-foreground text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            Remote
          </button>
        </div>

        {/* Tab content */}
        {tab === "local" ? (
          <FileBrowser
            open={open && tab === "local"}
            onSelect={handleLocalSelect}
            onClose={() => onOpenChange(false)}
            mode="directory"
            placeholder="Search folders or type a path..."
            initialQuery={initialLocalPath === "~" ? "" : initialLocalPath}
          />
        ) : (
          <RepoBrowser
            open={open && tab === "remote"}
            onSelect={handleRemoteSelect}
            onClose={() => onOpenChange(false)}
            placeholder="Search repos or enter a URL..."
            initialQuery={initialRemoteQuery}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
