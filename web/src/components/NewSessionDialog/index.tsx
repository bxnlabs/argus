import { useState, useEffect, useRef } from "react";
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
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { AgentSelector } from "./AgentSelector";
import { SourcePicker } from "@/components/SourcePicker";
import { useProfilesQuery } from "@/data/sessions";
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
  const [source, setSource] = useState("");
  const [sourceTab, setSourceTab] = useState<SourceTab>("local");
  const [autoApprove, setAutoApprove] = useState(true);
  const [profile, setProfile] = useState("");
  const [showSourcePicker, setShowSourcePicker] = useState(false);
  const sourcePickerClosingRef = useRef(false);
  const { data: profilesData } = useProfilesQuery();
  const profiles = profilesData?.profiles ?? [];

  useEffect(() => {
    if (open) {
      setName("");
      setAgentType("claude");
      setSource("");
      setSourceTab("local");
      setAutoApprove(true);
      setProfile("");
      setShowSourcePicker(false);
    }
  }, [open]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const trimmedName = name.trim();
    if (!trimmedName) return;

    const params: CreateSessionParams = {
      name: trimmedName,
      agent_type: agentType,
    };

    if (source) {
      params.source = source;
    }

    if (autoApprove) {
      params.auto_approve = true;
    }

    if (profile) {
      params.profile = profile;
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
            if (sourcePickerClosingRef.current) e.preventDefault();
          }}
          onFocusOutside={(e) => {
            if (sourcePickerClosingRef.current) {
              e.preventDefault();
              sourcePickerClosingRef.current = false;
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
              <label className="text-sm font-medium">Name</label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-feature"
                autoFocus
              />
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">Source</label>
              <Input
                value={source}
                readOnly
                onClick={() => setShowSourcePicker(true)}
                placeholder="Click to select a folder or repository..."
                className="cursor-pointer"
              />
            </div>

            {profiles.length > 0 && (
              <div className="space-y-2">
                <label className="text-sm font-medium">Profile</label>
                <Select
                  value={profile}
                  onValueChange={(v) => setProfile(v === "." ? "" : v)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Default" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value=".">Default</SelectItem>
                    {profiles.map((p) => (
                      <SelectItem key={p} value={p}>
                        {p}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}

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
              <Button type="submit" disabled={!name.trim()}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <SourcePicker
        open={showSourcePicker}
        onOpenChange={(o) => {
          if (!o) sourcePickerClosingRef.current = true;
          setShowSourcePicker(o);
        }}
        onSelect={(value, tab) => {
          setSource(value);
          setSourceTab(tab);
        }}
        initialTab={sourceTab}
        initialLocalPath={sourceTab === "local" ? source : undefined}
        initialRemoteQuery={sourceTab === "remote" ? source : undefined}
      />
    </>
  );
}
