/** Shell-escape a path so it's safe to paste into a terminal command line. */
export function shellEscape(s: string): string {
  if (/^[a-zA-Z0-9_./:@~-]+$/.test(s)) return s;
  return "'" + s.replace(/'/g, "'\\''") + "'";
}
