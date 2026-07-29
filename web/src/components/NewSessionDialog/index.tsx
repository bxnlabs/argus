import { useState, useEffect, useRef } from "react";
import { Loader2 } from "lucide-react";
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
import { DockerizedBadge } from "@/components/DockerizedBadge";
import { cn } from "@/lib/utils";
import { isMac } from "@/lib/device";
import { useViewport } from "@/hooks/useViewport";
import { useSessionMutationState } from "@/hooks/useSessionMutationState";
import { useSingleFlight } from "@/hooks/useSingleFlight";
import type { ProviderType, CreateSessionParams } from "@/types";

type SourceTab = "local" | "remote";

interface NewSessionDialogProps {
  open: boolean;
  onClose: () => void;
  // Returning a promise is what lets the dialog hold its submit lock for the
  // life of the create; a void return narrows the lock to the current tick.
  onCreateSession: (params: CreateSessionParams) => void | Promise<void>;
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
  // Ownership, not a lock. `isCreating` is node-wide: it says *a* create is in
  // flight, not that *this* form submitted it. Reopening the dialog mid-create
  // would otherwise show a blank form claiming to be "Creating…". Deliberately
  // outlives the settle — unlike the lock below, which clears on it — so
  // `isOtherCreatePending` cannot misfire on the way down, while `isCreating`
  // is still falling asynchronously.
  const [ownsCreate, setOwnsCreate] = useState(false);
  // Serialises submits: App's single-slot toast handoff assumes creates are
  // serialised, and `isCreating` arrives too late to lock out a second submit
  // in the same tick — see useSingleFlight for why. `dispatched` is the render
  // mirror of that lock, so the form disables in the same commit as the
  // dispatch rather than a task later.
  const {
    pending: dispatched,
    run: runCreate,
    reset: resetCreate,
  } = useSingleFlight();
  const childPickerClosingRef = useRef(false);
  const sourceTriggerRef = useRef<HTMLButtonElement>(null);
  const branchTriggerRef = useRef<HTMLButtonElement>(null);
  const { data: profilesData, refetch: refetchProfiles } = useProfilesQuery();
  const profiles = profilesData?.profiles ?? [];
  const { isMobile } = useViewport();
  const { isCreating } = useSessionMutationState();
  // Anything holding the create lock, whether or not React has seen it yet.
  const creating = isCreating || dispatched;
  // This form is submitting; vs. someone else's create holding the lock.
  const isSubmitting = (isCreating && ownsCreate) || dispatched;
  const isOtherCreatePending = isCreating && !ownsCreate;

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
      setOwnsCreate(false);
      childPickerClosingRef.current = false;
      // Reopening drops the lock too: it guards one create, and the dialog
      // outlives that create (App leaves it closed once the toast has taken
      // over, but reuses this same instance for the next open). So a new submit
      // never opens already claiming to create.
      resetCreate();
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

  const createSession = () => {
    // Creates are serialised: one at a time, so App's single-slot toast
    // handoff bookkeeping stays correct. `runCreate` holds the same line
    // synchronously for a second submit in this same tick.
    if (creating) return;
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

    setOwnsCreate(true);
    // Errors stay the caller's — App resolves them into a toast.
    runCreate(() => onCreateSession(params));
    // No onClose() here: App closes this dialog in its success branch, so a
    // failed create leaves the form intact instead of discarding it.
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createSession();
  };

  return (
    <>
      <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
        <DialogContent
          showCloseButton={false}
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
          // Capture phase is load-bearing: a focused Select trigger (e.g. the
          // provider Select) opens on *any* Enter (no modifier check), and its
          // keydown would run before a bubble-phase handler. Running in capture
          // and calling preventDefault first keeps Cmd/Ctrl+Enter a clean submit
          // (Radix composes its handler to skip opening once defaultPrevented);
          // stopPropagation is a version-independent hard stop on top.
          onKeyDownCapture={(e) => {
            // Ignore keydowns from portaled dropdowns (e.g. the provider
            // Select): their options render outside this DOM subtree but still
            // traverse the React tree, so Cmd/Ctrl+Enter while navigating an
            // open dropdown would submit with a stale value. Returning early
            // (before stopPropagation) lets the dropdown commit normally.
            if (!e.currentTarget.contains(e.target as Node)) return;
            if (e.nativeEvent.isComposing) return;
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              e.stopPropagation();
              createSession();
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>New Session</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <fieldset disabled={creating} className="min-w-0 space-y-4">
              <ProviderSelector value={providerType} onChange={setProviderType} />

              <div className="space-y-2">
                <label className="text-sm font-medium">Name</label>
                <Input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  onKeyDown={(e) => {
                    // Enter never submits; submit is ⌘/Ctrl+Enter or the Create
                    // button. Let ⌘/Ctrl+Enter bubble to the dialog handler.
                    if (e.nativeEvent.isComposing) return;
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
                        <SelectItem key={p.name} value={p.name}>
                          <span className="flex items-center gap-2">
                            {p.name}
                            {p.type === "docker" && (
                              <DockerizedBadge className="shrink-0" />
                            )}
                          </span>
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
            </fieldset>

            {/* Outside the fieldset so it stays legible while the fields grey out. */}
            {isOtherCreatePending && (
              <p className="text-muted-foreground text-sm">
                Another session is still being created…
              </p>
            )}

            <DialogFooter className="sm:items-center">
              <div className="flex justify-end gap-2 sm:ml-auto">
                <Button type="button" variant="outline" onClick={onClose}>
                  Cancel
                </Button>
                <Button type="submit" disabled={!name.trim() || creating}>
                  {isSubmitting ? (
                    <>
                      <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
                      Creating…
                    </>
                  ) : (
                    <>
                      Create
                      {!isMobile && (
                        <kbd
                          aria-hidden="true"
                          className="bg-primary-foreground/15 hidden rounded px-1 py-0.5 text-[10px] sm:inline-block"
                        >
                          {isMac() ? "⌘ ↵" : "Ctrl ↵"}
                        </kbd>
                      )}
                    </>
                  )}
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
