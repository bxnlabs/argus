import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PROVIDER_OPTIONS } from "@/types";
import type { ProviderType } from "@/types";

interface ProviderSelectorProps {
  value: ProviderType;
  onChange: (value: ProviderType) => void;
}

export function ProviderSelector({ value, onChange }: ProviderSelectorProps) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">Provider</label>
      <Select value={value} onValueChange={(v) => onChange(v as ProviderType)}>
        <SelectTrigger>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PROVIDER_OPTIONS.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              <span className="font-medium">{option.label}</span>
              <span className="text-muted-foreground ml-2 text-xs">
                {option.description}
              </span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
