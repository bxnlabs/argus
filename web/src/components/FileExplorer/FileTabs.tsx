import { useRef, useEffect } from "react";
import { X, File } from "lucide-react";
import { cn } from "@/lib/utils";

interface FileTabsProps {
  paths: string[];
  activeFilePath: string | null;
  onSelect: (path: string) => void;
  onClose: (path: string) => void;
}

export function FileTabs({
  paths,
  activeFilePath,
  onSelect,
  onClose,
}: FileTabsProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const activeTabRef = useRef<HTMLDivElement>(null);

  // Scroll active tab into view
  useEffect(() => {
    if (activeTabRef.current && scrollRef.current) {
      activeTabRef.current.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
        inline: "center",
      });
    }
  }, [activeFilePath]);

  if (paths.length === 0) return null;

  return (
    <div
      ref={scrollRef}
      className="bg-muted/30 scrollbar-none flex items-center gap-0.5 overflow-x-auto px-1"
    >
      {paths.map((path) => {
        const isActive = path === activeFilePath;
        const fileName = path.split("/").pop() || path;

        return (
          <div
            key={path}
            ref={isActive ? activeTabRef : null}
            role="button"
            tabIndex={0}
            onClick={() => onSelect(path)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onSelect(path);
              }
            }}
            className={cn(
              "group flex cursor-pointer items-center gap-1.5 px-3 py-2 text-sm whitespace-nowrap transition-colors",
              "min-h-[40px] md:min-h-[36px]",
              "hover:bg-accent/50",
              isActive
                ? "bg-background text-foreground"
                : "text-muted-foreground",
            )}
          >
            <File className="text-muted-foreground h-3.5 w-3.5 flex-shrink-0" />
            <span className="max-w-[120px] truncate">{fileName}</span>
            <button
              onClick={(e) => {
                e.stopPropagation();
                onClose(path);
              }}
              className={cn(
                "hover:bg-accent ml-1 flex-shrink-0 rounded p-0.5",
                "opacity-0 group-hover:opacity-100",
                isActive && "opacity-100",
              )}
            >
              <X className="h-3 w-3" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
