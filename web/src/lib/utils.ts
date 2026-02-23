import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatRelativeTime(dateStr: string | null) {
  if (!dateStr) return "";
  const now = new Date();
  const date = new Date(dateStr);
  const diff = now.getTime() - date.getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

/**
 * Contract absolute home path to tilde for display.
 * Requires homePath (learned from API response's "path" field).
 * e.g. contractTilde("/home/jeevb/Workspace", "/home/jeevb") → "~/Workspace"
 */
export function contractTilde(path: string, homePath: string): string {
  if (!homePath) return path;
  if (path === homePath) return "~";
  if (path.startsWith(homePath + "/")) {
    return "~" + path.slice(homePath.length);
  }
  return path;
}
