# Git Diff Context Expansion

**Linear Issue:** BXN-30
**Date:** 2026-03-31

## Overview

Add GitHub-style expand buttons to the diff viewer so users can incrementally reveal surrounding code above, below, and between hunks. Applies to all diff views: changes tab (staged/unstaged), compare/review view, and commit history.

## Design Decisions

- **Interaction model:** GitHub-style inline expand buttons (not a configurable slider)
- **Backend strategy:** On-demand line fetching per expansion click (not upfront large diffs)
- **Initial context:** Reduce from `-U20` to git's default `-U3` so the diff starts compact and expand buttons are the primary way to reveal context
- **Expand increment:** 20 lines per click
- **Hunk merging:** When expanded context fully bridges the gap between two adjacent hunks, they merge into one
- **Line source per view:** Working tree for changes tab (matching the combined `HEAD → working tree` diff), git ref for compare/commit views

## Backend: File Lines Endpoint

### `GET /api/git/file-lines`

**Query parameters:**

| Param   | Required | Description                                              |
|---------|----------|----------------------------------------------------------|
| `path`  | yes      | Repository directory path                                |
| `file`  | yes      | File path relative to repo root                          |
| `start` | yes      | First line number (1-based, inclusive, **postimage/new-side** coordinates) |
| `end`   | yes      | Last line number (1-based, inclusive, **postimage/new-side** coordinates) |
| `ref`   | no       | Git ref to read from. Omit to read from working tree.    |

**Ref resolution by view:**

| View             | `ref` value        | Source                          |
|------------------|--------------------|---------------------------------|
| Changes tab      | _(omitted)_        | Working tree file on disk (matches the `HEAD → working tree` diff produced by `GetWorkingDiff`) |
| Compare view     | `<commit-oid>`     | Git object (use the exact commit OID from the diff response, not symbolic `HEAD`) |
| Commit history   | `<commit-hash>`    | Git object (already immutable)  |

> **Snapshot coherence:** Expanded lines should come from the same snapshot that produced the displayed diff. The strength of this guarantee varies by view:
>
> - **Compare/history views (strict):** Use the exact commit OID returned with the diff, not symbolic refs. Since commits are immutable, snapshot coherence is guaranteed.
> - **Changes tab (best-effort):** Reads from the working tree, which is inherently mutable. A file can change between diff render and the next expand click. The diff auto-refreshes every 5 seconds — each working diff response includes a fingerprint, and when a refresh produces a **changed** fingerprint, the reducer resets to freshly parsed hunks and in-flight expand requests are cancelled (see "Diff fingerprint" in the reducer section). Unchanged fingerprints preserve expanded state. This means stale expanded lines can persist for up to one refresh cycle (5 s) before self-correcting. See "Accepted Trade-offs" for rationale and upgrade path.
>
> The changes tab currently renders one combined `HEAD → working tree` diff, not separate staged/unstaged diffs. If separate staged/unstaged panes are introduced later, each pane will need its own explicit source contract (e.g., `:0` for staged-only).

**Response:**

```json
{
  "lines": ["line 1 content", "line 2 content"],
  "start": 10,
  "end": 29,
  "totalLines": 150
}
```

`totalLines` tells the frontend whether more lines exist beyond the current viewport, controlling expand button visibility.

**Error responses:**

| Status | Condition | Retryable | Client action |
|--------|-----------|-----------|---------------|
| `400`  | Missing/invalid query params, `start > end`, span > 500 | No | Show validation error |
| `404`  | File not found in working tree or at given ref | No | Hide expand button for this file |
| `413`  | File exceeds size limit (> 2 MB) | No | Hide expand button; show "file too large" hint |
| `422`  | Binary file detected | No | Hide expand button |
| `500`  | Internal / git failure | Yes | Show inline retry prompt |

**Implementation location:** New `GetFileLines` function in `internal/node/git/operations.go`, new handler in `internal/node/api/git.go`.

