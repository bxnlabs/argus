# BXN-67: Inline Comments on All Diff Line Types

**Date:** 2026-04-02
**Linear Issue:** [BXN-67](https://linear.app/bxnlabs/issue/BXN-67/improved-inline-comments-in-diff-view)
**Branch:** `jeev/bxn-67-deletions-inline-comments`

## Problem

Inline comments can only be placed on addition lines. The comment system anchors everything to `newLineNumber`, which is `null` for deletion lines. Context lines and expanded context lines are also not commentable. This limits the usefulness of code reviews — reviewers often need to comment on deleted/updated code or unchanged context.

## Solution

Make the comment data model side-aware using GitHub's `L` (old/left/base) and `R` (right/new/head) convention. Every diff line becomes commentable, and comments on lines outside visible diff context are surfaced via sparse synthetic hunks.

## Data Model

### TypeScript (`web/src/types/review.ts`)

```ts
export type DiffSide = "L" | "R";

export interface DiffPosition {
  side: DiffSide; // L = old/base file, R = new/head file
  line: number;   // 1-based; refers to oldLineNumber when "L", newLineNumber when "R"
}

export interface LineRange {
  from: DiffPosition;
  to: DiffPosition;
}

export interface ReviewComment {
  id: string;
  file: string;       // canonical path key from getDiffPathKey()
  oldPath?: string;    // original path for renamed files (L-side resolution)
  line: LineRange;     // for single-line: from === to
  snippet: string;     // code content at comment time
  snippetContext?: string; // 1-2 lines above/below snippet for re-anchoring disambiguation
  body: string;
  submitted: boolean;
  createdAt: string;
}
```

### Go (`internal/node/git/review/review.go`)

```go
type DiffSide string

const (
    DiffSideLeft  DiffSide = "L"
    DiffSideRight DiffSide = "R"
)

type DiffPosition struct {
    Side DiffSide `json:"side"`
    Line int      `json:"line"`
}

type LineRange struct {
    From DiffPosition `json:"from"`
    To   DiffPosition `json:"to"`
}

type ReviewComment struct {
    ID             string    `json:"id"`
    File           string    `json:"file"`
    OldPath        string    `json:"oldPath,omitempty"` // original path for renamed files
    Line           LineRange `json:"line"`
    Snippet        string    `json:"snippet"`
    SnippetContext string    `json:"snippetContext,omitempty"` // surrounding lines for disambiguation
    Body           string    `json:"body"`
    Submitted      bool      `json:"submitted"`
    CreatedAt      string    `json:"createdAt"`
}
```

### Backward Compatibility

Custom `UnmarshalJSON` on `LineRange` detects legacy format `{"from": N, "to": N}` (plain integers) and upgrades to `{"from": {"side": "R", "line": N}, "to": {"side": "R", "line": N}}`. The new format is always written back. Existing review JSON files are auto-migrated on first load.

### Rename Support

When creating a comment on a renamed file with side `"L"`, populate `oldPath` from `ParsedDiff.oldFile`. This is needed for:
- Staleness detection (`git show baseRef:oldPath`)
- Sparse hunk fetches (`fetchFileLines` with the old path and base ref)

When `oldPath` is absent, the system uses `file` for both sides (correct for non-renamed files).

## Comment Anchoring & Indexing

### Anchor Keys

Each rendered diff line exposes anchor keys based on its type:

| Line type | Anchor keys |
|-----------|-------------|
| Deletion  | `L{oldLineNumber}` |
| Addition  | `R{newLineNumber}` |
| Context   | `L{oldLineNumber}` and `R{newLineNumber}` |
| Header    | none (not commentable) |

### Comment Lookup

Replace `Map<number, ReviewComment[]>` (keyed by `newLineNumber`) with `Map<string, ReviewComment[]>` keyed by anchor key string (e.g., `"L43"`, `"R12"`).

Index each comment by `"{comment.line.to.side}{comment.line.to.line}"`.

When rendering a diff line:
- Deletion row: look up `"L" + oldLineNumber`
- Addition row: look up `"R" + newLineNumber`
- Context row: look up both, merge results

### `isCommentable` Logic

Any line with at least one non-null line number and non-empty content is commentable. Headers remain non-commentable.

### `onLineClick` Callback

Changes from passing `number` to passing `DiffPosition`:
- Deletion: `{ side: "L", line: oldLineNumber }`
- Addition: `{ side: "R", line: newLineNumber }`
- Context: `{ side: "R", line: newLineNumber }` (prefer right side, matches current behavior for existing comments)

## Snippet Extraction

When creating a comment, extract `line.content` from the diff line matching the `DiffPosition`:
- Side `"R"`: match by `newLineNumber`
- Side `"L"`: match by `oldLineNumber`

Also extract `snippetContext`: the content of 1-2 lines above and below the anchor line in the rendered diff. This surrounding context is stored alongside the snippet and used during re-anchoring to disambiguate common single-line snippets (e.g., `}`, `return nil`).

## Staleness Detection

### Side-Aware Re-Anchoring Against Immutable Refs

`detectStaleness` gains both ref parameters and anchors against immutable commit OIDs, not the working tree or mutable branch names:

```go
func detectStaleness(repoDir string, headRef string, baseRef string, comments []ReviewComment) []ReviewComment
```

- **R-side comments:** Search via `git show headRef:path` (not `os.ReadFile` — the working tree may have uncommitted changes that don't match the compare view)
- **L-side comments:** Search via `git show baseRef:path`. For renamed files, use `comment.OldPath` if set, otherwise `comment.File`.

`Load()` already receives `head` and `base` as branch names. It must resolve these to commit OIDs before calling `detectStaleness`. The compare API already computes `mergeBase` and `headRef` OIDs (`compare.go:61,75`); these should be passed through or re-derived.

### Enhanced Snippet Matching

`findSnippet` is extended to use `snippetContext` when available. When multiple matches for the snippet are found within the 50-line threshold, check the surrounding context against `snippetContext` to disambiguate. If the context doesn't match any candidate, mark the comment as stale rather than silently relocating it to a wrong position. Single-match cases continue to re-anchor as before.

Re-anchored comments preserve the original `side`; only the `line` number changes.

## Sparse Comment Hunks (Auto-Expand)

### Problem

Comments may be anchored to lines outside visible diff context (collapsed hunks or unexpanded regions).

### Approach

On load, identify comments whose anchor lines aren't in any rendered hunk. For each:

1. Fetch a small window (anchor line +/- 3 lines of context) via `fetchFileLines`. For R-side comments, fetch from the HEAD ref. For L-side comments, fetch from the BASE ref. The `fetchFileLines` endpoint already accepts a `ref` parameter.
2. Insert as a new synthetic hunk in the correct position in the hunk list with proper `oldStart`/`newStart`/counts. For L-side hunks, line numbers are computed using the old-file numbering; the old-to-new offset is derived from the nearest existing hunk boundary.
3. Gaps between this hunk and neighbors behave like normal inter-hunk gaps with expand buttons

This reuses existing diff viewer patterns — no new UI concepts.

### Hunk Identity

Synthetic comment hunks are keyed by a stable identity (`oldStart-newStart` pair at creation time) rather than array index. This prevents index instability when hunks are inserted — existing expand loading state and error badges remain attached to the correct hunk. Synthetic comment hunks are initially non-expandable; the gap boundaries between them and adjacent hunks use the standard expand buttons.

### Fetch Failure Fallback

If `fetchFileLines` fails for a hidden comment (404, 413, network error), insert a minimal placeholder row at the comment's logical position. The placeholder displays the comment card with an error indicator ("context unavailable") and an optional retry button. The comment remains navigable via `CommentNav` and its content is readable — it just lacks the surrounding code context.

### On Comment Navigation

When `CommentNav` prev/next targets a comment whose anchor isn't visible, create the sparse hunk first, then scroll into view.

## UI Changes

### `DiffLineRow`

- All non-header lines with non-empty content are clickable
- Hover affordance (comment icon / highlight) on all commentable lines
- Click handler passes `DiffPosition` instead of line number

### `CompareView`

- `handleLineClick` signature: `(position: DiffPosition)` instead of `(line: number)`
- `activeComment` state: `{ file: string, position: DiffPosition }` instead of `{ file, from, to }`
- `handleAddComment` builds `ReviewComment` using `DiffPosition`, extracts snippet from correct side

### `MobileCommentSheet`

Update active line filtering to match by the appropriate line number based on side.

### `CommentNav`

Sort comments by a stable logical order independent of rendering state: `(file index in diff list, side, line number)`. File order comes from the deterministic diff file list. Within a file, sort by line position: L-side comments by `oldLineNumber`, R-side by `newLineNumber`. This works whether or not the comment's hunk is rendered, avoiding the chicken-and-egg problem of sorting by rendered position before sparse hunks exist.

### `InlineCommentForm` / `InlineCommentCard`

No structural changes. Can now appear after deletion and context lines.

## Input Validation

The POST handler (`api/review.go`) validates comment structure on save:
- `line.from.side` and `line.to.side` must be `"L"` or `"R"`
- `line.from.line` and `line.to.line` must be `> 0`
- `line.from` must equal `line.to` (this iteration only supports single-line comments)
- `file` must pass existing path traversal validation

Invalid comments are rejected with HTTP 400. This prevents malformed payloads from being persisted and causing render/staleness errors downstream.

## Not In Scope

- **Multi-line range selection** — tracked in a separate Linear issue. Data model supports it (from !== to) but UI only creates single-line comments.
- **Cross-side ranges** — depends on multi-line selection.
- **Full `sidePaths` struct** — the simpler `oldPath` field covers the rename case for single-line comments. A full `sidePaths { left, right }` struct can be added when range selection lands.
- **Split `anchorSnippets`** — the single `snippet` + `snippetContext` fields suffice for single-line comments targeting one side.
