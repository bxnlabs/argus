import { useCallback, useEffect, useRef } from "react";
import { toast } from "sonner";
import type { Session } from "@/types";
import {
  useSessionsQuery,
  useRosterFetchState,
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

  // What the server has actually said, tracked off the query's cache events
  // rather than its rendered status. `settled` gates anything that acts on a
  // session's absence; `fetchedRevision` counts answers, which the toast below
  // needs instead of a state.
  const { settled: rosterSettled, fetchedRevision } = useRosterFetchState();

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
  // rows and the query keeps polling, so there's nothing to retry by hand.
  //
  // Fires on the transition into failure, not on every failed poll, and is held
  // while a fetch is in flight, because an error can outlive the outage that
  // caused it — a remount after a failed visit starts with that error on the
  // query and a refetch already running to settle it.
  //
  // Re-arms by consuming the answer count rather than reading `settled`: a poll
  // that succeeds and is immediately followed by a mutation's optimistic write
  // renders once, as `settled: false`, so reading the state would miss the
  // recovery and leave the next real outage unannounced.
  //
  // The message stays neutral about the cause: any rejected query lands here,
  // including 4xx/5xx and malformed JSON, not just an unreachable node.
  const announcedFailure = useRef(false);
  const seenRevision = useRef(0);
  useEffect(() => {
    if (fetchedRevision !== seenRevision.current) {
      seenRevision.current = fetchedRevision;
      announcedFailure.current = false;
    }
    if (isError && !isFetching && !announcedFailure.current) {
      announcedFailure.current = true;
      toast.error("Couldn't load sessions — retrying…", { id: "sessions-fetch" });
    }
  }, [isError, isFetching, fetchedRevision]);

  return {
    sessions,
    homeDir,
    // Answers "is there something to draw", so the sidebar can tell "no
    // sessions" apart from "not yet known". Keyed off holding data rather than
    // query status: a failed poll leaves `status: "error"` on top of a perfectly
    // good cached list, which the sidebar should keep drawing.
    isLoaded: data !== undefined,
    // Whether the list can be trusted to say which sessions *don't* exist.
    // Cached data under a failed refetch is drawable while the server's current
    // roster is unknown, so this is a separate flag from `isLoaded`: rendering
    // wants the weaker question, acting on absence wants this one.
    //
    // False whenever a fetch is in flight, the last one failed, or the cache was
    // written locally — exactly when "not in the list" means "not yet" rather
    // than "not anymore".
    isRosterSettled: rosterSettled,
    deleteSession,
    renameSession,
    changeProfile,
    togglePin,
    markRead,
    markUnread,
  };
}
