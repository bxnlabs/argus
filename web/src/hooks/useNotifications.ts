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
  const previousStates = useRef<Map<string, string>>(new Map());
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

  const checkStateChanges = useCallback(
    (
      states: Array<{ id: string; name: string; status: string }>,
      activeSessionId?: string | null
    ) => {
      if (!settings.enabled) return;

      for (const state of states) {
        const prev = previousStates.current.get(state.id);
        previousStates.current.set(state.id, state.status);

        if (prev === "running" && state.status === "waiting" && state.id !== activeSessionId) {
          toast.info(`${state.name} is waiting for input`);

          if (permission === "granted" && document.hidden) {
            new Notification("Argus", {
              body: `${state.name} is waiting for input`,
              tag: `waiting-${state.id}`,
            });
          }

          // Flash tab title
          document.title = `⚡ ${state.name} waiting`;
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

  return { settings, permission, requestPermission, updateSettings, checkStateChanges };
}
