import { cn } from "@/lib/utils";

interface DockerizedBadgeProps {
  className?: string;
}

// DockerizedBadge marks a profile whose sessions run inside a docker compose
// stack. Shown next to the profile name in the profile selectors and the
// session info dialog.
export function DockerizedBadge({ className }: DockerizedBadgeProps) {
  return (
    <span
      role="img"
      aria-label="dockerized"
      className={cn("text-muted-foreground text-xs", className)}
    >
      🐳
    </span>
  );
}
