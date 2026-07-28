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
import { DockerizedBadge } from "@/components/DockerizedBadge";
import { useProfilesQuery } from "@/data/sessions";
import { isMac } from "@/lib/device";
import { useViewport } from "@/hooks/useViewport";
import { useSessionMutationState } from "@/hooks/useSessionMutationState";
import type { Session } from "@/types";

// Sentinel for the "no profile" option. Radix Select disallows "", and the "@"
// chars fall outside the backend profile-name charset ([a-zA-Z0-9_-]), so this
// can never collide with a real profile name.
const NONE_VALUE = "@@none@@";

interface ChangeProfileDialogProps {
  session: Session | null;
  onClose: () => void;
  // Returning a promise is what lets the dialog hold its apply lock for the
  // life of the change; a void return narrows the lock to the current tick.
  onApply: (sessionId: string, profile: string | null) => void | Promise<void>;
}

export function ChangeProfileDialog({
  session,
  onClose,
  onApply,
}: ChangeProfileDialogProps) {
  const { data: profilesData } = useProfilesQuery();
  const profiles = profilesData?.profiles ?? [];
  const { isMobile } = useViewport();
  const { busySessions } = useSessionMutationState();
  const isApplying = session ? busySessions[session.id] === "profile" : false;

  // Synchronous mirror of `isApplying`, which reaches this component
  // asynchronously — TanStack recomputes its snapshot on each cache event but
  // schedules React's re-read — so it cannot lock out a second apply dispatched
  // in the same tick (the button and the Cmd/Ctrl+Enter handler are separate
  // entry points into `handleApply`). Serialising matters more here than for a
  // create: a profile change restarts the session, so a duplicate is a second
  // restart whose request races the resulting id change.
  const applyingRef = useRef(false);
  // Which apply the lock currently belongs to. A settled apply may land long
  // after the dialog moved on — retargeted, or reopened on the same session —
  // and an ungated release would then clear a *newer* apply's lock.
  const applyGenerationRef = useRef(0);
  // Render mirror of that lock. The ref is what serialises applies, but a ref
  // change does not re-render, so the button and Select would stay visually
  // live for the whole task it takes `isApplying` to arrive — locked but not
  // looking it. Setting state alongside the ref disables them in the same
  // commit as the dispatch.
  const [submitted, setSubmitted] = useState(false);
  const applying = isApplying || submitted;

  const currentValue = session?.profile ?? NONE_VALUE;
  const [selected, setSelected] = useState(currentValue);

  // Reset the selection only when a different session is targeted (by id).
  // Keying on the whole `session` would also reset on every background refetch
  // that touches this session, clobbering an in-progress selection.
  useEffect(() => {
    setSelected(session?.profile ?? NONE_VALUE);
    // Retarget drops the lock too: it guards one session's apply, and the
    // dialog outlives that apply (App keeps it open on failure). The mirror
    // goes with it, so a new target never opens already claiming to apply.
    applyGenerationRef.current += 1;
    applyingRef.current = false;
    setSubmitted(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.id]);

  const handleApply = () => {
    if (!session || applying || applyingRef.current) return;
    const profile = selected === NONE_VALUE ? null : selected;

    const generation = ++applyGenerationRef.current;
    applyingRef.current = true;
    setSubmitted(true);
    // Release on settle, from the call itself rather than from watching
    // `isApplying` fall: an apply that settles before TanStack's scheduled
    // notify reaches React never renders a pending snapshot at all, and a
    // release keyed on that transition would strand the dialog. Errors stay the
    // caller's — App resolves them into a toast.
    const release = () => {
      if (applyGenerationRef.current !== generation) return;
      applyingRef.current = false;
      setSubmitted(false);
    };
    try {
      Promise.resolve(onApply(session.id, profile)).then(release, release);
    } catch (err) {
      // A synchronous throw escapes the promise chain and would strand the lock
      // the same way an unobserved settle does. Release, then let it propagate.
      release();
      throw err;
    }
    // No onClose() here: App closes this dialog in its success branch, so a
    // failed apply leaves the dialog open with the selection intact.
  };

  const unchanged = selected === currentValue;

  return (
    <Dialog open={session !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        // Capture phase is load-bearing here: the Select trigger is focused on
        // open and Radix opens the dropdown on *any* Enter (no modifier check),
        // so a bubble-phase handler would fire too late — the trigger's own
        // keydown runs first. Running in capture and calling preventDefault
        // *before* the trigger's handler keeps an unchanged shortcut a true
        // no-op (Radix composes its handler so it skips opening once the event
        // is defaultPrevented). stopPropagation is a version-independent hard
        // stop on top of that.
        onKeyDownCapture={(e) => {
          // Ignore keydowns from the portaled Select dropdown: its options
          // render outside this DOM subtree but still traverse the React tree.
          // Without this guard, Cmd/Ctrl+Enter while keyboard-navigating the
          // open dropdown would apply the *previous* selection (the new one is
          // only scheduled, not yet committed) and close. Returning early here
          // (before stopPropagation) lets Radix commit the option normally.
          if (!e.currentTarget.contains(e.target as Node)) return;
          if (e.nativeEvent.isComposing) return;
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            e.stopPropagation();
            if (!unchanged) handleApply();
          }
        }}
      >
        <DialogHeader>
          <DialogTitle>Change Profile</DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium">Profile</label>
            <Select
              value={selected}
              onValueChange={setSelected}
              disabled={applying}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select a profile..." />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={NONE_VALUE}>None (detach)</SelectItem>
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

          <p className="text-muted-foreground text-xs">
            Changing the profile restarts this session.
          </p>
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            type="button"
            onClick={handleApply}
            disabled={unchanged || applying}
          >
            {applying ? (
              <>
                <Loader2 aria-hidden="true" className="h-4 w-4 animate-spin" />
                Applying…
              </>
            ) : (
              <>
                Apply
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
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
