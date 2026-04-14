import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { apiFetch } from "@/api/client";
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
  const query = useQuery({
    queryKey: statusKeys.all,
    queryFn: () => apiFetch<StatusResponse>("/node/api/sessions/status"),
    enabled: sessions.length > 0,
    staleTime: 2000,
    refetchInterval: sessions.length > 0 ? 2000 : false,
  });

  // Send heartbeat for the actively viewed session, piggybacked on poll cadence.
  useEffect(() => {
    if (!activeSessionId || !query.data) return;
    if (document.hidden) return;

    // Fire-and-forget heartbeat — errors are silently ignored.
    fetch(`${import.meta.env.VITE_NODE_URL || ""}/node/api/sessions/${encodeURIComponent(activeSessionId)}/heartbeat`, {
      method: "POST",
    }).catch(() => {});
  }, [query.data, activeSessionId]);

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
