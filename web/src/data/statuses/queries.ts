import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { apiFetch, apiTextFetch } from "@/api/client";
import { useActiveNode } from "@/hooks/useActiveNode";
import type { Session, SessionStatusInfo } from "@/types";
import { statusKeys } from "../sessions/keys";

interface StatusResponse {
  statuses: Record<string, SessionStatusInfo>;
}

interface UseSessionStatusesOptions {
  sessions: Session[];
  activeSessionId?: string | null;
  checkStateChanges: (
    states: Array<{
      id: string;
      name: string;
      status: SessionStatusInfo["status"];
      unreadSince?: string | null;
    }>,
    activeSessionId?: string | null,
  ) => void;
}

export function useSessionStatusesQuery({
  sessions,
  activeSessionId,
  checkStateChanges,
}: UseSessionStatusesOptions) {
  const { scope, baseUrl } = useActiveNode();
  const query = useQuery({
    queryKey: statusKeys.all(scope),
    queryFn: () => apiFetch<StatusResponse>(baseUrl, "/api/node/sessions/status"),
    enabled: sessions.length > 0,
    staleTime: 2000,
    refetchInterval: sessions.length > 0 ? 2000 : false,
  });

  // Send heartbeat for the actively viewed session. Fires immediately when
  // activeSessionId is set (covering app-load before the first poll completes)
  // and again on every poll result change (~2s cadence).
  useEffect(() => {
    if (!activeSessionId) return;
    if (document.hidden) return;

    // Fire-and-forget heartbeat — errors are silently ignored. Routed through
    // the API client so it targets the active node's base URL.
    apiTextFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(activeSessionId)}/heartbeat`, {
      method: "POST",
    }).catch(() => {});
  }, [query.data, activeSessionId, baseUrl]);

  // Auto-acknowledge the automatic unread_since for the actively viewed session.
  // Covers the case where the app opens with a session already selected
  // (from localStorage) that became unread while the user was away. Acknowledge
  // clears unread_since only and leaves the manual user_marked_unread_at intact,
  // so a sticky "Mark as unread" survives viewing.
  useEffect(() => {
    if (!activeSessionId || !query.data) return;
    if (document.hidden) return;

    const status = query.data.statuses?.[activeSessionId];
    if (status?.unreadSince) {
      apiTextFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(activeSessionId)}/acknowledge`, {
        method: "POST",
      }).catch(() => {});
    }
  }, [query.data, activeSessionId, baseUrl]);

  useEffect(() => {
    if (!query.data?.statuses) return;

    const statuses = query.data.statuses;

    const sessionStates = sessions.map((s) => ({
      id: s.id,
      name: s.name,
      status: (statuses[s.id]?.status || "dead") as SessionStatusInfo["status"],
      unreadSince: statuses[s.id]?.unreadSince,
    }));
    checkStateChanges(sessionStates, activeSessionId);
  }, [query.data, sessions, activeSessionId, checkStateChanges]);

  return {
    sessionStatuses: query.data?.statuses ?? ({} as Record<string, SessionStatusInfo>),
    isLoading: query.isLoading,
  };
}

// Mark a session read: clears both the automatic unread_since and the manual
// user_marked_unread_at, optimistically updating the status cache.
export function useMarkRead() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiTextFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/read`, {
        method: "POST",
      }),
    onMutate: async (sessionId: string) => {
      await queryClient.cancelQueries({ queryKey: statusKeys.all(scope) });
      const previous = queryClient.getQueryData<StatusResponse>(statusKeys.all(scope));
      queryClient.setQueryData<StatusResponse>(statusKeys.all(scope), (old) => {
        const status = old?.statuses?.[sessionId];
        if (!old || !status) return old;
        return {
          ...old,
          statuses: {
            ...old.statuses,
            [sessionId]: { ...status, unreadSince: null, userMarkedUnreadAt: null },
          },
        };
      });
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(statusKeys.all(scope), context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: statusKeys.all(scope) });
    },
  });
}

// Mark a session unread: sets the manual user_marked_unread_at marker,
// optimistically updating the status cache.
export function useMarkUnread() {
  const queryClient = useQueryClient();
  const { scope, baseUrl } = useActiveNode();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiTextFetch(baseUrl, `/api/node/sessions/${encodeURIComponent(sessionId)}/unread`, {
        method: "POST",
      }),
    onMutate: async (sessionId: string) => {
      await queryClient.cancelQueries({ queryKey: statusKeys.all(scope) });
      const previous = queryClient.getQueryData<StatusResponse>(statusKeys.all(scope));
      queryClient.setQueryData<StatusResponse>(statusKeys.all(scope), (old) => {
        const status = old?.statuses?.[sessionId];
        if (!old || !status) return old;
        return {
          ...old,
          statuses: {
            ...old.statuses,
            [sessionId]: { ...status, userMarkedUnreadAt: new Date().toISOString() },
          },
        };
      });
      return { previous };
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) {
        queryClient.setQueryData(statusKeys.all(scope), context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: statusKeys.all(scope) });
    },
  });
}
