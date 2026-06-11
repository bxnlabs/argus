import { ServerOff, PanelLeft } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * Shown in the workspace area when the active node is unreachable, so an offline
 * node reads as "down" rather than as an empty workspace. The rail stays visible
 * (this replaces only the main content), so the user can switch to another node.
 *
 * On mobile the rail/sidebar live behind a hamburger that normally sits in the
 * workspace this screen replaces, so `onMenuClick` (passed only on mobile) keeps
 * that trigger present — otherwise an offline active node would trap the user
 * with no way to switch away.
 */
export function NodeOffline({
  name,
  onMenuClick,
}: {
  name: string;
  onMenuClick?: () => void;
}) {
  return (
    <div className="relative flex h-full flex-col items-center justify-center gap-3 p-8 text-center">
      {onMenuClick && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onMenuClick}
          aria-label="Open menu"
          className="absolute left-2 top-2 h-8 w-8"
        >
          <PanelLeft className="h-4 w-4" />
        </Button>
      )}
      <ServerOff className="text-muted-foreground h-10 w-10" />
      <div className="space-y-1">
        <p className="text-lg font-medium">{name} is unreachable</p>
        <p className="text-muted-foreground max-w-sm text-sm">
          This node isn&rsquo;t responding. Check that it&rsquo;s running and
          reachable, or switch to another node.
        </p>
      </div>
    </div>
  );
}
