import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { useProfilesQuery } from "@/data/sessions";
import type { Session } from "@/types";

// Sentinel value for the "no profile" option, since Radix Select cannot use "".
const NONE_VALUE = "__none__";

interface ChangeProfileDialogProps {
  session: Session | null;
  onClose: () => void;
  onApply: (sessionId: string, profile: string | null) => void;
}

export function ChangeProfileDialog({
  session,
  onClose,
  onApply,
}: ChangeProfileDialogProps) {
  const { data: profilesData } = useProfilesQuery();
  const profiles = profilesData?.profiles ?? [];

  const currentValue = session?.profile ?? NONE_VALUE;
  const [selected, setSelected] = useState(currentValue);

  // Reset the selection whenever a new session is targeted.
  useEffect(() => {
    setSelected(session?.profile ?? NONE_VALUE);
  }, [session]);

  const handleApply = () => {
    if (!session) return;
    const profile = selected === NONE_VALUE ? null : selected;
    onApply(session.id, profile);
    onClose();
  };

  const unchanged = selected === currentValue;

  return (
    <Dialog open={session !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Change Profile</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Profile</label>
            <Select value={selected} onValueChange={setSelected}>
              <SelectTrigger>
                <SelectValue placeholder="Select a profile..." />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE_VALUE}>None (detach)</SelectItem>
                {profiles.map((p) => (
                  <SelectItem key={p} value={p}>
                    {p}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <p className="text-muted-foreground text-xs">
            Changing the profile restarts this session; your conversation is
            preserved.
          </p>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button type="button" onClick={handleApply} disabled={unchanged}>
            Apply
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
