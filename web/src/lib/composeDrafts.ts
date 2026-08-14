// Compose-bar drafts, persisted client-side.
//
// A draft is text the user has typed but not sent, and nothing about it belongs
// to the server — yet it used to live only in ComposeBar's own state, so any
// unmount destroyed it. On a weak network that happened for reasons the user
// never asked for: a single failed 5s summary poll flipped the active node
// offline and swapped the whole workspace out, taking the half-typed message
// with it. Persisting here means the draft outlives the mount entirely — the
// remount, a tab switch, a node switch, and a reload.
//
// Scoped per node (id+origin, the same token as tab state and the query cache)
// so two nodes' drafts can never surface in each other's compose bar, and keyed
// within that scope by the id of the tab holding the compose box. The text
// belongs to the box rather than to the session behind it, so pointing the tab
// at a different session leaves the draft alone, and closing the tab is what
// disposes of it (see TabContext.closeTab).
//
// ---------------------------------------------------------------------------
// Why a key per draft rather than one map per node
// ---------------------------------------------------------------------------
// localStorage.setItem is synchronous on the UI thread and costs in proportion
// to the payload. Holding every draft in one JSON map would mean a single typed
// character re-serialises and rewrites every OTHER draft in the scope with it.
// Per-key, a keystroke writes only the draft being edited — which is what lets
// the write happen on every keystroke rather than behind a debounce, so no
// teardown the browser doesn't announce can lose the last characters typed.

const KEY_PREFIX = "argus-compose-draft:";

// Both halves are encoded because both carry colons of their own — a node scope
// is itself "<id>:<url>". Without encoding, "a" + "b:c" and "a:b" + "c" would
// address the same entry.
export function draftStorageKey(nodeScope: string, key: string): string {
  return `${KEY_PREFIX}${encodeURIComponent(nodeScope)}:${encodeURIComponent(key)}`;
}

/**
 * The stored draft for `key`, or "" when there is none.
 *
 * Never throws: this is called during render, from ComposeBar's useState
 * initializer, where a throw would blank the workspace.
 */
export function loadDraft(nodeScope: string, key: string): string {
  try {
    return localStorage.getItem(draftStorageKey(nodeScope, key)) ?? "";
  } catch {
    // Storage unavailable (private mode).
    return "";
  }
}

/**
 * Stores `text` for `key`.
 *
 * Called on every keystroke. That is deliberate: the write is one small
 * setItem against one key, and paying it immediately is what makes the draft
 * durable across teardowns React never sees — a reload, a tab close, an OS
 * kill — including the teardown that follows CLEARING the box, which a
 * deferred write would lose and so resurrect text the user deleted.
 */
export function saveDraft(nodeScope: string, key: string, text: string): void {
  try {
    const storageKey = draftStorageKey(nodeScope, key);
    // An emptied input is not a draft, and storing blanks would leave entries
    // behind for every box the user ever cleared.
    if (text === "") localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, text);
  } catch {
    // Storage unavailable or full — the draft still lives in component state
    // for this mount, so a failed write costs persistence, not the text.
  }
}
