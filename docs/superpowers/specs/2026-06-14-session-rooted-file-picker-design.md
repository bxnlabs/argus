# Session-rooted file picker (BXN-108)

## Problem

When the file picker opens from the context of an Argus session, it should be
grounded on that session's working directory. Today:

- Fuzzy **search** is already grounded: a `searchPath` prop threads from both
  entry points (`Workspace` → `activeWorkingDirectory`, `TerminalToolbar` /
  Compose → `workingDirectory`) into `FileBrowser`, and on into the
  `/api/node/files/search` request. The Go backend (`filesearch.Search`) walks
  from that `path`, defaulting to `$HOME` when empty.
- **Browsing is not grounded.** The picker opens with an empty input showing
  "Looking for something?" — no listing until you type. Path/browse mode
  (typing `~` or `/`) and the Home button always target `$HOME`, ignoring the
  session's working directory.

BXN-108: open the picker already listing the session's working directory, so
both browsing and search start there.

## Goal & scope

Open the picker rooted at the session working directory. Specifically:

- Empty input shows a live listing of the session working directory.
- Typing a filename fuzzy-searches, grounded at the session working directory
  (already wired via `searchPath`).
- `~` / `/` still enters explicit path mode; the Home button still targets
  `$HOME` (no rebinding — out of scope).

Non-goals:

- Rebinding Home / tilde to the session directory.
- Jailing navigation to the session directory (free navigation is allowed).
- Any backend change.

## Architecture

A **single-file, frontend-only** change in
`web/src/components/FileBrowser.tsx`. No backend changes and no new props.

The existing `searchPath` prop's meaning broadens from "fuzzy-search root" to
"**browse-and-search root**." It is already plumbed to `FileBrowser` from every
session entry point, so no wiring changes are needed.

## Behavior — input → state model

Three states, keyed off the input text and whether `searchPath` is present:

1. **Empty input + `searchPath` present → base listing** *(new)*: list the
   session working directory via the existing `useFilesQuery`, with breadcrumbs,
   parent entry, and drill-in.
2. **Non-path text → fuzzy search**, grounded at `searchPath` (unchanged).
3. **Input starts with `~` or `/` → explicit path mode** (unchanged).

When `searchPath` is **absent** (e.g. `SourcePicker`'s directory mode, or a
session with no working directory), the empty-input state stays as today's
"Looking for something?". The new behavior is gated on `searchPath`, so
directory-mode callers are untouched.

### Internal generalization

Generalize the internal `isPathMode` / `directoryToList` into an effective
`browseDir` + `isBrowsing`:

- `isBrowsing` is true when in explicit path mode **or** in the base listing
  (empty input + `searchPath`).
- `browseDir` is the directory to list: derived from the query in path mode, or
  `searchPath` in the base listing.

The existing listing, breadcrumbs, parent-entry, drill-in, and error UI key off
`isBrowsing` / `browseDir`, so the base listing reuses that machinery with
minimal new surface.

Drilling into a subfolder from the base listing sets the query to that folder's
path (→ path mode). Clearing the input returns to the base listing.

### Navigation root

**Free navigation.** The base listing shows the `..` parent entry and
breadcrumbs up to root, reusing the existing parent/breadcrumb logic as-is. The
session directory is where the picker opens, not a boundary.

## Data flow

- Open → reset effect sets `query = ""` → `browseDir = searchPath` →
  `useFilesQuery(searchPath)` lists it.
- Type a name → debounced `useFileSearchQuery(q, { searchPath })` (grounded;
  already wired).
- Type `~` / `/` → existing path parsing.

## Error & edge handling

- Unreadable / missing `searchPath` directory → existing error UI
  ("Permission denied" for 403, otherwise "Could not load directory"), now
  reachable from the base listing.
- Normalize a trailing slash on `searchPath` before listing.
- Reopening the picker resets to the base listing (existing open-reset effect
  clears the query).

## Testing

Vitest component tests on `FileBrowser`:

- Base listing appears and lists `searchPath` contents when `searchPath` is set
  and the input is empty.
- Falls back to "Looking for something?" when `searchPath` is absent.
- Typing a name switches to grounded fuzzy search (request carries the
  `searchPath`).
- Drill into a subfolder, then clear the input → returns to the base listing.
- Error state renders from the base listing when the listing query fails.
- `SourcePicker` (directory mode, no `searchPath`) is unchanged.

Mock the files / search queries (`useFilesQuery`, `useFileSearchQuery`)
following the existing FileBrowser/FilePicker test patterns.

## Files

- `web/src/components/FileBrowser.tsx` — implementation.
- `web/src/components/FileBrowser.test.tsx` (or the existing FilePicker test
  file, matching repo precedent) — tests.