- **Working tree reads:** Use `os.Open` with line-oriented streaming — read lines sequentially and stop at `end`, avoiding full-file materialization.
- **Ref-based reads:** Three-step resolution: (1) resolve the file to a blob OID with `git rev-parse <ref>:<file>^{blob}` — this validates existence, confirms the object is a blob (not a tree entry), and returns the blob OID in one call; (2) size-check with `git cat-file -s <blob-oid>`; (3) stream with `git cat-file blob <blob-oid>` piped through a line scanner that stops at the requested `end` line. This avoids materializing the full blob in memory — critical because `diffMaxBuffer` is 5 MB and `longTimeout` is 30 s, so large files would otherwise hit these caps on every expand click.
- **Size guard:** Before reading, check the blob/file size (`git cat-file -s <oid>` for refs, `os.Stat` for working tree). Reject files larger than 2 MB with a `413` response. This is a safety valve — the streaming approach handles normal large files, but pathological cases (e.g., vendored binaries misidentified as text) should fail fast.

**Validation requirements:**

- `path`: Validate with `shared.SafeExpandPath()` (consistent with all other git API handlers)
- `file`: Validation depends on read mode:
  - **Working tree reads** (no `ref`): Validate with `sanitizeFilePath()` — full path traversal, absolute path, and symlink escape checks against the current filesystem.
  - **Ref-based reads** (`ref` provided): Use **lexical-only** validation (reject `..` components and absolute paths). Skip symlink resolution — `sanitizeFilePath()` resolves symlinks against the current working tree, which can reject valid historical paths whose parent directories have since been restructured or symlinked. Existence and type validation is handled by the `git rev-parse <ref>:<file>^{blob}` step in the read flow (see above).
- `start`/`end`: Validate `1 ≤ start ≤ end`; cap maximum span to prevent abuse (e.g., 500 lines)
- `ref`: If provided, first validate that the input matches a full hex OID pattern (`/^[0-9a-f]{40}$/`). This rejects symbolic refs (like `HEAD`, `main`) and short hashes at the input boundary — `git cat-file -t` resolves symbolic refs before reporting type, so it alone cannot enforce the "exact OID only" contract. After the format check, validate with `git cat-file -t <ref>` and require the result to be `commit`. This enforces the snapshot invariant: only immutable, fully-qualified commit OIDs are accepted.

## Backend: Diff Response Contract Changes

The existing diff endpoints must be extended to support expansion. Two additions are needed across all diff response types:

### Per-file `totalLines`

Add a `totalLines` map (keyed by canonical diff path) to each diff response type:

- `WorkingDiffResult`: Add `TotalLines map[string]int` — computed from the working tree file on disk at diff time. **Important:** `totalLines` is the logical line count (number of lines a scanner would yield), including a final unterminated line if present. This differs from `wc -l`, which counts newline characters and would undercount by one for files without a trailing newline. Use the same line-counting semantics as `GetFileLines` to ensure consistency.
- `CompareResult`: Add `TotalLines map[string]int` — computed from the postimage blob at the head commit.
- Commit full-diff response (`/api/git/history/{hash}/full-diff`): Add `TotalLines map[string]int` — computed from the postimage blob at the commit.

**Path key rules:** Use the new (postimage) path for renames. Use the old (preimage) path for deletes. This matches `ParsedDiff.newFile` for renames and `ParsedDiff.oldFile` for deletes.

### Full commit OIDs in compare response

`CompareResult` currently returns 7-character truncated hashes in `BaseRef` and `HeadRef` (via `truncateRef()`). The expansion feature requires exact commit OIDs for snapshot invariance — truncated hashes are not guaranteed unique and cannot be safely passed to `git show <ref>:<file>`.

**Change:** Stop truncating. Return full 40-character OIDs in `BaseRef` and `HeadRef`. The frontend already displays these in a truncated form in the UI — add client-side truncation for display while using the full OID for API calls.

> **Note:** The commit history view already returns full hashes in `CommitSummary.Hash`, so no changes are needed there.

## Frontend: Diff State Management

### Reducer-based mutable diff state

The initial `parseDiff()` call remains unchanged and produces the starting `ParsedDiff`. A shared `useExpandableDiff(initialHunks, totalLines, expansionContext)` custom hook encapsulates the reducer and expand handler. All three diff views consume this hook:

