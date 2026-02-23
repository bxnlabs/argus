import { UnifiedDiff } from "./UnifiedDiff";
import { parseDiff, getDiffFileName } from "@/lib/diff-parser";

interface DiffViewProps {
  diff: string;
  fileName?: string;
}

export function DiffView({ diff, fileName }: DiffViewProps) {
  if (!diff) {
    return (
      <div className="text-muted-foreground p-4 text-center">
        <p className="text-sm">No changes to display</p>
      </div>
    );
  }

  const parsedDiff = parseDiff(diff);
  const displayName = fileName || getDiffFileName(parsedDiff);

  return <UnifiedDiff diff={parsedDiff} fileName={displayName} expanded />;
}
