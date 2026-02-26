import { Button } from "@/components/ui/button";
import {
  Terminal as TerminalIcon,
  GitBranch,
  FilePenLine,
  Paperclip,
} from "lucide-react";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { SidePanel } from "@/components/views/types";

interface RightSidebarProps {
  activePanel: SidePanel;
  onSetActivePanel: (panel: SidePanel) => void;
  isGitEnabled: boolean;
  isEditorEnabled: boolean;
  onAttachments?: () => void;
}

function SidebarButton({
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
    <div className="relative">
      {/* Active indicator pill — anchored to the right edge */}
      {active && (
        <div className="bg-primary absolute -right-1 top-1/2 h-2.5 w-1 -translate-y-1/2 rounded-full" />
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onClick}
            disabled={disabled}
            className="h-10 w-10"
            aria-label={tooltip}
          >
            {children}
          </Button>
        </TooltipTrigger>
        <TooltipContent side="left">{tooltip}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function RightSidebar({
  activePanel,
  onSetActivePanel,
  isGitEnabled,
  isEditorEnabled,
  onAttachments,
}: RightSidebarProps) {
  const isTerminalActive = activePanel === null;
  const isGitActive = activePanel === "git";
  const isEditorActive = activePanel === "editor";

  return (
    <div className="border-border flex shrink-0 flex-col items-center gap-1 border-l px-1 py-1">
      {/* View mode icons */}
      <SidebarButton
        active={isTerminalActive}
        onClick={() => onSetActivePanel(null)}
        tooltip="Terminal"
      >
        <TerminalIcon
          className={cn("h-5 w-5", isTerminalActive && "text-primary")}
        />
      </SidebarButton>

      <SidebarButton
        active={isGitActive}
        disabled={!isGitEnabled}
        onClick={() => onSetActivePanel("git")}
        tooltip="Git"
      >
        <GitBranch
          className={cn("h-5 w-5", isGitActive && "text-primary")}
        />
      </SidebarButton>

      <SidebarButton
        active={isEditorActive}
        disabled={!isEditorEnabled}
        onClick={() => onSetActivePanel("editor")}
        tooltip="Editor"
      >
        <FilePenLine
          className={cn("h-5 w-5", isEditorActive && "text-primary")}
        />
      </SidebarButton>

      {/* Divider */}
      <div className="border-border my-1 w-6 border-t" />

      {/* Attachments */}
      {onAttachments && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={onAttachments}
              className="h-10 w-10"
              aria-label="Attachments"
            >
              <Paperclip className="h-5 w-5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="left">Attachments</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}
