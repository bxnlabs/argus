import { Badge } from "@/components/ui/badge";
import { ProviderLogo, providerLabel } from "@/components/ProviderLogo";
import type { ProviderType } from "@/types";
import { cn } from "@/lib/utils";

interface ProviderBadgeProps {
  type: ProviderType;
  className?: string;
}

// ProviderBadge shows a provider's brand mark and name as a single chip.
export function ProviderBadge({ type, className }: ProviderBadgeProps) {
  return (
    <Badge variant="secondary" className={cn("gap-1 font-medium", className)}>
      <ProviderLogo type={type} className="h-3 w-3" />
      {providerLabel(type)}
    </Badge>
  );
}
