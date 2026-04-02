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
  line: LineRange;     // for single-line: from === to
  snippet: string;     // code content at comment time
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
    ID        string    `json:"id"`
    File      string    `json:"file"`
    Line      LineRange `json:"line"`
    Snippet   string    `json:"snippet"`
    Body      string    `json:"body"`
    Submitted bool      `json:"submitted"`
    CreatedAt string    `json:"createdAt"`
}
```

### Backward Compatibility

Custom `UnmarshalJSON` on `LineRange` detects legacy format `{"from": N, "to": N}` (plain integers) and upgrades to `{"from": {"side": "R", "line": N}, "to": {"side": "R", "line": N}}`. The new format is always written back. Existing review JSON files are auto-migrated on first load.

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

## Staleness Detection

### Side-Aware Re-Anchoring

`detectStaleness` gains the base ref parameter:

```go
func detectStaleness(repoDir string, baseRef string, comments []ReviewComment) []ReviewComment
```

- **R-side comments:** Search the HEAD file via `os.ReadFile` (current behavior)
- **L-side comments:** Search the BASE file via `git show baseRef:path`

`Load()` already receives `head` and `base` — it passes `base` through to `detectStaleness`.

The `findSnippet` algorithm is unchanged — it searches against the correct file version based on comment side. Re-anchored comments preserve the original `side`; only the `line` number changes.

## Sparse Comment Hunks (Auto-Expand)

### Problem

Comments may be anchored to lines outside visible diff context (collapsed hunks or unexpanded regions).

### Approach

On load, identify comments whose anchor lines aren't in any rendered hunk. For each:

1. Fetch a small window (anchor line +/- 3 lines of context) via `fetchFileLines`. For R-side comments, fetch from the HEAD ref. For L-side comments, fetch from the BASE ref. The `fetchFileLines` endpoint already accepts a `ref` parameter.
2. Insert as a new synthetic hunk in the correct position in the hunk list with proper `oldStart`/`newStart`/counts. For L-side hunks, line numbers are computed using the old-file numbering; the old-to-new offset is derived from the nearest existing hunk boundary.
3. Gaps between this hunk and neighbors behave like normal inter-hunk gaps with expand buttons

This reuses existing diff viewer patterns — no new UI concepts.

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

Sort comments by position in rendered hunk order (file index, then hunk index, then line index within hunk) instead of by raw `newLineNumber`.

### `InlineCommentForm` / `InlineCommentCard`

No structural changes. Can now appear after deletion and context lines.

## Not In Scope

- **Multi-line range selection** — tracked in a separate Linear issue. Data model supports it (from !== to) but UI only creates single-line comments.
- **Cross-side ranges** — depends on multi-line selection.
- **`sidePaths` for renames** — single-line comments on renames work with the existing `file` field. Can be added when range selection lands.
- **Split `anchorSnippets`** — the single `snippet` field suffices for single-line comments targeting one side.
