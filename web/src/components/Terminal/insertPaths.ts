import { shellEscape } from "@/lib/shell";

export interface InsertPathsResult {
  /** The full new textarea value. */
  text: string;
  /** Where the caret should sit afterwards — just past the inserted paths. */
  cursor: number;
}

/**
 * Insert shell-escaped paths into `text`, replacing the range
 * [selectionStart, selectionEnd). Separator spaces are added only where the
 * adjacent text needs them, so the result never has doubled spaces.
 */
export function insertPaths(
  text: string,
  selectionStart: number,
  selectionEnd: number,
  paths: string[],
): InsertPathsResult {
  if (paths.length === 0) return { text, cursor: selectionEnd };

  const insert = paths.map(shellEscape).join(" ");
  const before = text.slice(0, selectionStart);
  const after = text.slice(selectionEnd);

  const needsLeadingSpace = before.length > 0 && !before.endsWith(" ");
  const needsTrailingSpace = after.length > 0 && !after.startsWith(" ");

  return {
    text:
      before +
      (needsLeadingSpace ? " " : "") +
      insert +
      (needsTrailingSpace ? " " : "") +
      after,
    cursor: before.length + (needsLeadingSpace ? 1 : 0) + insert.length,
  };
}
