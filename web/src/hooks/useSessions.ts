import { useCallback, useRef } from "react";
import type { Session } from "@/types";
import {
  useSessionsQuery,
  useDeleteSession,
  useRenameSession,
  useUpdateSession,
} from "@/data/sessions";
import { useMarkRead, useMarkUnread } from "@/data/statuses/queries";

export function useSessions() {
  const { data, isSuccess } = useSessionsQuery();
  const sessions: Session[] = data?.sessions ?? [];
  const homeDir: string = data?.home_dir ?? "";

  const deleteMutation = useDeleteSession();
  const renameMutation = useRenameSession();
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

  return {
    sessions,
    homeDir,
    isLoaded: isSuccess,
    deleteSession,
    renameSession,
    togglePin,
    markRead,
    markUnread,
  };
}
