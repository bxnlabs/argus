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
 * Truncate a string to max characters, appending "…" if truncated.
 */
export function truncateRight(s: string, max: number): string {
  if (max <= 0) return "";
  if (s.length <= max) return s;
  if (max === 1) return "…";
  return s.slice(0, max - 1) + "…";
}

/**
 * Compress a path for display in constrained UI space, prioritizing the
 * basename (last segment) over parent directories when truncating.
 *
 * Compression stages:
 * 1. Replace home prefix with ~
 * 2. If ≤3 segments and over threshold, truncate the whole display string
 * 3. Keep first + last 2 segments: ~/first/.../parent/basename
 * 4. Drop first segment: ~/.../parent/basename
 * 5. Drop parent segment: ~/.../basename
 * 6. Truncate basename as last resort: ~/.../basen…
 */
export function compressPath(
  path: string,
  homePath: string,
  threshold: number = 30,
): string {
  // Step 1: tilde-shorten
  const display = contractTilde(path, homePath);

  if (display.length <= threshold) return display;

  let prefix = "";
  let rest = display;
  if (display.startsWith("~/")) {
    prefix = "~";
    rest = display.slice(1); // "/Workspace/repos/..."
  }

  const segments = rest.split("/").filter(Boolean);
  const basename = segments[segments.length - 1];

  if (segments.length <= 3) {
    // For exactly 3 segments, try dropping the middle to preserve basename
    if (segments.length === 3) {
      const result = `${prefix}/.../${basename}`;
      if (result.length <= threshold) return result;
    }
    return truncateRight(display, threshold);
  }

  // Try: ~/first/.../parent/basename
  const first = segments[0];
  const parent = segments[segments.length - 2];
  let result = `${prefix}/${first}/.../${parent}/${basename}`;
  if (result.length <= threshold) return result;

  // Try: ~/.../parent/basename
  result = `${prefix}/.../${parent}/${basename}`;
  if (result.length <= threshold) return result;

  // Try: ~/.../basename (drop parent, prioritize basename)
  result = `${prefix}/.../${basename}`;
  if (result.length <= threshold) return result;

  // Last resort: truncate the basename itself
  return truncateRight(result, threshold);
}

/**
 * Extract "org/repo" (or deeper path for subgroups) from a git remote URL.
 * Supports HTTPS (https://host/path) and SSH (git@host:path) formats.
 * Returns null if the URL doesn't match expected patterns.
 */
export function parseRepoFromRemoteURL(url: string): string | null {
  if (!url) return null;

  let path: string | undefined;

  // SSH SCP-style: user@host:path[.git]
  const sshMatch = url.match(/^[^@]+@[^:]+:(.+)$/);
  if (sshMatch) {
    path = sshMatch[1];
  }

  // HTTPS/HTTP: https://host/path[.git]
  if (!path) {
    try {
      const parsed = new URL(url);
      if (parsed.pathname && parsed.pathname.length > 1) {
        path = parsed.pathname.slice(1); // remove leading "/"
      }
    } catch {
      return null;
    }
  }

  if (!path) return null;

  // Strip trailing .git
  path = path.replace(/\.git$/, "");

  // Must have at least org/repo (two segments)
  if (!path || !path.includes("/")) return null;

  return path;
}
