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

type SourceTab = "local" | "remote";

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
  const [sourceTab, setSourceTab] = useState<SourceTab>("local");
  const [localDir, setLocalDir] = useState("");
  const [remoteRepo, setRemoteRepo] = useState("");
  const [autoApprove, setAutoApprove] = useState(true);
  const [showDirectoryPicker, setShowDirectoryPicker] = useState(false);
  const directoryPickerClosingRef = useRef(false);

  useEffect(() => {
    if (open) {
      setName("");
      setAgentType("claude");
      setSourceTab("local");
      setLocalDir("");
      setRemoteRepo("");
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

    const sourceValue =
      sourceTab === "local" ? localDir.trim() : remoteRepo.trim();
    if (sourceValue) {
      params.source = sourceValue;
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

            <div className="space-y-2">
              {/* Tab switcher */}
              <div className="flex gap-1 rounded-md bg-muted p-1 w-fit">
                <button
                  type="button"
                  onClick={() => setSourceTab("local")}
                  className={`px-3 py-1 text-sm rounded-sm transition-colors ${
                    sourceTab === "local"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  Local
                </button>
                <button
                  type="button"
                  onClick={() => setSourceTab("remote")}
                  className={`px-3 py-1 text-sm rounded-sm transition-colors ${
                    sourceTab === "remote"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  Remote
                </button>
              </div>

              {sourceTab === "local" ? (
                <div>
                  <label className="text-sm font-medium">Directory</label>
                  <div className="flex gap-2 mt-1">
                    <Input
                      value={localDir}
                      onChange={(e) => setLocalDir(e.target.value)}
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
              ) : (
                <div>
                  <label className="text-sm font-medium">Repository</label>
                  <Input
                    value={remoteRepo}
                    onChange={(e) => setRemoteRepo(e.target.value)}
                    placeholder="org/repo  or  https://github.com/org/repo.git"
                    className="mt-1"
                  />
                </div>
              )}
            </div>

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
              <Button type="button" variant="outline" onClick={onClose}>
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
        onSelect={(path) => setLocalDir(path)}
        initialPath={localDir}
      />
    </>
  );
}
