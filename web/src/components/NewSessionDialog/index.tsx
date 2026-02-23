import { useState, useEffect, useRef } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { AgentSelector } from "./AgentSelector";
import { DirectoryPicker } from "@/components/DirectoryPicker";
import { FolderOpen } from "lucide-react";
import type { AgentType, CreateSessionParams } from "@/types";

interface NewSessionDialogProps {
  open: boolean;
  onClose: () => void;
  onCreateSession: (params: CreateSessionParams) => void;
}

export function NewSessionDialog({
  open,
  onClose,
  onCreateSession,
}: NewSessionDialogProps) {
  const [name, setName] = useState("");
  const [agentType, setAgentType] = useState<AgentType>("claude");
  const [workingDirectory, setWorkingDirectory] = useState("");
  const [autoApprove, setAutoApprove] = useState(true);
  const [showDirectoryPicker, setShowDirectoryPicker] = useState(false);
  const directoryPickerClosingRef = useRef(false);

  // Reset form when dialog opens
  useEffect(() => {
    if (open) {
      setName("");
      setAgentType("claude");
      setWorkingDirectory("");
      setAutoApprove(true);
      setShowDirectoryPicker(false);
    }
  }, [open]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const params: CreateSessionParams = {
      agent_type: agentType,
    };

    if (name.trim()) {
      params.name = name.trim();
    }
    if (workingDirectory.trim()) {
      params.working_directory = workingDirectory.trim();
    }
    if (autoApprove) {
      params.auto_approve = true;
    }

    onCreateSession(params);
    onClose();
  };

  return (
    <>
    <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="top-[env(safe-area-inset-top)] translate-y-0 max-h-[85vh] overflow-y-auto sm:top-[50%] sm:translate-y-[-50%]"
        onPointerDownOutside={(e) => {
          if (directoryPickerClosingRef.current) e.preventDefault();
        }}
        onFocusOutside={(e) => {
          if (directoryPickerClosingRef.current) {
            e.preventDefault();
            directoryPickerClosingRef.current = false;
          }
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && e.shiftKey) {
            e.preventDefault();
            handleSubmit(e as unknown as React.FormEvent);
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>New Session</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <AgentSelector value={agentType} onChange={setAgentType} />

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Name{" "}
              <span className="text-muted-foreground font-normal">
                (optional)
              </span>
            </label>
            <Input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Auto-generated if empty"
              autoFocus
            />
          </div>

          {/* TODO: When path is a git repo, show repo info (current branch),
             branch/worktree selector, and option to create new worktree */}
          <div className="space-y-2">
            <label className="text-sm font-medium">Working Directory</label>
            <div className="flex gap-2">
              <Input
                value={workingDirectory}
                onChange={(e) => setWorkingDirectory(e.target.value)}
                placeholder="~/projects/my-app"
                className="flex-1"
              />
              <Button
                type="button"
                variant="outline"
                size="icon"
                onClick={() => setShowDirectoryPicker(true)}
                aria-label="Browse folders"
              >
                <FolderOpen className="h-4 w-4" />
              </Button>
            </div>
          </div>

          {/* Auto-approve toggle */}
          <div className="flex items-center justify-between">
            <div className="space-y-0.5">
              <label className="text-sm font-medium">Auto-approve</label>
              <p className="text-muted-foreground text-xs">
                Skip permission prompts for tool calls
              </p>
            </div>
            <Switch
              checked={autoApprove}
              onCheckedChange={setAutoApprove}
            />
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
            >
              Cancel
            </Button>
            <Button type="submit">Create</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
    <DirectoryPicker
      open={showDirectoryPicker}
      onOpenChange={(o) => {
        if (!o) directoryPickerClosingRef.current = true;
        setShowDirectoryPicker(o);
      }}
      onSelect={(path) => setWorkingDirectory(path)}
      initialPath={workingDirectory}
    />
    </>
  );
}

