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
- **Line source per view:** Working tree for unstaged changes, git index for staged, git ref for compare/commit views

## Backend: File Lines Endpoint

### `GET /api/git/file-lines`

**Query parameters:**

| Param   | Required | Description                                              |
|---------|----------|----------------------------------------------------------|
| `path`  | yes      | Repository directory path                                |
| `file`  | yes      | File path relative to repo root                          |
| `start` | yes      | First line number (1-based, inclusive)                    |
| `end`   | yes      | Last line number (1-based, inclusive)                     |
| `ref`   | no       | Git ref to read from. Omit to read from working tree.    |

**Ref resolution by view:**

| View             | `ref` value        | Source                          |
|------------------|--------------------|---------------------------------|
| Unstaged changes | _(omitted)_        | Working tree file on disk       |
| Staged changes   | `:0`               | Git index (`git show :0:<file>`)  |
| Compare view     | `HEAD` (or head ref) | Git object                     |
| Commit history   | `<commit-hash>`    | Git object                      |

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

**Implementation location:** New `GetFileLines` function in `internal/node/git/operations.go`, new handler in `internal/node/api/git.go`. Working tree reads use `os.Open`; ref-based reads use `git show <ref>:<file>` and extract the requested line range.

## Frontend: Diff State Management

### Reducer-based mutable diff state

The initial `parseDiff()` call remains unchanged and produces the starting `ParsedDiff`. A `useReducer` in the `UnifiedDiff` component (or a parent wrapper) takes ownership of the `DiffHunk[]` array from there.

**Actions:**

- `EXPAND_UP` — prepend context lines before a hunk
- `EXPAND_DOWN` — append context lines after a hunk
- `EXPAND_BETWEEN` — insert context lines between two adjacent hunks; merge hunks if the gap is fully bridged

**Hunk merging:** When `EXPAND_BETWEEN` fetches lines that completely close the gap between two hunks, the reducer merges them into a single hunk: hunk A lines + new context lines + hunk B lines, with the header updated to span the full range.

**Ref resolution:** Each diff view passes a `ref` string (or undefined for working tree) to the expand handler so it knows which ref to use when calling the file-lines endpoint.

### Reducing initial context

Change all `-U20` flags in the backend to `-U3` (git's default). This makes the initial diff compact. The 8 locations:

- `internal/node/git/compare.go:82`
- `internal/node/git/operations.go:123-125` (staged and unstaged)
- `internal/node/git/operations.go:134` (untracked)
- `internal/node/git/operations.go:195-204` (working tree diffs)
- `internal/node/git/history.go:261` (commit full diff)

## Frontend: Expand UI

### Expand button placement

Expand buttons appear as small interactive elements anchored to the gutter (line number area), following GitHub's style:

1. **Before the first hunk** — if the hunk doesn't start at line 1, show expand-up button
2. **Between adjacent hunks** — show expand button in the gap between hunks
3. **After the last hunk** — if the hunk doesn't reach end of file, show expand-down button

### Behavior

- Each click fetches 20 lines in the appropriate direction
- The expand button disappears when no more lines exist in that direction (determined by `totalLines` from the API and line 1 boundary)
- While fetching, the button shows a subtle loading indicator
- Fetched lines are inserted as `context` type `DiffLine`s with correct line numbers

### Styling

Small clickable button/icon in the gutter area. The rest of the row is a subtle separator line. Distinct from diff content but consistent with existing hunk header aesthetics.

## Error Handling & Edge Cases

### Errors

- **File deleted in target ref:** No expand buttons shown for deleted files
- **Binary files:** No expand buttons (already skipped in diff rendering)
- **Fetch failure:** Brief inline error on the expand row; retry on next click

### Edge cases

- **Expand up from first hunk:** Clamp `start` to line 1
- **Expand down from last hunk:** Clamp `end` to `totalLines`
- **Gap between hunks smaller than 20 lines:** Fetch only the actual gap; merge hunks
- **Gap of 0 lines:** No expand row shown (hunks already adjacent)
- **File with no trailing newline:** `\ No newline at end of file` marker already parsed; expansion adds context lines above it

## Testing

### Backend

- Unit tests for `GetFileLines`: working tree reads, ref-based reads, line range clamping, invalid refs, missing files
- HTTP handler test for `/api/git/file-lines`: query param validation, error responses (using `httptest.NewServer` with a temp git repo)

### Frontend

- Unit tests for the diff reducer: `EXPAND_UP`, `EXPAND_DOWN`, `EXPAND_BETWEEN`, hunk merging, line number correctness after insertion
- Component tests for expand buttons: correct placement, disappearance when no more lines, loading state during fetch
