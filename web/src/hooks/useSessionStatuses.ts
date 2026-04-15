import type { Session, SessionStatusInfo } from "@/types";
import { useSessionStatusesQuery } from "@/data/statuses/queries";

interface UseSessionStatusesOptions {
  sessions: Session[];
  activeSessionId?: string | null;
  checkStateChanges: (
    states: Array<{ id: string; name: string; status: SessionStatusInfo["status"]; unreadSince?: string | null }>,
    activeSessionId?: string | null
  ) => void;
}

export function useSessionStatuses({ sessions, activeSessionId, checkStateChanges }: UseSessionStatusesOptions) {
  const { sessionStatuses } = useSessionStatusesQuery({ sessions, activeSessionId, checkStateChanges });
  return { sessionStatuses };
}
