import { cn } from "@/lib/utils";

export type GitTab = "changes" | "history" | "compare";

/**
 * A request to open a specific git sub-tab, modeled as an event rather than a
 * value. `seq` makes each request distinct so repeating the same chord (e.g.
 * `g h`) re-navigates even after the user manually switched tabs — identical
 * `tab` values alone would dedupe and silently no-op.
 */
export type GitTabRequest = { tab: GitTab; seq: number };

interface GitPanelTabsProps {
  activeTab: GitTab;
  onTabChange: (tab: GitTab) => void;
}

export function GitPanelTabs({ activeTab, onTabChange }: GitPanelTabsProps) {
  return (
    <div role="tablist" className="border-border/50 flex border-b">
      <button
        role="tab"
        aria-selected={activeTab === "changes"}
        onClick={() => onTabChange("changes")}
        className={cn(
          "flex-1 px-3 py-1.5 text-sm font-medium transition-colors",
          activeTab === "changes"
            ? "text-foreground border-primary border-b-2"
            : "text-muted-foreground hover:text-foreground",
        )}
      >
        Changes
      </button>
      <button
        role="tab"
        aria-selected={activeTab === "history"}
        onClick={() => onTabChange("history")}
        className={cn(
          "flex-1 px-3 py-1.5 text-sm font-medium transition-colors",
          activeTab === "history"
            ? "text-foreground border-primary border-b-2"
            : "text-muted-foreground hover:text-foreground",
        )}
      >
        History
      </button>
      <button
        role="tab"
        aria-selected={activeTab === "compare"}
        onClick={() => onTabChange("compare")}
        className={cn(
          "flex-1 px-3 py-1.5 text-sm font-medium transition-colors",
          activeTab === "compare"
            ? "text-foreground border-primary border-b-2"
            : "text-muted-foreground hover:text-foreground",
        )}
      >
        Compare
      </button>
    </div>
  );
}
