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
import {
  getStatusAnimation,
  getStatusColor,
  getStatusLabel,
} from "@/lib/sessionStatus";
import { cn } from "@/lib/utils";
import type { Session } from "@/types";
import { buildSessionInfoModel } from "./fields";
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
  const model = session
    ? buildSessionInfoModel(session, status, homeDir)
    : null;

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
        {model && (
          <>
            <DialogHeader className="text-left">
              <div className="mb-1.5 flex items-center gap-1.5">
                <ProviderBadge type={model.providerType} />
                {model.autoApprove && (
                  <Badge
                    variant="outline"
                    className="border-current px-1 py-0 text-[10px] font-medium text-yellow-500"
                  >
                    YOLO
                  </Badge>
                )}
              </div>
              <DialogTitle asChild>
                <div className="min-w-0 truncate">{model.name}</div>
              </DialogTitle>
              <DialogDescription asChild>
                <div className="flex items-center gap-1.5">
                  <span
                    className={cn(
                      "h-1.5 w-1.5 flex-shrink-0 rounded-full",
                      getStatusColor(model.status),
                      getStatusAnimation(model.status),
                    )}
                  />
                  <span>
                    {getStatusLabel(model.status) || "Unknown"}
                    {" · "}
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span
                          tabIndex={0}
                          className="cursor-default rounded-sm underline decoration-dotted underline-offset-2 outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px]"
                        >
                          {model.updatedRelative}
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>
                        <div>Created: {model.createdAbsolute}</div>
                        <div>Updated: {model.updatedAbsolute}</div>
                      </TooltipContent>
                    </Tooltip>
                    {model.profile ? ` · ${model.profile}` : ""}
                  </span>
                </div>
              </DialogDescription>
            </DialogHeader>

            <div className="space-y-4">
              <div className="space-y-1">
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  Details
                </div>
                <CopyableField
                  label="ID"
                  displayValue={model.details.id}
                  inline
                />
                {model.details.model && (
                  <div className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground w-20 flex-shrink-0">
                      Model
                    </span>
                    <span className="min-w-0 flex-1 truncate">
                      {model.details.model}
                    </span>
                  </div>
                )}
              </div>

              <div className="space-y-2">
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  Location
                </div>
                <CopyableField
                  label="Directory"
                  displayValue={model.location.directory.display}
                  copyValue={model.location.directory.copy}
                />
                {model.location.repo && (
                  <CopyableField
                    label="Repo"
                    displayValue={model.location.repo}
                  />
                )}
                {model.location.branch && (
                  <CopyableField
                    label="Branch"
                    displayValue={model.location.branch}
                  />
                )}
                {model.location.worktreeDir && (
                  <CopyableField
                    label="Worktree dir"
                    displayValue={model.location.worktreeDir.display}
                    copyValue={model.location.worktreeDir.copy}
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
