# Git Compare Tab + History Tab Upgrade

## Problem

The git panel's History tab requires clicking through individual files one at a time to view diffs within a commit. There is no way to compare the current branch against an upstream branch (e.g. `main`, or a parent branch when stacking) to see all changes at once.

## Decision

1. Add a **Compare tab** to the git panel that shows a full branch diff against a configurable upstream base, with a file tree on the left and all diffs stacked in one scrollable pane on the right (GitHub "Files changed" style).
2. Upgrade the **History tab** to show all diffs for an expanded commit stacked in the right pane (instead of one file at a time), with click-to-scroll from the file list.

Both features share a new `parseMultiFileDiff` function and reuse the existing `UnifiedDiff` renderer.

## Backend

### New Go types (`internal/agent/git/types.go`)

```go
type CompareResult struct {
    Diff           string       `json:"diff"`
    Files          []CommitFile `json:"files"`
    TotalAdditions int          `json:"totalAdditions"`
    TotalDeletions int          `json:"totalDeletions"`
    BaseRef        string       `json:"baseRef"`
    HeadRef        string       `json:"headRef"`
}

type BranchList struct {
    Branches    []string `json:"branches"`
    DefaultBase string   `json:"defaultBase"`
}
```

### New functions (`internal/agent/git/compare.go`)

**`GetCompare(dir, base string) (*CompareResult, error)`**

1. `git merge-base <base> HEAD` — find common ancestor
2. `git diff -U20 <merge-base>..HEAD` — full combined unified diff
3. `git diff --name-status <merge-base>..HEAD` + `git diff --numstat <merge-base>..HEAD` — per-file metadata (status, additions, deletions), same parsing pattern as `GetCommitDetail`

Returns the combined diff string, per-file metadata, and total stats.

**`GetBranches(dir string) (*BranchList, error)`**

1. `git branch -a --format='%(refname:short)'` — list all branches
2. `git rev-parse --abbrev-ref @{upstream}` — detect upstream tracking branch
3. Falls back to `main`, then `master` if no upstream is configured

Returns the branch list and the auto-detected default base.

### New function (`internal/agent/git/history.go`)

**`GetCommitFullDiff(dir, hash string) (string, error)`**

Runs `git show -U20 -m --first-parent <hash>` (no `-- file` filter) to get the full combined diff for all files in a commit.

### New API endpoints (`internal/agent/api/git.go` + `router.go`)

| Route | Handler | Function |
|-------|---------|----------|
| `GET /api/git/compare?path=...&base=...` | `gh.compare` | `git.GetCompare()` |
| `GET /api/git/compare/branches?path=...` | `gh.compareBranches` | `git.GetBranches()` |
| `GET /api/git/history/{hash}/full-diff?path=...` | `gh.commitFullDiff` | `git.GetCommitFullDiff()` |

All handlers follow the existing pattern: validate `path` via `SafeExpandPath`, validate `hash` via `validateHash` where applicable.

### Removed endpoint

`GET /api/git/history/{hash}/diff` (per-file commit diff) becomes unused and will be removed along with `GetCommitFileDiff`.

## Frontend

### Diff parser extension (`web/src/lib/diff-parser.ts`)

New exported function:

```typescript
export function parseMultiFileDiff(diffText: string): ParsedDiff[]
```

Splits a combined diff on `diff --git ` boundaries, then calls the existing `parseDiff` on each segment. Returns one `ParsedDiff` per file. Used by both Compare tab and History tab.

### New TypeScript types (`web/src/types.ts`)

```typescript
interface CompareResult {
  diff: string;
  files: CommitFile[];
  totalAdditions: number;
  totalDeletions: number;
  baseRef: string;
  headRef: string;
}

interface BranchList {
  branches: string[];
  defaultBase: string;
}
```

### New query hooks (`web/src/data/git/queries.ts` + `keys.ts`)

| Hook | Endpoint | staleTime | Notes |
|------|----------|-----------|-------|
| `useCompareBranchesQuery(path)` | `/api/git/compare/branches` | 30s | Fires on Compare tab mount |
| `useCompareQuery(path, base)` | `/api/git/compare` | 30s | Re-fetches when base branch changes |
| `useCommitFullDiffQuery(path, hash)` | `/api/git/history/{hash}/full-diff` | Infinity | Commits don't change |

**Removed hook:** `useCommitFileDiffQuery` — no longer needed.

