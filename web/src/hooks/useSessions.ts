import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import type { Session } from "@/types";
import {
  useSessionsQuery,
  useDeleteSession,
  useRenameSession,
  useChangeSessionProfile,
  useUpdateSession,
} from "@/data/sessions";
import { useMarkRead, useMarkUnread } from "@/data/statuses/queries";

export function useSessions() {
  const { data, isError, isSuccess, isFetching } = useSessionsQuery();
  const sessions: Session[] = data?.sessions ?? [];
  const homeDir: string = data?.home_dir ?? "";

  // Whether a roster fetch has actually come back from the server since this
  // mount. Watching the fetch settle, rather than reading `isSuccess`, because
  // `isSuccess` answers a different question than it appears to:
  //
  //   - A manual cache write counts as success. `useRenameSession` and
  //     `useUpdateSession` both `setQueryData` on this exact key in `onMutate`,
  //     and on 5.100.14 that flips a query sitting at `status: "error"` straight
  //     to `"success"` with no request in between (measured). Renaming one
  //     session during a roster outage would otherwise declare the stale list
  //     authoritative and detach a tab holding a session it predates.
  //   - Cached success is immediate on mount, which lands *before* TabProvider
  //     restores tabs from localStorage — child effects commit before parent
  //     ones — so the one-shot cleanup would consume its guard against the
  //     generated blank tab and never see the real ones.
  //
  // A settled fetch answers both: `setQueryData` never toggles `isFetching`, and
  // any completed fetch necessarily lands after the mount effects. The cost is
  // that a remount whose cache is still fresh waits for the next poll before
  // reconciling, which is the right way to be wrong here.
  const [rosterFetched, setRosterFetched] = useState(false);
  const wasFetching = useRef(false);
  useEffect(() => {
    if (wasFetching.current && !isFetching && isSuccess) setRosterFetched(true);
    wasFetching.current = isFetching;
  }, [isFetching, isSuccess]);

  const deleteMutation = useDeleteSession();
  const renameMutation = useRenameSession();
  const changeProfileMutation = useChangeSessionProfile();
  const updateMutation = useUpdateSession();
  const markReadMutation = useMarkRead();
  const markUnreadMutation = useMarkUnread();

  // Keep stable refs to mutateAsync so callbacks don't change on every render.
  // TanStack Query's useMutation returns a new object each render, which would
  // otherwise cascade new function references through the entire component tree.
  const deleteMutateRef = useRef(deleteMutation.mutateAsync);
  deleteMutateRef.current = deleteMutation.mutateAsync;
  const renameMutateRef = useRef(renameMutation.mutateAsync);
  renameMutateRef.current = renameMutation.mutateAsync;
  const changeProfileMutateRef = useRef(changeProfileMutation.mutateAsync);
  changeProfileMutateRef.current = changeProfileMutation.mutateAsync;
  const updateMutateRef = useRef(updateMutation.mutateAsync);
  updateMutateRef.current = updateMutation.mutateAsync;
  const markReadRef = useRef(markReadMutation.mutateAsync);
  markReadRef.current = markReadMutation.mutateAsync;
  const markUnreadRef = useRef(markUnreadMutation.mutateAsync);
  markUnreadRef.current = markUnreadMutation.mutateAsync;

  const deleteSession = useCallback(
    async (sessionId: string, deleteBranch?: boolean) => {
      const message = deleteBranch
        ? "Delete this session and its branch? This cannot be undone."
        : "Delete this session? This cannot be undone.";
      if (!confirm(message)) return null;
      return await deleteMutateRef.current({ sessionId, deleteBranch });
    },
    [],
  );

  const renameSession = useCallback(
    async (sessionId: string, newName: string) => {
      await renameMutateRef.current({ sessionId, newName });
    },
    [],
  );

  const changeProfile = useCallback(
    async (sessionId: string, profile: string | null) => {
      await changeProfileMutateRef.current({ sessionId, profile });
    },
    [],
  );

  const togglePin = useCallback(
    async (sessionId: string, pinned: boolean) => {
      await updateMutateRef.current({ sessionId, pinned });
    },
    [],
  );

  const markRead = useCallback(async (sessionId: string) => {
    await markReadRef.current(sessionId);
  }, []);

  const markUnread = useCallback(async (sessionId: string) => {
    await markUnreadRef.current(sessionId);
  }, []);

  // A failed fetch is announced, not rendered: the list keeps its placeholder
  // rows and the query keeps polling on its own interval, so there's nothing to
  // retry by hand and nothing worth taking the sidebar over for.
  //
  // Fires on the transition into failure, not on every failed poll — otherwise
  // an unreachable node would emit a toast every 10s for as long as it stayed
  // down. Re-arms once a fetch succeeds, so a second outage is announced again.
  //
  // Re-arming keys off `isSuccess`, not `!isError`, because those differ exactly
  // where it matters. With no cached data the status cycles error → pending →
  // error across a retry (measured on 5.100.14), so `!isError` goes true for the
  // in-flight moment and clears the flag — meaning a node that has *never*
  // answered re-announces on every poll, which is precisely the case this guard
  // exists for. `isSuccess` is false throughout that cycle and only turns true
  // on an actual answer.
  //
  // The message stays neutral about the cause: `isError` is any rejected query,
  // and apiFetch rejects on 4xx/5xx and malformed JSON as readily as on an
  // unreachable host, so naming connectivity would misdiagnose a node that is
  // reachable and failing.
  const announcedFailure = useRef(false);
  useEffect(() => {
    if (isError && !announcedFailure.current) {
      announcedFailure.current = true;
      toast.error("Couldn't load sessions — retrying…", { id: "sessions-fetch" });
    } else if (isSuccess && announcedFailure.current) {
      announcedFailure.current = false;
    }
  }, [isError, isSuccess]);

  return {
    sessions,
    homeDir,
    // Surfaced so the sidebar can tell "no sessions" apart from "not yet known".
    // Until the first fetch lands, an empty list is the absence of an answer,
    // not an answer.
    //
    // Keyed off holding data rather than query status: the list polls every 10s,
    // and a failed poll leaves `status: "error"` sitting on top of a perfectly
    // good cached list, so `isSuccess` would drop the whole sidebar back to
    // placeholders every time a remote node blinked.
    isLoaded: data !== undefined,
    // Whether the list can be trusted to say which sessions *don't* exist.
    // `isLoaded` deliberately answers a weaker question — "is there something to
    // draw" — and cached data under a failed refetch answers it yes while the
    // server's current roster is unknown. That's the right call for rendering
    // and the wrong one for anything that deletes on absence, so the two are
    // separate flags rather than one doing double duty.
    isRosterAuthoritative: rosterFetched,
    deleteSession,
    renameSession,
    changeProfile,
    togglePin,
    markRead,
    markUnread,
  };
}
