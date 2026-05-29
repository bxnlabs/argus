import { Badge } from "@/components/ui/badge";
import { ProviderLogo, providerLabel, providerColor } from "@/components/ProviderLogo";
import type { ProviderType } from "@/types";
import { cn } from "@/lib/utils";

interface ProviderBadgeProps {
  type: ProviderType;
  className?: string;
}

// ProviderBadge shows a provider's brand mark and name as a compact outlined
// chip tinted with the provider's brand color (border + text + icon). Codex and
// shell have no literal brand color, so they fall back to the foreground and
// muted-foreground colors respectively.
export function ProviderBadge({ type, className }: ProviderBadgeProps) {
  const brand = providerColor(type);
  const hex = brand && brand !== "currentColor" ? brand : undefined;
  return (
    <Badge
      variant="outline"
      className={cn(
        "gap-1 border-current px-1 py-0 text-[10px] font-medium",
        !hex && type === "shell" && "text-muted-foreground",
        className,
      )}
      style={hex ? { color: hex } : undefined}
    >
      <ProviderLogo type={type} className="h-3 w-3" decorative />
      {providerLabel(type)}
    </Badge>
  );
}