### Tab integration (`web/src/components/GitPanel/`)

**`GitPanelTabs.tsx`:** Extend `GitTab` union to `"changes" | "history" | "compare"`. Add a third tab button labeled "Compare".

**`GitPanel/index.tsx`:** Add a branch for `activeTab === "compare"` in both mobile and desktop layouts that renders `<CompareView workingDirectory={workingDirectory} />`.

### Compare tab component (`web/src/components/GitPanel/CompareView.tsx`)

**Desktop layout:** Left sidebar + divider + right pane (all diffs stacked, scrollable).

**Left sidebar (top to bottom):**
1. **Branch selector** — dropdown showing the current base branch (auto-detected via `useCompareBranchesQuery`). Selecting a different branch updates state and re-fetches the compare diff.
2. **Summary line** — compact text: "12 files changed, +42 −18"
3. **File list** — flat list of changed files sorted by path. Each entry shows:
   - Status icon (color-coded, same palette as existing `FileChanges`)
   - File path (truncated)
   - `+N −N` inline stats
   - Clicking scrolls the right pane to that file's diff via `scrollIntoView({ behavior: 'smooth', block: 'start' })`

**Right pane:**
- All `ParsedDiff` objects from `parseMultiFileDiff(data.diff)` rendered as stacked `<UnifiedDiff>` components
- Each has a ref keyed by file path for scroll targeting
- All files initially expanded (collapsible via existing `UnifiedDiff` toggle)

**Mobile layout:** Branch selector + summary at top, stacked diffs below (no file tree sidebar). A floating button opens a bottom sheet with the file list for jump-to-file navigation.

**Data flow:**
1. `useCompareBranchesQuery(workingDirectory)` → populates dropdown, sets `baseBranch` state
2. `useCompareQuery(workingDirectory, baseBranch)` → returns `CompareResult`
3. `parseMultiFileDiff(result.diff)` → `ParsedDiff[]`
4. Left pane uses `result.files` for the file list with metadata
5. Right pane renders `ParsedDiff[]` as stacked `UnifiedDiff` components

### History tab upgrade (`web/src/components/GitPanel/CommitHistory.tsx` + `CommitItem.tsx`)

**Layout preserved:** Left pane (300px, commit list with expandable items) + divider + right pane.

**Changes to the right pane:**
- When a commit is expanded, fetch full diff via `useCommitFullDiffQuery(workingDirectory, expandedHash)`
- Parse with `parseMultiFileDiff(diff)` → `ParsedDiff[]`
- Render all diffs stacked in the right pane (instead of a single file's diff)
- Each `UnifiedDiff` has a ref keyed by file path

**Changes to the left pane (commit file list):**
- `useCommitDetailQuery` stays — needed for per-file metadata in the expanded file list
- Clicking a file in the expanded commit's file list scrolls the right pane to that file's diff via `scrollIntoView`
- The selected file is visually highlighted in the file list

**Mobile:** When a commit is expanded and a file is tapped, navigate to a full-screen stacked-diffs view, auto-scrolled to the tapped file.

## Files changed

### Backend — new files
- `internal/agent/git/compare.go` — `GetCompare`, `GetBranches`

### Backend — modified files
- `internal/agent/git/types.go` — add `CompareResult`, `BranchList`
- `internal/agent/git/history.go` — add `GetCommitFullDiff`, remove `GetCommitFileDiff`
- `internal/agent/api/git.go` — add `compare`, `compareBranches`, `commitFullDiff` handlers; remove `commitFileDiff`
- `internal/agent/api/router.go` — register new routes, remove old route

### Frontend — new files
- `web/src/components/GitPanel/CompareView.tsx`

### Frontend — modified files
- `web/src/lib/diff-parser.ts` — add `parseMultiFileDiff`
- `web/src/types.ts` — add `CompareResult`, `BranchList`
- `web/src/data/git/keys.ts` — add new cache keys
- `web/src/data/git/queries.ts` — add new hooks, remove `useCommitFileDiffQuery`
- `web/src/data/git/index.ts` — export new hooks
- `web/src/components/GitPanel/GitPanelTabs.tsx` — extend `GitTab`, add Compare button
- `web/src/components/GitPanel/index.tsx` — add Compare tab branch
- `web/src/components/GitPanel/CommitHistory.tsx` — stacked diffs in right pane, scroll-to-file
- `web/src/components/GitPanel/CommitItem.tsx` — update file click behavior
