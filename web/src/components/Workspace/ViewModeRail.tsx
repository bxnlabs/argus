import {
  Terminal as TerminalIcon,
  GitBranch,
  FilePenLine,
  Paperclip,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { isMac } from "@/lib/device";
import type { SidePanel } from "@/components/views/types";

interface ViewModeRailProps {
  activePanel: SidePanel;
  onSetActivePanel: (panel: SidePanel) => void;
  isGitEnabled: boolean;
  isEditorEnabled: boolean;
  onAttachments?: () => void;
}

function RailButton({
  active,
  disabled,
  onClick,
  tooltip,
  children,
}: {
  active?: boolean;
  disabled?: boolean;
  onClick: () => void;
  tooltip: string;
  children: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          onClick={disabled ? undefined : onClick}
          disabled={disabled}
          className={cn(
            "relative h-12 w-12",
            active && "bg-accent"
          )}
          aria-label={tooltip}
          aria-pressed={active}
        >
          {/* Active view mode indicator pill — anchored to right border */}
          {active && (
            <span aria-hidden="true" className="bg-primary absolute right-0 top-0 h-full w-1 rounded-full" />
          )}
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent side="left">{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export function ViewModeRail({
  activePanel,
  onSetActivePanel,
  isGitEnabled,
  isEditorEnabled,
  onAttachments,
}: ViewModeRailProps) {
  const isTerminalActive = activePanel === null;
  const isGitActive = activePanel === "git";
  const isEditorActive = activePanel === "editor";
  const leader = isMac() ? "⌘ ;" : "Ctrl ;";

  return (
    <nav
      className="flex w-14 shrink-0 flex-col items-center gap-1 pt-1"
      aria-label="View mode"
    >
      {/* View mode icons */}
      <RailButton
        active={isTerminalActive}
        onClick={() => onSetActivePanel(null)}
        tooltip={`Terminal (${leader} T)`}
      >
        <TerminalIcon className={cn("h-6 w-6", isTerminalActive && "text-primary")} />
      </RailButton>

      <RailButton
        active={isGitActive}
        disabled={!isGitEnabled}
        onClick={() => onSetActivePanel("git")}
        tooltip={`Git (${leader} G)`}
      >
        <GitBranch className={cn("h-6 w-6", isGitActive && "text-primary")} />
      </RailButton>

      <RailButton
        active={isEditorActive}
        disabled={!isEditorEnabled}
        onClick={() => onSetActivePanel("editor")}
        tooltip={`Editor (${leader} E)`}
      >
        <FilePenLine className={cn("h-6 w-6", isEditorActive && "text-primary")} />
      </RailButton>

      {/* Divider */}
      {onAttachments && <div className="border-border my-1 w-6 border-t" />}

      {/* Attachments */}
      {onAttachments && (
        <RailButton onClick={onAttachments} tooltip="Attachments">
          <Paperclip className="h-6 w-6" />
        </RailButton>
      )}
    </nav>
  );
}