- **`GitPanel`** (changes tab): Passes `ExpansionContext` with no `ref` (working tree reads). Handles fingerprint-based reset on auto-refresh.
- **`CompareView`**: Passes `ExpansionContext` with the full commit OID from `CompareResult.HeadRef`.
- **`CommitHistory`** (commit detail diff): Passes `ExpansionContext` with the full commit hash from `CommitSummary.Hash`. Since commits are immutable, no refresh/reset logic is needed.

Each view passes its own `ExpansionContext` while sharing the same reducer logic. `UnifiedDiff` receives the expanded diff as a read-only prop.

> **Why parent-owned:** `CompareView` derives comment snippets and mobile selected-line context from `parsedDiffs` (see `handleAddComment` and `MobileCommentSheet`). If expanded lines only existed in `UnifiedDiff` local state, commenting on a newly revealed context line would fail — the parent wouldn't see it. The expanded diff state must be the single source of truth for all line lookups, comment anchoring, and snippet extraction.

**Actions:**

- `EXPAND_UP` — prepend context lines before a hunk
- `EXPAND_DOWN` — append context lines after a hunk
- `EXPAND_BETWEEN` — insert context lines between two adjacent hunks; merge hunks if the gap is fully bridged

**Generation counter:** The reducer maintains a monotonically increasing generation number. The generation increments on:

- Diff refresh/reset (fingerprint change on changes tab, new data on compare/history)
- Every accepted state-changing expansion (any `EXPAND_UP`, `EXPAND_DOWN`, or `EXPAND_BETWEEN` that modifies the hunk array)

Every expand request carries the generation at the time it was issued. The reducer rejects any response whose generation doesn't match current state. Because the generation increments on every mutation — not just refreshes — this handles both cross-snapshot staleness and same-generation structural changes (e.g., request A merges hunks, shifting indices; request B from the pre-merge generation is correctly rejected when it arrives).

**Hunk merging:** When `EXPAND_BETWEEN` fetches lines that completely close the gap between two hunks, the reducer merges them into a single hunk: hunk A lines + new context lines + hunk B lines, with the header recomputed:

- `oldStart = hunkA.oldStart`
- `oldCount = hunkA.oldCount + gapOldLines + hunkB.oldCount`
- `newStart = hunkA.newStart`
- `newCount = hunkA.newCount + gapNewLines + hunkB.newCount`

Since the generation increments on every accepted expansion (including merges), in-flight requests issued before the merge are automatically rejected — no separate stable identity tracking is needed.

The merged hunk's `header` string is reconstructed from the computed fields: `@@ -${oldStart},${oldCount} +${newStart},${newCount} @@`. Any trailing context text from the original headers (e.g., function names) is dropped — the renderer should derive display headers from the structured `oldStart`/`oldCount`/`newStart`/`newCount` fields rather than parsing the header string.

**Diff fingerprint (changes tab only):** The working diff response must include a fingerprint (e.g., a hash of the raw diff string) so the frontend can detect whether a 5-second auto-refresh produced a new diff or a no-op. On refresh:

- **Fingerprint changed:** Reset the reducer to freshly parsed hunks (increment generation, discard expanded state), and cancel in-flight expand requests via `AbortController`.
- **Fingerprint unchanged:** Keep expanded state as-is — no unnecessary reset.

This replaces the earlier "discard on every refresh" approach, which would cause jarring UX on no-op refreshes where the user is actively expanding context.

**Initial metadata — `totalLines`:** The parsed diff alone doesn't tell us how long the file is, so the "expand down" button after the last hunk can't be shown correctly on first render. The backend diff responses include `totalLines` per file (see "Backend: Diff Response Contract Changes" above). The frontend initializes the reducer with this value so expand buttons render correctly without an extra metadata fetch. For the file-lines endpoint, `totalLines` is also returned per response as already specified.

### Line number derivation for expanded context

The `/api/git/file-lines` endpoint returns lines with postimage (new-side) coordinates only. The reducer must derive `oldLineNumber` for each expanded `DiffLine`. For context lines (which exist identically in both old and new versions), old-side and new-side numbers differ by a constant offset within a hunk.

**Offset formula:** For a given hunk boundary, the old-side offset is:

