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
  // rather than its rendered status — see useRosterFetchState for why the
  // render layer cannot answer this. `everFetched` gates one-shot work,
  // `settled` gates the repeated kind.
  const {
    everFetched: rosterFetched,
    settled: rosterSettled,
    fetchedRevision,
  } = useRosterFetchState();

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
  // Re-arming keys off a real server answer, not `!isError` and not `isSuccess`.
  // `!isError` goes true for the in-flight moment of every retry, since with no
  // cached data the status cycles error → pending → error (measured), which
  // would re-announce on every poll for a node that has never answered — the
  // exact case this guard exists for. `isSuccess` fails the other way: an
  // optimistic rename or pin counts as success, so editing a cached row during
  // an outage would score as recovery and let the next failure announce again.
  //
  // The message stays neutral about the cause: `isError` is any rejected query,
  // and apiFetch rejects on 4xx/5xx and malformed JSON as readily as on an
  // unreachable host, so naming connectivity would misdiagnose a node that is
  // reachable and failing.
  //
  // Held while a fetch is in flight, because an error can outlive the outage
  // that caused it. Coming back to a node that failed last time you were here
  // arrives with that stale error still on the query and a refetch already
  // running to settle it — announcing then reports a node as down that may be
  // answering by the time the words are on screen. Waiting for the fetch to land
  // costs a round trip on a genuinely dead node and nothing on a live one.
  //
  // Re-arming consumes the revision rather than reading `settled`, because
  // recovery is an event and `settled` is a state you can already have left.
  // A poll that succeeds and is immediately followed by a mutation's optimistic
  // write renders once, as `settled: false` — so the node came back, the guard
  // never noticed, and the *next* real outage was announced to nobody. The
  // count survives that collapse; a level cannot.
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
    // Whether the roster on hand is a settled server answer *right now*. The
    // flag above latches once per mount, which suits a one-shot pass but is
    // useless to anything asking the question repeatedly — by the time a dialog
    // opens it has long since latched true. This one goes false again whenever a
    // fetch is in flight, the last one failed, or the cache was written locally,
    // which is exactly when "that session isn't in the list" means "not yet"
    // rather than "not anymore".
    isRosterSettled: rosterSettled,
    deleteSession,
    renameSession,
    changeProfile,
    togglePin,
    markRead,
    markUnread,
  };
}
