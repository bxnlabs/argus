import { GitBranch, RefreshCw, ArrowUp, ArrowDown } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useGitStatusSummaryQuery } from "@/data/git";

interface GitStatusHeaderProps {
  workingDirectory: string;
  onRefresh: () => void;
  isFetching?: boolean;
}

export function GitStatusHeader({ workingDirectory, onRefresh, isFetching = false }: GitStatusHeaderProps) {
  const {
    data: summary,
    isRefetching,
  } = useGitStatusSummaryQuery(workingDirectory);

  const branch = summary?.branch ?? "";
  const ahead = summary?.ahead ?? 0;
  const behind = summary?.behind ?? 0;
  const busy = isRefetching || isFetching;

  return (
    <div className="flex items-center gap-2 px-3 py-1.5">
      <GitBranch className="text-muted-foreground h-4 w-4 flex-shrink-0" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          {branch || "Git Status"}
        </p>
        {(ahead > 0 || behind > 0) && (
          <div className="text-muted-foreground flex items-center gap-2 text-xs">
            {ahead > 0 && (
              <span className="flex items-center gap-0.5">
                <ArrowUp className="h-3 w-3" />
                {ahead}
              </span>
            )}
            {behind > 0 && (
              <span className="flex items-center gap-0.5">
                <ArrowDown className="h-3 w-3" />
                {behind}
              </span>
            )}
          </div>
        )}
      </div>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={onRefresh}
        disabled={busy}
        className="h-6 w-6"
      >
        <RefreshCw className={`h-4 w-4 ${busy ? "animate-spin" : ""}`} />
      </Button>
    </div>
  );
}
