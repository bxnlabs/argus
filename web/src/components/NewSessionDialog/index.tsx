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
import { ProviderSelector } from "./ProviderSelector";
import { PickerTriggerField } from "./PickerTriggerField";
import { SourcePicker } from "@/components/SourcePicker";
import { useProfilesQuery } from "@/data/sessions";
import { useGitCheckQuery } from "@/data/git/queries";
import {
  Dialog as BranchDialog,
  DialogContent as BranchDialogContent,
  DialogHeader as BranchDialogHeader,
  DialogTitle as BranchDialogTitle,
} from "@/components/ui/dialog";
import { BranchPicker } from "@/components/BranchPicker";
import { cn } from "@/lib/utils";
import { isMac } from "@/lib/device";
import { useViewport } from "@/hooks/useViewport";
import type { ProviderType, CreateSessionParams } from "@/types";

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
  const [providerType, setProviderType] = useState<ProviderType>("claude");
  const [source, setSource] = useState("");
  const [sourceTab, setSourceTab] = useState<SourceTab>("local");
  const [autoApprove, setAutoApprove] = useState(true);
  const [profile, setProfile] = useState("");
  const [branch, setBranch] = useState("");
  const [showBranchPicker, setShowBranchPicker] = useState(false);
  const [showSourcePicker, setShowSourcePicker] = useState(false);
  const childPickerClosingRef = useRef(false);
  const sourceTriggerRef = useRef<HTMLButtonElement>(null);
  const branchTriggerRef = useRef<HTMLButtonElement>(null);
  const { data: profilesData, refetch: refetchProfiles } = useProfilesQuery();
  const profiles = profilesData?.profiles ?? [];
  const { isMobile } = useViewport();

  const isRemoteSource = sourceTab === "remote" && source !== "";
  const { data: isLocalGitRepo = false } = useGitCheckQuery(
    sourceTab === "local" && source ? source : null,
  );
  const isGitBacked = isRemoteSource || isLocalGitRepo;
  const showBranchField = isGitBacked && providerType !== "shell";

  useEffect(() => {
    if (open) {
      refetchProfiles();
      setName("");
      setProviderType("claude");
      setSource("");
      setSourceTab("local");
      setAutoApprove(true);
      setProfile("");
      setShowSourcePicker(false);
      setBranch("");
      setShowBranchPicker(false);
      childPickerClosingRef.current = false;
    }
  }, [open]);

  useEffect(() => {
    setBranch("");
  }, [source, sourceTab, providerType]);

  const restoreFocus = (ref: React.RefObject<HTMLButtonElement | null>) => {
    requestAnimationFrame(() => ref.current?.focus());
  };

  const closeBranchPicker = () => {
    childPickerClosingRef.current = true;
    setShowBranchPicker(false);
    restoreFocus(branchTriggerRef);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const trimmedName = name.trim();
    if (!trimmedName) return;

    const params: CreateSessionParams = {
      name: trimmedName,
      provider_type: providerType,
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

    if (branch) {
      params.branch = branch;
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
            if (childPickerClosingRef.current) {
              e.preventDefault();
              childPickerClosingRef.current = false;
            }
          }}
          onFocusOutside={(e) => {
            if (childPickerClosingRef.current) {
              e.preventDefault();
              childPickerClosingRef.current = false;
            }
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              handleSubmit(e as unknown as React.FormEvent);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>New Session</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <ProviderSelector value={providerType} onChange={setProviderType} />

            <div className="space-y-2">
              <label className="text-sm font-medium">Name</label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  // Enter never submits; submit is ⌘/Ctrl+Enter or the Create
                  // button. Let ⌘/Ctrl+Enter bubble to the dialog handler.
                  if (e.key === "Enter" && !(e.metaKey || e.ctrlKey)) {
                    e.preventDefault();
                  }
                }}
                placeholder="my-feature"
                autoFocus
              />
            </div>

            <PickerTriggerField
              ref={sourceTriggerRef}
              label="Source"
              value={source}
              placeholder="Select a folder or repository..."
              onOpen={() => setShowSourcePicker(true)}
              open={showSourcePicker}
            />

            {showBranchField && (
              <PickerTriggerField
                ref={branchTriggerRef}
                label="Branch"
                optional
                value={branch}
                placeholder="Select or type a branch..."
                onOpen={() => setShowBranchPicker(true)}
                open={showBranchPicker}
              />
            )}

            {profiles.length > 0 && (
              <div className="space-y-2">
                <label className="text-sm font-medium">Profile</label>
                <Select
                  value={profile || undefined}
                  onValueChange={setProfile}
                >
                  <SelectTrigger>
                    <SelectValue placeholder="Select a profile..." />
                  </SelectTrigger>
                  <SelectContent>
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

            <DialogFooter className="sm:items-center sm:justify-between">
              {!isMobile && (
                <span className="text-muted-foreground hidden text-xs sm:inline-flex sm:items-center sm:gap-1">
                  <kbd className="bg-muted rounded px-1.5 py-0.5">
                    {isMac() ? "⌘↵" : "Ctrl ↵"}
                  </kbd>
                  create
                </span>
              )}
              <div className="flex gap-2">
                <Button type="button" variant="outline" onClick={onClose}>
                  Cancel
                </Button>
                <Button type="submit" disabled={!name.trim()}>
                  Create
                </Button>
              </div>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <SourcePicker
        open={showSourcePicker}
        onOpenChange={(o) => {
          if (!o) {
            childPickerClosingRef.current = true;
            restoreFocus(sourceTriggerRef);
          }
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
      {showBranchField && (
        <BranchDialog
          open={showBranchPicker}
          onOpenChange={(o) => {
            if (!o) {
              childPickerClosingRef.current = true;
              restoreFocus(branchTriggerRef);
            }
            setShowBranchPicker(o);
          }}
        >
          <BranchDialogContent
            className={cn(
              "gap-0 overflow-hidden p-0",
              isMobile
                ? "top-[env(safe-area-inset-top)] left-0 right-0 h-[calc(var(--app-height)_-_env(safe-area-inset-top))] flex flex-col max-w-none translate-x-0 translate-y-0 rounded-none border-0"
                : "top-[50%] left-[50%] translate-x-[-50%] translate-y-[-50%] sm:max-w-md",
            )}
            showCloseButton={false}
          >
            <BranchDialogHeader className="sr-only">
              <BranchDialogTitle>Select Branch</BranchDialogTitle>
            </BranchDialogHeader>
            <BranchPicker
              open={showBranchPicker}
              source={source}
              onSelect={(b) => {
                setBranch(b);
                closeBranchPicker();
              }}
              onClose={closeBranchPicker}
            />
          </BranchDialogContent>
        </BranchDialog>
      )}
    </>
  );
}
