import { useState, useEffect } from "react";
import { ChevronDown, ChevronRight, Plus, Minus } from "lucide-react";
import { cn } from "@/lib/utils";
import type { ParsedDiff, DiffHunk, DiffLine } from "@/lib/diff-parser";

interface UnifiedDiffProps {
  diff: ParsedDiff;
  fileName: string;
  expanded?: boolean;
  onToggle?: () => void;
}

export function UnifiedDiff({
  diff,
  fileName,
  expanded = true,
  onToggle,
}: UnifiedDiffProps) {
  const [localExpanded, setLocalExpanded] = useState(expanded);
  useEffect(() => {
    if (!onToggle) setLocalExpanded(expanded);
  }, [expanded, onToggle]);
  const isExpanded = onToggle ? expanded : localExpanded;

  const handleToggle = () => {
    if (onToggle) {
      onToggle();
    } else {
      setLocalExpanded(!localExpanded);
    }
  };

  return (
    <div className="border-border overflow-hidden rounded-lg border">
      {/* File header */}
      <button
        onClick={handleToggle}
        className={cn(
          "flex w-full items-center gap-2 px-3 py-2.5 text-sm",
          "bg-muted/50 hover:bg-muted text-left transition-colors",
          "min-h-[44px]",
        )}
      >
        {isExpanded ? (
          <ChevronDown className="text-muted-foreground h-4 w-4 flex-shrink-0" />
        ) : (
          <ChevronRight className="text-muted-foreground h-4 w-4 flex-shrink-0" />
        )}

        <span className="flex-1 truncate font-mono text-xs">{fileName}</span>

        <span className="flex flex-shrink-0 items-center gap-2 text-xs">
          {diff.additions > 0 && (
            <span className="flex items-center gap-0.5 text-green-500">
              <Plus className="h-3 w-3" />
              {diff.additions}
            </span>
          )}
          {diff.deletions > 0 && (
            <span className="flex items-center gap-0.5 text-red-500">
              <Minus className="h-3 w-3" />
              {diff.deletions}
            </span>
          )}
        </span>
      </button>

      {isExpanded && (
        <div className="overflow-x-auto">
          {diff.isBinary ? (
            <div className="text-muted-foreground px-4 py-8 text-center text-sm">
              Binary file not shown
            </div>
          ) : diff.hunks.length === 0 ? (
            <div className="text-muted-foreground px-4 py-8 text-center text-sm">
              No changes
            </div>
          ) : (
            <div className="w-fit min-w-full font-mono text-xs">
              {diff.hunks.map((hunk, index) => (
                <Hunk key={index} hunk={hunk} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function Hunk({ hunk }: { hunk: DiffHunk }) {
  return (
    <div className="min-w-full">
      <div className="border-border border-y bg-blue-500/10 px-3 py-1 text-xs text-blue-400">
        {hunk.header}
      </div>
      <table className="min-w-full border-collapse">
        <tbody>
          {hunk.lines.map((line, index) => (
            <DiffLineRow key={index} line={line} />
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DiffLineRow({ line }: { line: DiffLine }) {
  if (line.type === "header") return null;

  const bgColor =
    line.type === "addition"
      ? "bg-green-500/10"
      : line.type === "deletion"
        ? "bg-red-500/10"
        : "";

  const textColor =
    line.type === "addition"
      ? "text-green-400"
      : line.type === "deletion"
        ? "text-red-400"
        : "text-foreground";

  const marker =
    line.type === "addition" ? "+" : line.type === "deletion" ? "-" : "";

  return (
    <tr className={cn("hover:bg-muted/30", bgColor)}>
      <td className="text-muted-foreground border-border/50 w-12 border-r px-2 py-0.5 text-right tabular-nums select-none">
        {line.oldLineNumber ?? ""}
      </td>
      <td className="text-muted-foreground border-border/50 w-12 border-r px-2 py-0.5 text-right tabular-nums select-none">
        {line.newLineNumber ?? ""}
      </td>
      <td className={cn("w-6 px-1 py-0.5 text-center select-none", textColor)}>
        {marker}
      </td>
      <td className={cn("px-2 py-0.5 whitespace-pre", textColor)}>
        {line.content || " "}
      </td>
    </tr>
  );
}