```
oldOffset = (hunk.oldStart + hunk.oldCount) - (hunk.newStart + hunk.newCount)
```

This represents the cumulative difference between old and new line numbering at the end of the hunk. For expanded context lines adjacent to this hunk, `oldLineNumber = newLineNumber + oldOffset`.

**Per action:**

- `EXPAND_UP` (before hunk at index `i`): Use the offset from the end of hunk `i-1` (or `0` if `i == 0`, meaning old and new are in sync before the first hunk).
- `EXPAND_DOWN` (after hunk at index `i`): Use the offset from the end of hunk `i`.
- `EXPAND_BETWEEN` (between hunk `i` and hunk `i+1`): Use the offset from the end of hunk `i`. All gap lines share this offset since no additions or deletions occur in context.

**Worked example — mixed hunk:**

Given hunk `@@ -10,5 +12,7 @@` (2 more additions than deletions), the offset at the end of this hunk is `(10+5) - (12+7) = -4`. Expanding 20 lines below this hunk: new-side lines 19–38 get old-side numbers `19 + (-4)` = 15 through `38 + (-4)` = 34.

**Alternative:** If dual-sided derivation proves error-prone during implementation, the backend can return both `oldStart` and `newStart` per response, removing the need for client-side offset computation. This is a fallback — try the offset approach first.

**Expansion context:** Each diff view provides an expansion context object to the expand handler:

```typescript
interface ExpansionContext {
  repoPath: string;   // Repository directory path (for the `path` query param)
  filePath: string;    // File path relative to repo root (for the `file` query param)
  ref?: string;        // Git ref — omit for working tree (changes tab), commit OID for compare/history
}
```

The parent view (GitPanel, CompareView, or CommitHistory) constructs this from the current view's state and passes it down. `UnifiedDiff` does not need to know about refs or repo paths — it only invokes an `onExpand` callback provided by the parent.

### Reducing initial context

