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

interface DirectoryPickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (path: string) => void; // returns tilde-contracted path
  initialPath?: string;
}

export function DirectoryPicker({
  open,
  onOpenChange,
  onSelect,
  initialPath = "~",
}: DirectoryPickerProps) {
  const { isMobile } = useViewport();
  const [homePath, setHomePath] = useState("");

  const homeQuery = useFilesQuery("~", { enabled: !homePath });
  useEffect(() => {
    if (homeQuery.data?.path && !homePath) {
      setHomePath(homeQuery.data.path);
    }
  }, [homeQuery.data, homePath]);

  const handleSelect = useCallback(
    (absolutePath: string) => {
      const contracted = homePath
        ? contractTilde(absolutePath, homePath)
        : absolutePath;
      onSelect(contracted);
      onOpenChange(false);
    },
    [homePath, onSelect, onOpenChange],
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
          <DialogTitle>Select Folder</DialogTitle>
        </DialogHeader>

        <FileBrowser
          open={open}
          onSelect={handleSelect}
          onClose={() => onOpenChange(false)}
          mode="directory"
          placeholder="Search folders or type a path..."
          initialQuery={initialPath === "~" ? "" : initialPath}
        />
      </DialogContent>
    </Dialog>
  );
}
