import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { ProviderBadge } from "@/components/ProviderBadge";
import { Badge } from "@/components/ui/badge";
import { getStatusMeta } from "@/lib/sessionStatus";
import { cn, formatRelativeTime } from "@/lib/utils";
import type { Session } from "@/types";
import { getSessionLocation } from "./fields";
import { CopyableField } from "./CopyableField";

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
  const location = session ? getSessionLocation(session, homeDir) : null;
  const statusMeta = getStatusMeta(status);

  return (
    <Dialog open={session !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        tabIndex={-1}
        onOpenAutoFocus={(event) => {
          // Focus the dialog, not its first focusable child (the timestamp
          // caption), so that caption's tooltip stays closed until hover/focus.
          event.preventDefault();
          (event.currentTarget as HTMLElement).focus();
        }}
      >
        {session && location && (
          <>
            <DialogHeader className="text-left">
              <div className="mb-1.5 flex items-center gap-1.5">
                <ProviderBadge type={session.provider_type} />
                {session.auto_approve && (
                  <Badge
                    variant="outline"
                    className="border-current px-1 py-0 text-[10px] font-medium text-yellow-500"
                  >
                    YOLO
                  </Badge>
                )}
              </div>
              <DialogTitle className="min-w-0 truncate">
                {session.name || "Session"}
              </DialogTitle>
              <DialogDescription asChild>
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                      statusMeta.color,
                      statusMeta.animation,
                    )}
                  />
                  <span>
                    {statusMeta.label || "Unknown"}
                    {" · "}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span
                          tabIndex={0}
                          className="cursor-default rounded-sm underline decoration-dotted underline-offset-2 outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px]"
                        >
                          {formatRelativeTime(session.updated_at)}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        <div>Created: {session.created_at}</div>
                        <div>Updated: {session.updated_at}</div>
                      </TooltipContent>
                    </Tooltip>
                  </span>
                </div>
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-2">
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  Details
                </div>
                <CopyableField label="ID" displayValue={session.id} />
                {session.profile && (
                  <CopyableField label="Profile" displayValue={session.profile} />
                )}
                {session.model && (
                  <CopyableField label="Model" displayValue={session.model} />
                )}
              </div>

              <div className="space-y-2">
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  Location
                </div>
                <CopyableField
                  label="Directory"
                  displayValue={location.directory.display}
                  copyValue={location.directory.copy}
                />
                {location.repo && (
                  <CopyableField label="Repo" displayValue={location.repo} />
                )}
                {location.branch && (
                  <CopyableField label="Branch" displayValue={location.branch} />
                )}
                {location.worktreeDir && (
                  <CopyableField
                    label="Worktree dir"
                    displayValue={location.worktreeDir.display}
                    copyValue={location.worktreeDir.copy}
                  />
                )}
              </div>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}
