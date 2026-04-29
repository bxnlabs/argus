import { useState } from "react";
import { ChevronRight, FileWarning } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ReviewComment } from "@/types";

export interface OutOfDiffFileGroup {
  /** Stable scroll-target key, typically the file path (or {file, oldPath} composite). */
  key: string;
  /** User-facing file label; for renames, show both paths. */
  displayFile: string;
  /** Comments belonging to this file, preserving Review order. */
  comments: ReviewComment[];
}

interface Props {
  groups: OutOfDiffFileGroup[];
  onFileClick: (key: string) => void;
  selectedKey?: string;
}

export function OutOfDiffSection({ groups, onFileClick, selectedKey }: Props) {
  const [expanded, setExpanded] = useState(true);
  if (groups.length === 0) return null;

  const totalComments = groups.reduce((n, g) => n + g.comments.length, 0);

  return (
    <div className="mb-4">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          onClick={() => setExpanded((v) => !v)}
          className="text-muted-foreground hover:text-foreground flex items-center gap-2 text-sm font-medium transition-colors"
        >
          <ChevronRight
            className={cn("h-4 w-4 transition-transform", expanded && "rotate-90")}
          />
          <FileWarning className="h-4 w-4" />
          <span>Out-of-diff comments</span>
        </button>
        <span className="bg-muted ml-auto rounded-full px-2 py-0.5 text-xs">
          {totalComments}
        </span>
      </div>
      {expanded && (
        <div className="space-y-0.5">
          {groups.map((g) => (
            <button
              key={g.key}
              onClick={() => onFileClick(g.key)}
              className={cn(
                "hover:bg-muted/70 flex min-h-[44px] w-full items-center gap-2 px-3 py-1.5 text-left transition-colors",
                selectedKey === g.key && "bg-primary/10 hover:bg-primary/20",
              )}
            >
              <span className="flex-1 truncate text-xs font-medium">{g.displayFile}</span>
              <span className="text-muted-foreground flex-shrink-0 text-xs">
                {g.comments.length}
              </span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
