import { Container } from "lucide-react";
import { cn } from "@/lib/utils";

interface DockerizedBadgeProps {
  className?: string;
}

// DockerizedBadge marks a profile whose sessions run inside a docker compose
// stack. Shown next to the profile name in the profile selectors and the
// session info dialog.
export function DockerizedBadge({ className }: DockerizedBadgeProps) {
  return (
    <Container
      role="img"
      aria-label="dockerized"
      className={cn("text-muted-foreground h-3.5 w-3.5", className)}
    />
  );
}
