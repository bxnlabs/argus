import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatRelativeTime(dateStr: string | null) {
  if (!dateStr) return "";
  const now = new Date();
  // SQLite datetime() returns UTC strings like "2025-03-03 15:04:05" with no
  // timezone indicator. JS Date() would treat that as local time, so convert
  // to ISO 8601 UTC format ("...T...Z") before parsing.
  const date = new Date(dateStr.replace(" ", "T") + "Z");
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

/**
 * Compress a path for display in constrained UI space.
 * 1. Replace home prefix with ~
 * 2. If longer than threshold, keep first + last 2 segments with /.../
 */
export function compressPath(
  path: string,
  homePath: string,
  threshold: number = 30,
): string {
  // Step 1: tilde-shorten
  let display = contractTilde(path, homePath);

  // Step 2: compress if over threshold
  if (display.length <= threshold) return display;

  let prefix = "";
  let rest = display;
  if (display.startsWith("~/")) {
    prefix = "~";
    rest = display.slice(1); // "/Workspace/repos/..."
  }

  const segments = rest.split("/").filter(Boolean);
  if (segments.length <= 3) return display;

  const first = segments[0];
  const tail = segments.slice(-2);
  return `${prefix}/${first}/.../${tail.join("/")}`;
}
