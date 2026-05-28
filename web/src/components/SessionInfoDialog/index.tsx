import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import type { Session } from "@/types";
import { buildSessionInfoSections } from "./fields";

interface SessionInfoDialogProps {
  session: Session | null;
  status?: string;
  homeDir: string;
  onClose: () => void;
}

export function SessionInfoDialog({
  session,
  status,
  homeDir,
  onClose,
}: SessionInfoDialogProps) {
  const sections = session
    ? buildSessionInfoSections(session, status, homeDir)
    : [];

  return (
    <Dialog open={session !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="truncate">
            {session?.name || "Session"}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {sections.map((section, i) => (
            <div key={section.title ?? `section-${i}`} className="space-y-1">
              {section.title && (
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  {section.title}
                </div>
              )}
              <dl className="space-y-1">
                {section.rows.map((row) => (
                  <div key={row.label} className="flex gap-2 text-sm">
                    <dt className="text-muted-foreground w-28 flex-shrink-0">
                      {row.label}
                    </dt>
                    <dd className="min-w-0 break-all">{row.value}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
