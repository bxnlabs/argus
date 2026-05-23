import { Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import { clearMenuItems, type ClearCategory } from "@/lib/compare-comments";

interface ClearCommentsMenuProps {
  counts: Record<ClearCategory, number>;
  onClear: (category: ClearCategory) => void;
}

/**
 * Toolbar dropdown for bulk-clearing review comments by category. Purely
 * presentational: the row model (labels, counts, disabled state) comes from
 * `clearMenuItems`, and the destructive confirm lives in the caller's `onClear`.
 */
export function ClearCommentsMenu({ counts, onClear }: ClearCommentsMenuProps) {
  const items = clearMenuItems(counts);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label="Clear comments"
          className="text-destructive border-destructive/40 hover:bg-destructive hover:text-white hover:border-destructive dark:hover:bg-destructive/70"
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {items.map((item) => (
          <DropdownMenuItem
            key={item.category}
            disabled={item.disabled}
            onClick={() => onClear(item.category)}
          >
            <span className="flex-1">{item.label}</span>
            <span className="text-muted-foreground ml-3 tabular-nums">
              {item.count}
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