Change all `-U20` flags in the backend to `-U3` (git's default). This makes the initial diff compact. The 7 locations:

- `internal/node/git/compare.go:82`
- `internal/node/git/operations.go:123-125` (staged and unstaged)
- `internal/node/git/operations.go:195-204` (working tree diffs)
- `internal/node/git/history.go:261` (commit full diff)

> **Note:** The untracked file diff at `operations.go:134` (`git diff --no-index /dev/null <file>`) is excluded — new/untracked files produce a single full-content addition hunk regardless of `-U` value, so changing it has no effect.

## Frontend: Expand UI

### Expand button placement

Expand buttons appear as small interactive elements anchored to the gutter (line number area), following GitHub's style:

1. **Before the first hunk** — if the hunk doesn't start at line 1, show expand-up button
2. **Between adjacent hunks** — show expand button in the gap between hunks
3. **After the last hunk** — if the hunk doesn't reach end of file, show expand-down button

### Behavior

- Each click fetches 20 lines in the appropriate direction
- The expand button disappears when no more lines exist in that direction (determined by `totalLines` from the API and line 1 boundary)
- While fetching, the button shows a subtle loading indicator and is **disabled** to prevent duplicate requests
- Fetched lines are inserted as `context` type `DiffLine`s with correct line numbers

**Concurrency rules:**

- **Disable during flight:** When an expand request is in flight, disable the clicked expander (and any adjacent expanders whose target gap would be affected by a hunk merge).
- **Generation-based rejection:** Each expand request carries the reducer's current generation number. The generation increments on every accepted expansion and on refresh resets. The reducer discards any response whose generation doesn't match current state. This handles: (a) out-of-order responses from rapid clicks, (b) responses arriving after a diff refresh reset, and (c) same-generation structural changes (e.g., a merge shifts hunk indices before a concurrent response arrives — the merge incremented the generation, so the stale response is rejected).
- **Refresh cancellation (changes tab):** When a working-tree diff refresh produces a new fingerprint, cancel all in-flight expand requests via `AbortController` and increment the generation. This prevents stale expanded lines from being inserted into fresh diff state.

### Styling

Small clickable button/icon in the gutter area. The rest of the row is a subtle separator line. Distinct from diff content but consistent with existing hunk header aesthetics.

**Accessibility:** Expand controls must be `<button>` elements (not `<div>` with click handlers), keyboard-focusable with visible focus states, and include `aria-label` attributes (e.g., "Show 20 more lines above"). Minimum touch target size for mobile.

## Accepted Trade-offs

### Changes-tab expand without server-side snapshot token

The changes tab reads from the mutable working tree. Between a diff render and the next 5-second refresh, the file can change, and an expand request issued during that window will read lines from the newer version. The generation counter + diff fingerprint mechanism detects this on the _next_ refresh (fingerprint changes → reset expanded state), but does not prevent the stale expand from landing in the first place.

A server-side snapshot token (e.g., requiring the backend to cache or pin working-tree state per diff render) would close this gap, but adds disproportionate complexity for a local development tool where the user is the sole editor. The 5-second refresh cycle limits the staleness window, and the fingerprint reset ensures it self-corrects.

If this proves problematic in practice (e.g., users report visually inconsistent expanded context), the fix is to include a file mtime or content hash in the expand request and validate it server-side with a `409 Conflict` response.

### No upfront expandable/non-expandable signal per file

The current design shows expand buttons for all non-binary, non-deleted, non-conflict files. Files that exceed the 2 MB size limit or are binary will only fail on first expand click (with `413` or `422`). An upfront `expandable: boolean` field per file in diff responses would prevent this, but requires the backend to check every file's size and type at diff time — an O(files) cost on every diff response.

For MVP, the error-on-first-click approach is acceptable. The error response hides the expand button permanently for that file, so the user sees the failure only once. If the 2 MB limit causes frequent false starts, add per-file metadata to diff responses in a follow-up.

## Error Handling & Edge Cases

### Errors

- **File deleted in target ref:** No expand buttons shown for deleted files. (Expanding deleted files would require old-side/preimage line fetching via `git show <parent>:<file>` — a different API contract. Accepted as an MVP limitation; can be added later if needed.)
- **Unmerged/conflict files:** No expand buttons — conflict markers make line mapping unreliable
- **Renamed files:** Expand using the new (postimage) path
- **Binary files:** No expand buttons (already skipped in diff rendering). The backend should also reject file-lines requests for binary files with a `422` response, rather than relying solely on the client to prevent the call.
- **Fetch failure:** Brief inline error on the expand row; retry on next click

### Edge cases

- **Expand up from first hunk:** Clamp `start` to line 1
- **Expand down from last hunk:** Clamp `end` to `totalLines`
- **Gap between hunks smaller than 20 lines:** Fetch only the actual gap; merge hunks
- **Gap of 0 lines:** No expand row shown (hunks already adjacent)
- **File with no trailing newline:** The diff parser does not currently handle `\ No newline at end of file` markers (they fall through as context lines). Expansion should insert context lines above the last hunk line regardless; no special marker handling is needed since expanded lines are always full context lines from the source file

## Testing

### Backend

- Unit tests for `GetFileLines`: working tree reads, ref-based reads, line range clamping, invalid refs, missing files, binary file rejection, oversized file rejection (413)
- HTTP handler test for `/api/git/file-lines`: query param validation, error responses for all status codes (400, 404, 413, 422, 500), using `httptest.NewServer` with a temp git repo
- Snapshot tests verifying `-U3` produces expected compact diffs (ensure no regressions in existing diff output)

### Frontend

- Unit tests for the diff reducer: `EXPAND_UP`, `EXPAND_DOWN`, `EXPAND_BETWEEN`, hunk merging with correct `oldStart`/`oldCount`/`newStart`/`newCount` recomputation, old-side line number derivation using offset formula, generation-based stale response rejection
- Component tests for expand buttons: correct placement, disappearance when no more lines, loading state during fetch
- Integration tests:
  - **Refresh reset:** Verify that a changed diff fingerprint resets expanded state and cancels in-flight requests; verify that an unchanged fingerprint preserves expanded state
  - **Comment on expanded line:** Expand context, click an expanded line, verify `handleAddComment` correctly extracts the snippet and anchors the comment
  - **Stale response rejection:** Simulate an expand response arriving after a generation increment (via refresh or rapid clicks); verify it is discarded
