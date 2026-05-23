import { useState, useEffect, useCallback, useRef } from "react";
import { toast } from "sonner";

interface NotificationSettings {
  enabled: boolean;
  sound: boolean;
}

const STORAGE_KEY = "argus-notification-settings";

function loadSettings(): NotificationSettings {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored) return JSON.parse(stored);
  } catch {}
  return { enabled: true, sound: false };
}

function saveSettings(settings: NotificationSettings) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(settings));
}

export function useNotifications() {
  const [settings, setSettings] = useState<NotificationSettings>(loadSettings);
  const [permission, setPermission] = useState<NotificationPermission>(
    typeof Notification !== "undefined" ? Notification.permission : "default"
  );
  const previousUnread = useRef<Set<string>>(new Set());
  const initialized = useRef(false);
  const originalTitle = useRef(document.title);

  const requestPermission = useCallback(async () => {
    if (typeof Notification === "undefined") return;
    const result = await Notification.requestPermission();
    setPermission(result);
  }, []);

  const updateSettings = useCallback((updates: Partial<NotificationSettings>) => {
    setSettings(prev => {
      const next = { ...prev, ...updates };
      saveSettings(next);
      return next;
    });
  }, []);

  // Pre-mark a session as already-unread so a manual "mark unread" doesn't
  // surface as a "finished working" notification. checkStateChanges treats the
  // session as unchanged; the existing read transition later clears it.
  const suppressUnreadNotification = useCallback((id: string) => {
    previousUnread.current.add(id);
  }, []);

  const checkStateChanges = useCallback(
    (
      states: Array<{ id: string; name: string; status: string; unreadSince?: string | null }>,
      activeSessionId?: string | null
    ) => {
      if (!settings.enabled) return;

      // Seed previousUnread on first call to avoid false notifications. Add-only
      // on purpose: a manual suppression added before the baseline must survive,
      // so never delete here (rebuilding to the observed set would drop it and
      // surface a false "finished working" toast once the mark-unread lands).
      if (!initialized.current) {
        initialized.current = true;
        for (const state of states) {
          if (state.unreadSince) {
            previousUnread.current.add(state.id);
          }
        }
        return;
      }

      for (const state of states) {
        const wasUnread = previousUnread.current.has(state.id);
        const isUnread = !!state.unreadSince;

        if (isUnread) {
          previousUnread.current.add(state.id);
        } else {
          previousUnread.current.delete(state.id);
        }

        // Notify when unreadSince newly appears on a non-active session
        if (!wasUnread && isUnread && state.id !== activeSessionId) {
          toast.info(`${state.name} finished working`);

          if (permission === "granted" && document.hidden) {
            new Notification("Argus", {
              body: `${state.name} finished working`,
              tag: `unread-${state.id}`,
            });
          }

          // Flash tab title
          document.title = `⚡ ${state.name} finished`;
          setTimeout(() => { document.title = originalTitle.current; }, 3000);
        }
      }
    },
    [settings.enabled, permission]
  );

  // Reset title on visibility change
  useEffect(() => {
    const handler = () => {
      if (document.visibilityState === "visible") {
        document.title = originalTitle.current;
      }
    };
    document.addEventListener("visibilitychange", handler);
    return () => document.removeEventListener("visibilitychange", handler);
  }, []);

  return { settings, permission, requestPermission, updateSettings, checkStateChanges, suppressUnreadNotification };
}
