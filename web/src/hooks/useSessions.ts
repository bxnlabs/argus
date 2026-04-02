import { useCallback } from "react";
import type { Session } from "@/types";
import {
  useSessionsQuery,
  useDeleteSession,
  useRenameSession,
} from "@/data/sessions";

export function useSessions() {
  const { data, isSuccess } = useSessionsQuery();
  const sessions: Session[] = data?.sessions ?? [];
  const homeDir: string = data?.home_dir ?? "";

  const deleteMutation = useDeleteSession();
  const renameMutation = useRenameSession();

  const deleteSession = useCallback(
    async (sessionId: string, deleteBranch?: boolean) => {
      const message = deleteBranch
        ? "Delete this session and its branch? This cannot be undone."
        : "Delete this session? This cannot be undone.";
      if (!confirm(message)) return;
      await deleteMutation.mutateAsync({ sessionId, deleteBranch });
    },
    [deleteMutation],
  );

  const renameSession = useCallback(
    async (sessionId: string, newName: string) => {
      await renameMutation.mutateAsync({ sessionId, newName });
    },
    [renameMutation]
  );

  return { sessions, homeDir, isLoaded: isSuccess, deleteSession, renameSession };
}
