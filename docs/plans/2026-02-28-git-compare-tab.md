# Git Compare Tab + History Tab Upgrade Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a Compare tab to the git panel that shows a full branch diff against a configurable upstream base (GitHub "Files changed" style), and upgrade the History tab to show all diffs for an expanded commit stacked in the right pane.

**Architecture:** Backend adds three new git functions and API endpoints for branch comparison, branch listing, and full commit diffs. Frontend adds a `parseMultiFileDiff` parser, new React Query hooks, a `CompareView` component with file tree + stacked diffs, and updates `CommitHistory` to render stacked diffs with scroll-to-file.

**Tech Stack:** Go stdlib `net/http`, React 18 + TypeScript, TanStack Query v5, existing `UnifiedDiff` renderer, `parseDiff` pipeline.

---

### Task 1: Backend — `GetBranches` function

**Files:**
- Create: `internal/agent/git/compare.go`
- Test: `internal/agent/git/compare_test.go`

**Step 1: Write the failing test**

Create `internal/agent/git/compare_test.go`:

```go
package git

import (
	"os/exec"
	"testing"
)

func TestGetBranches(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa", "initial commit")

	t.Run("returns branches", func(t *testing.T) {
		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Branches) == 0 {
			t.Error("expected at least one branch")
		}
	})

	t.Run("default base with no upstream", func(t *testing.T) {
		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		// No upstream configured, no main/master branch => empty default
		if result.DefaultBase != "" {
			t.Errorf("expected empty default base, got %q", result.DefaultBase)
		}
	})

	t.Run("default base falls back to main", func(t *testing.T) {
		// Create a "main" branch
		cmd := exec.Command("git", "branch", "main")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch main failed: %s: %s", err, out)
		}

		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.DefaultBase != "main" {
			t.Errorf("expected default base %q, got %q", "main", result.DefaultBase)
		}
	})

	t.Run("includes feature branches", func(t *testing.T) {
		cmd := exec.Command("git", "branch", "feature/test")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch failed: %s: %s", err, out)
		}

		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, b := range result.Branches {
			if b == "feature/test" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected feature/test in branches, got %v", result.Branches)
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-compare-mode && go test ./internal/agent/git/ -run TestGetBranches -v`
Expected: FAIL — `GetBranches` not defined

**Step 3: Write minimal implementation**

Create `internal/agent/git/compare.go`:

```go
package git

import (
	"context"
	"sort"
	"strings"
)

// GetBranches returns all local branches and the auto-detected default base branch.
func GetBranches(dir string) (*BranchList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	out, err := runGit(ctx, dir, defaultMaxBuffer,
		"branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	sort.Strings(branches)

	defaultBase := detectDefaultBase(ctx, dir, branches)

	return &BranchList{
		Branches:    branches,
		DefaultBase: defaultBase,
	}, nil
}

// detectDefaultBase tries upstream tracking branch, then falls back to main/master.
func detectDefaultBase(ctx context.Context, dir string, branches []string) string {
	// Try upstream tracking branch
	if upstream, err := runGit(ctx, dir, defaultMaxBuffer,
		"rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		upstream = strings.TrimSpace(upstream)
		if upstream != "" {
			return upstream
		}
	}

	// Fall back to main, then master
	branchSet := make(map[string]bool, len(branches))
	for _, b := range branches {
		branchSet[b] = true
	}
	if branchSet["main"] {
		return "main"
	}
	if branchSet["master"] {
		return "master"
	}
	return ""
}
```

Add types to `internal/agent/git/types.go`:

```go
// CompareResult holds the diff and file metadata for a branch comparison.
type CompareResult struct {
	Diff           string       `json:"diff"`
	Files          []CommitFile `json:"files"`
	TotalAdditions int          `json:"totalAdditions"`
	TotalDeletions int          `json:"totalDeletions"`
	BaseRef        string       `json:"baseRef"`
	HeadRef        string       `json:"headRef"`
}

// BranchList holds available branches and the auto-detected default base.
type BranchList struct {
	Branches    []string `json:"branches"`
	DefaultBase string   `json:"defaultBase"`
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-compare-mode && go test ./internal/agent/git/ -run TestGetBranches -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/git/compare.go internal/agent/git/compare_test.go internal/agent/git/types.go
git commit -m "feat(git): add GetBranches with upstream auto-detection"
```

---

### Task 2: Backend — `GetCompare` function

**Files:**
- Modify: `internal/agent/git/compare.go`
- Modify: `internal/agent/git/compare_test.go`

**Step 1: Write the failing test**

Append to `internal/agent/git/compare_test.go`:

```go
func TestGetCompare(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "base.txt", "base content", "base commit")

	// Create a feature branch and add changes
	for _, args := range [][]string{
		{"git", "checkout", "-b", "feature"},
		{"git", "checkout", "-b", "main", "HEAD~0"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.CombinedOutput()
	}
	// Switch back to set up main as a branch at current commit
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s: %s", args, err, out)
		}
	}
	run("git", "checkout", "feature")
	commitFile(t, dir, "new.txt", "new content", "add new file")
	commitFile(t, dir, "base.txt", "modified content", "modify base file")

	t.Run("returns diff and files", func(t *testing.T) {
		result, err := GetCompare(dir, "main")
		if err != nil {
			t.Fatal(err)
		}
		if result.Diff == "" {
			t.Error("expected non-empty diff")
		}
		if len(result.Files) != 2 {
			t.Fatalf("expected 2 files, got %d", len(result.Files))
		}
		if result.TotalAdditions == 0 {
			t.Error("expected additions > 0")
		}
		if result.BaseRef == "" || result.HeadRef == "" {
			t.Error("expected baseRef and headRef")
		}
	})

	t.Run("invalid base ref", func(t *testing.T) {
		_, err := GetCompare(dir, "nonexistent-branch")
		if err == nil {
			t.Error("expected error for invalid base ref")
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/git/ -run TestGetCompare -v`
Expected: FAIL — `GetCompare` not defined

**Step 3: Write minimal implementation**

Append to `internal/agent/git/compare.go`:

```go
// GetCompare returns the full diff and per-file metadata comparing base to HEAD.
func GetCompare(dir, base string) (*CompareResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	// Find the merge base
	mergeBase, err := runGit(ctx, dir, defaultMaxBuffer, "merge-base", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base: %w", err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	// Get HEAD ref
	headRef, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--short", "HEAD")
	if err != nil {
		return nil, err
	}
	headRef = strings.TrimSpace(headRef)

	// Full combined diff
	diff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U20", mergeBase+"..HEAD")
	if err != nil {
		return nil, err
	}

	// Per-file metadata: name-status
	statusMap := map[string]struct {
		status  string
		oldPath string
	}{}
	if nsOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--name-status", mergeBase+"..HEAD"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(nsOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			statusChar := fields[0]
			if len(fields) == 3 {
				statusMap[fields[2]] = struct {
					status  string
					oldPath string
				}{statusChar[:1], fields[1]}
			} else {
				statusMap[fields[1]] = struct {
					status  string
					oldPath string
				}{statusChar[:1], ""}
			}
		}
	}

	// Per-file metadata: numstat
	var files []CommitFile
	totalAdds, totalDels := 0, 0
	if numOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--numstat", mergeBase+"..HEAD"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(numOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) != 3 {
				continue
			}

			adds, dels := 0, 0
			isBinary := fields[0] == "-" && fields[1] == "-"
			if !isBinary {
				adds, _ = strconv.Atoi(fields[0])
				dels, _ = strconv.Atoi(fields[1])
			}

			path := fields[2]
			if idx := strings.Index(path, " => "); idx != -1 {
				path = path[idx+4:]
			}

			st := StatusModified
			var oldPath string
			if info, ok := statusMap[path]; ok {
				switch info.status {
				case "A":
					st = StatusAdded
				case "D":
					st = StatusDeleted
				case "R":
					st = StatusRenamed
					oldPath = info.oldPath
				case "C":
					st = StatusCopied
				}
			}

			files = append(files, CommitFile{
				Path:      path,
				Status:    st,
				Additions: adds,
				Deletions: dels,
				OldPath:   oldPath,
			})

			totalAdds += adds
			totalDels += dels
		}
	}

	if files == nil {
		files = []CommitFile{}
	}

	return &CompareResult{
		Diff:           diff,
		Files:          files,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
		BaseRef:        mergeBase[:7],
		HeadRef:        headRef,
	}, nil
}
```

Note: This requires adding `"fmt"` and `"strconv"` to the imports in `compare.go`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/git/ -run TestGetCompare -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/git/compare.go internal/agent/git/compare_test.go
git commit -m "feat(git): add GetCompare for branch diff with file metadata"
```

---

### Task 3: Backend — `GetCommitFullDiff` function

**Files:**
- Modify: `internal/agent/git/history.go`
- Modify: `internal/agent/git/history_test.go`

**Step 1: Write the failing test**

Append to `internal/agent/git/history_test.go`:

```go
func TestGetCommitFullDiff(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa\n", "add a")
	commitFile(t, dir, "b.txt", "bbb\n", "add b")

	// Make a commit that changes both files
	writeTestFile(dir, "a.txt", "aaa modified\n")
	writeTestFile(dir, "b.txt", "bbb modified\n")
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "modify both files")
	cmd.Dir = dir
	cmd.CombinedOutput()

	commits, _ := GetHistory(dir, 1)
	hash := commits[0].Hash

	t.Run("returns combined diff", func(t *testing.T) {
		diff, err := GetCommitFullDiff(dir, hash)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(diff, "a.txt") {
			t.Error("expected diff to contain a.txt")
		}
		if !strings.Contains(diff, "b.txt") {
			t.Error("expected diff to contain b.txt")
		}
		// Should contain multiple diff --git sections
		if strings.Count(diff, "diff --git") < 2 {
			t.Errorf("expected multiple diff sections, got %d", strings.Count(diff, "diff --git"))
		}
	})

	t.Run("invalid hash", func(t *testing.T) {
		_, err := GetCommitFullDiff(dir, "not-a-hash!")
		if err == nil {
			t.Error("expected error for invalid hash")
		}
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/git/ -run TestGetCommitFullDiff -v`
Expected: FAIL — `GetCommitFullDiff` not defined

**Step 3: Write minimal implementation**

Append to `internal/agent/git/history.go`:

```go
// GetCommitFullDiff returns the full combined diff for all files in a commit.
func GetCommitFullDiff(dir, hash string) (string, error) {
	if err := validateHash(hash); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	return runGit(ctx, dir, diffMaxBuffer, "show", "-U20", "-m", "--first-parent", hash)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/git/ -run TestGetCommitFullDiff -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/git/history.go internal/agent/git/history_test.go
git commit -m "feat(git): add GetCommitFullDiff for full commit diffs"
```

---

### Task 4: Backend — API handlers and routes

**Files:**
- Modify: `internal/agent/api/git.go`
- Modify: `internal/agent/api/router.go`

**Step 1: Add the three new handlers to `git.go`**

Append to `internal/agent/api/git.go` (before the closing of the file):

```go
// GET /api/git/compare?path=...&base=...
func (h *gitHandler) compare(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	base := r.URL.Query().Get("base")
	if path == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path and base parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := git.GetCompare(expandedPath, base)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/compare/branches?path=...
func (h *gitHandler) compareBranches(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := git.GetBranches(expandedPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/history/{hash}/full-diff?path=...
func (h *gitHandler) commitFullDiff(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	diff, err := git.GetCommitFullDiff(expandedPath, hash)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"diff": diff})
}
```

**Step 2: Register routes in `router.go`**

Add these three lines to the `// Git routes (read-only)` section of `NewRouter`, after the existing routes:

```go
mux.HandleFunc("GET /api/git/compare/branches", gh.compareBranches)
mux.HandleFunc("GET /api/git/compare", gh.compare)
mux.HandleFunc("GET /api/git/history/{hash}/full-diff", gh.commitFullDiff)
```

Important: `compare/branches` must be registered BEFORE `compare` so the more-specific path matches first.

**Step 3: Remove the old per-file commit diff route**

In `router.go`, remove the line:
```go
mux.HandleFunc("GET /api/git/history/{hash}/diff", gh.commitFileDiff)
```

In `git.go`, remove the `commitFileDiff` handler method entirely.

**Step 4: Verify build compiles**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-compare-mode && go build ./...`
Expected: Success

**Step 5: Run all Go tests**

Run: `go test ./internal/agent/...`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/agent/api/git.go internal/agent/api/router.go
git commit -m "feat(api): add compare, branches, and full-diff endpoints"
```

---

### Task 5: Frontend — `parseMultiFileDiff` function

**Files:**
- Modify: `web/src/lib/diff-parser.ts`

**Step 1: Add `parseMultiFileDiff` to `diff-parser.ts`**

Append to the end of `web/src/lib/diff-parser.ts`:

```typescript
/**
 * Splits a combined multi-file diff into individual ParsedDiff objects.
 * A combined diff contains multiple "diff --git a/... b/..." sections.
 */
export function parseMultiFileDiff(diffText: string): ParsedDiff[] {
  if (!diffText) return [];

  // Split on "diff --git " boundaries, keeping the delimiter
  const sections = diffText.split(/(?=^diff --git )/m);
  const results: ParsedDiff[] = [];

  for (const section of sections) {
    const trimmed = section.trim();
    if (!trimmed || !trimmed.startsWith("diff --git ")) continue;
    results.push(parseDiff(trimmed));
  }

  return results;
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-compare-mode/web && npx tsc --noEmit`
Expected: Success (or only pre-existing errors)

**Step 3: Commit**

```bash
git add web/src/lib/diff-parser.ts
git commit -m "feat(diff): add parseMultiFileDiff for combined diff splitting"
```

---

### Task 6: Frontend — TypeScript types, query hooks, and cache keys

**Files:**
- Modify: `web/src/types.ts`
- Modify: `web/src/data/git/keys.ts`
- Modify: `web/src/data/git/queries.ts`
- Modify: `web/src/data/git/index.ts`

**Step 1: Add types to `types.ts`**

Append after the `CommitDetail` interface:

```typescript
export interface CompareResult {
  diff: string;
  files: CommitFile[];
  totalAdditions: number;
  totalDeletions: number;
  baseRef: string;
  headRef: string;
}

export interface BranchList {
  branches: string[];
  defaultBase: string;
}
```

**Step 2: Add cache keys to `keys.ts`**

Add to the `gitKeys` object:

```typescript
compareBranches: (path: string) =>
  [...gitKeys.all, "compare-branches", path] as const,
compare: (path: string, base: string) =>
  [...gitKeys.all, "compare", path, base] as const,
commitFullDiff: (path: string, hash: string) =>
  [...gitKeys.all, "commit-full-diff", path, hash] as const,
```

Remove the `commitFileDiff` key.

**Step 3: Add query hooks to `queries.ts`**

Add the imports for the new types:

```typescript
import type {
  GitStatus,
  CommitSummary,
  CommitDetail,
  CompareResult,
  BranchList,
} from "@/types";
```

Append the new hooks:

```typescript
// --- Compare Branches ---

export function useCompareBranchesQuery(path: string) {
  return useQuery({
    queryKey: gitKeys.compareBranches(path),
    queryFn: async () => {
      const data = await apiFetch<BranchList>(
        `/agent/api/git/compare/branches?path=${encodeURIComponent(path)}`,
      );
      return data;
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0,
  });
}

// --- Compare ---

export function useCompareQuery(path: string, base: string | null) {
  return useQuery({
    queryKey: gitKeys.compare(path, base ?? ""),
    queryFn: async () => {
      const data = await apiFetch<CompareResult>(
        `/agent/api/git/compare?path=${encodeURIComponent(path)}&base=${encodeURIComponent(base!)}`,
      );
      return data;
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!base,
  });
}

// --- Commit Full Diff ---

export function useCommitFullDiffQuery(path: string, hash: string | null) {
  return useQuery({
    queryKey: gitKeys.commitFullDiff(path, hash ?? ""),
    queryFn: async () => {
      const data = await apiFetch<{ diff: string }>(
        `/agent/api/git/history/${hash}/full-diff?path=${encodeURIComponent(path)}`,
      );
      return data.diff ?? "";
    },
    staleTime: Infinity,
    enabled: path.trim().length > 0 && !!hash,
  });
}
```

Remove the `useCommitFileDiffQuery` hook.

**Step 4: Update exports in `index.ts`**

Replace the contents of `web/src/data/git/index.ts`:

```typescript
export { gitKeys } from "./keys";
export {
  useGitCheckQuery,
  useGitStatusQuery,
  useFileDiffQuery,
  useGitHistoryQuery,
  useCommitDetailQuery,
  useCompareBranchesQuery,
  useCompareQuery,
  useCommitFullDiffQuery,
} from "./queries";
```

Note: `useCommitFileDiffQuery` is removed from exports.

**Step 5: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: May have errors in `CommitHistory.tsx` that still imports `useCommitFileDiffQuery` — that's expected and will be fixed in Task 8.

**Step 6: Commit**

```bash
git add web/src/types.ts web/src/data/git/keys.ts web/src/data/git/queries.ts web/src/data/git/index.ts
git commit -m "feat(data): add compare and full-diff query hooks"
```

---

### Task 7: Frontend — Compare tab integration (tab + shell component)

**Files:**
- Modify: `web/src/components/GitPanel/GitPanelTabs.tsx`
- Modify: `web/src/components/GitPanel/index.tsx`
- Create: `web/src/components/GitPanel/CompareView.tsx`

**Step 1: Extend `GitPanelTabs`**

In `GitPanelTabs.tsx`, change the type and add a third button:

```typescript
export type GitTab = "changes" | "history" | "compare";
```

Add a third button after the "History" button, following the same pattern:

```tsx
<button
  onClick={() => onTabChange("compare")}
  className={cn(
    "flex-1 px-3 py-1.5 text-sm font-medium transition-colors",
    activeTab === "compare"
      ? "text-foreground border-primary border-b-2"
      : "text-muted-foreground hover:text-foreground",
  )}
>
  Compare
</button>
```

**Step 2: Create `CompareView.tsx`**

Create `web/src/components/GitPanel/CompareView.tsx`:

```tsx
import { useState, useRef, useCallback, useMemo, useEffect } from "react";
import {
  Loader2,
  GitCompareArrows,
  ChevronDown,
  Plus,
  Minus,
  FileText,
  FilePlus,
  FileX,
  ArrowRight,
  ArrowLeft,
  List,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { UnifiedDiff } from "@/components/DiffViewer/UnifiedDiff";
import { parseMultiFileDiff, getDiffFileName } from "@/lib/diff-parser";
import { useCompareBranchesQuery, useCompareQuery } from "@/data/git";
import { useViewport } from "@/hooks/useViewport";
import type { CommitFile, FileStatus } from "@/types";

interface CompareViewProps {
  workingDirectory: string;
}

export function CompareView({ workingDirectory }: CompareViewProps) {
  const { isMobile } = useViewport();
  const [baseBranch, setBaseBranch] = useState<string | null>(null);
  const [showMobileFileList, setShowMobileFileList] = useState(false);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const [selectedPath, setSelectedPath] = useState<string | null>(null);

  const {
    data: branchData,
    isLoading: loadingBranches,
  } = useCompareBranchesQuery(workingDirectory);

  // Set default base branch when branch data loads
  useEffect(() => {
    if (branchData?.defaultBase && baseBranch === null) {
      setBaseBranch(branchData.defaultBase);
    }
  }, [branchData, baseBranch]);

  const {
    data: compareData,
    isLoading: loadingCompare,
  } = useCompareQuery(workingDirectory, baseBranch);

  const parsedDiffs = useMemo(() => {
    if (!compareData?.diff) return [];
    return parseMultiFileDiff(compareData.diff);
  }, [compareData?.diff]);

  const setDiffRef = useCallback(
    (path: string) => (el: HTMLDivElement | null) => {
      if (el) {
        diffRefs.current.set(path, el);
      } else {
        diffRefs.current.delete(path);
      }
    },
    [],
  );

  const scrollToFile = useCallback((path: string) => {
    setSelectedPath(path);
    const el = diffRefs.current.get(path);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "start" });
    }
    setShowMobileFileList(false);
  }, []);

  if (loadingBranches) {
    return (
      <div className="flex flex-1 items-center justify-center">
        <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
      </div>
    );
  }

  if (!branchData?.defaultBase && !baseBranch) {
    return (
      <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
        <GitCompareArrows className="mb-2 h-8 w-8 opacity-50" />
        <p className="text-sm">No base branch detected</p>
        <p className="text-xs">
          Set an upstream tracking branch or create a main/master branch
        </p>
      </div>
    );
  }

  const branchSelector = (
    <div className="flex items-center gap-2 px-3 py-2">
      <span className="text-muted-foreground text-xs">Base:</span>
      <select
        value={baseBranch ?? ""}
        onChange={(e) => setBaseBranch(e.target.value)}
        className="bg-muted border-border rounded border px-2 py-1 text-xs"
      >
        {branchData?.branches.map((branch) => (
          <option key={branch} value={branch}>
            {branch}
          </option>
        ))}
      </select>
    </div>
  );

  const summary = compareData ? (
    <div className="text-muted-foreground border-border/50 border-b px-3 py-1.5 text-xs">
      {compareData.files.length} file{compareData.files.length !== 1 ? "s" : ""} changed
      {compareData.totalAdditions > 0 && (
        <span className="ml-2 text-green-500">+{compareData.totalAdditions}</span>
      )}
      {compareData.totalDeletions > 0 && (
        <span className="ml-1 text-red-500">-{compareData.totalDeletions}</span>
      )}
    </div>
  ) : null;

  const fileList = compareData?.files.length ? (
    <div className="flex-1 overflow-y-auto">
      {compareData.files.map((file) => (
        <CompareFileRow
          key={file.path}
          file={file}
          isSelected={selectedPath === file.path}
          onClick={() => scrollToFile(file.path)}
        />
      ))}
    </div>
  ) : null;

  const diffPane = (
    <div ref={scrollContainerRef} className="flex-1 overflow-y-auto p-3">
      {loadingCompare ? (
        <div className="flex h-32 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      ) : parsedDiffs.length === 0 ? (
        <div className="text-muted-foreground flex flex-col items-center justify-center py-12">
          <GitCompareArrows className="mb-4 h-12 w-12 opacity-50" />
          <p className="text-sm">No changes between branches</p>
        </div>
      ) : (
        <div className="space-y-3">
          {parsedDiffs.map((diff) => {
            const fileName = getDiffFileName(diff);
            return (
              <div key={fileName} ref={setDiffRef(fileName)}>
                <UnifiedDiff diff={diff} fileName={fileName} expanded />
              </div>
            );
          })}
        </div>
      )}
    </div>
  );

  // Mobile layout
  if (isMobile) {
    return (
      <div className="flex h-full flex-col">
        {branchSelector}
        {summary}
        {diffPane}
        {/* Floating file navigator button */}
        {compareData && compareData.files.length > 0 && (
          <>
            <button
              onClick={() => setShowMobileFileList(true)}
              className="bg-primary text-primary-foreground absolute bottom-4 right-4 rounded-full p-3 shadow-lg"
            >
              <List className="h-5 w-5" />
            </button>
            {showMobileFileList && (
              <div className="bg-background/95 absolute inset-0 z-50 flex flex-col backdrop-blur-sm">
                <div className="flex items-center gap-2 p-3">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    onClick={() => setShowMobileFileList(false)}
                  >
                    <ArrowLeft className="h-5 w-5" />
                  </Button>
                  <span className="text-sm font-medium">Files</span>
                </div>
                <div className="flex-1 overflow-y-auto">{fileList}</div>
              </div>
            )}
          </>
        )}
      </div>
    );
  }

  // Desktop layout
  return (
    <div className="flex h-full min-h-0">
      {/* Left sidebar */}
      <div className="flex w-[260px] flex-shrink-0 flex-col">
        {branchSelector}
        {summary}
        {fileList}
      </div>

      {/* Divider */}
      <div className="bg-muted/50 w-1 flex-shrink-0" />

      {/* Right pane */}
      <div className="bg-muted/20 flex min-w-0 flex-1 flex-col">
        {diffPane}
      </div>
    </div>
  );
}

function CompareFileRow({
  file,
  isSelected,
  onClick,
}: {
  file: CommitFile;
  isSelected: boolean;
  onClick: () => void;
}) {
  const StatusIcon = getStatusIcon(file.status);

  return (
    <button
      onClick={onClick}
      className={cn(
        "hover:bg-muted/70 flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors",
        isSelected && "bg-primary/10 hover:bg-primary/20",
      )}
    >
      <StatusIcon
        className={cn("h-4 w-4 flex-shrink-0", getStatusColor(file.status))}
      />
      <span className="flex-1 truncate text-xs">
        {file.oldPath ? (
          <span className="flex items-center gap-1">
            <span className="text-muted-foreground">{file.oldPath}</span>
            <ArrowRight className="h-3 w-3" />
            <span>{file.path}</span>
          </span>
        ) : (
          file.path
        )}
      </span>
      <div className="flex flex-shrink-0 items-center gap-1 text-xs">
        {file.additions > 0 && (
          <span className="text-green-500">+{file.additions}</span>
        )}
        {file.deletions > 0 && (
          <span className="text-red-500">-{file.deletions}</span>
        )}
      </div>
    </button>
  );
}

function getStatusIcon(status: FileStatus) {
  switch (status) {
    case "added":
      return FilePlus;
    case "deleted":
      return FileX;
    case "renamed":
      return ArrowRight;
    default:
      return FileText;
  }
}

function getStatusColor(status: FileStatus): string {
  switch (status) {
    case "added":
      return "text-green-500";
    case "deleted":
      return "text-red-500";
    case "renamed":
      return "text-yellow-500";
    default:
      return "text-muted-foreground";
  }
}
```

**Step 3: Wire up Compare tab in `GitPanel/index.tsx`**

Add the import at the top:

```typescript
import { CompareView } from "./CompareView";
```

In the mobile layout section, add a branch for compare BEFORE the history check (after line 162 `if (isMobile) {`):

```tsx
if (activeTab === "compare") {
  return (
    <div className="bg-background relative flex h-full w-full flex-col">
      <Header
        branch={status.branch}
        ahead={status.ahead}
        behind={status.behind}
        onRefresh={handleRefresh}
        refreshing={isRefetching}
      />
      <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />
      <CompareView workingDirectory={workingDirectory} />
    </div>
  );
}
```

In the desktop layout section, add a branch for compare BEFORE the history check (after line 259 `// --- Desktop layout ---`):

```tsx
if (activeTab === "compare") {
  return (
    <div className="bg-background flex h-full w-full flex-col">
      <Header
        branch={status.branch}
        ahead={status.ahead}
        behind={status.behind}
        onRefresh={handleRefresh}
        refreshing={isRefetching}
      />
      <GitPanelTabs activeTab={activeTab} onTabChange={setActiveTab} />
      <CompareView workingDirectory={workingDirectory} />
    </div>
  );
}
```

**Step 4: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: May still have errors in `CommitHistory.tsx` due to removed `useCommitFileDiffQuery` — fixed in next task.

**Step 5: Commit**

```bash
git add web/src/components/GitPanel/GitPanelTabs.tsx web/src/components/GitPanel/CompareView.tsx web/src/components/GitPanel/index.tsx
git commit -m "feat(ui): add Compare tab with file tree and stacked diffs"
```

---

### Task 8: Frontend — History tab upgrade (stacked diffs + scroll-to-file)

**Files:**
- Modify: `web/src/components/GitPanel/CommitHistory.tsx`
- Modify: `web/src/components/GitPanel/CommitItem.tsx`

**Step 1: Rewrite `CommitHistory.tsx`**

Replace the contents of `CommitHistory.tsx`:

```tsx
import { useState, useCallback, useRef, useMemo } from "react";
import { Loader2, History, ArrowLeft, FileCode } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CommitItem } from "./CommitItem";
import { UnifiedDiff } from "@/components/DiffViewer/UnifiedDiff";
import {
  useGitHistoryQuery,
  useCommitFullDiffQuery,
} from "@/data/git";
import { parseMultiFileDiff, getDiffFileName } from "@/lib/diff-parser";
import { useViewport } from "@/hooks/useViewport";
import type { CommitFile } from "@/types";

interface CommitHistoryProps {
  workingDirectory: string;
  header?: React.ReactNode;
}

export function CommitHistory({ workingDirectory, header }: CommitHistoryProps) {
  const { isMobile } = useViewport();
  const {
    data: commits,
    isLoading,
    error,
  } = useGitHistoryQuery(workingDirectory);

  const [expandedHash, setExpandedHash] = useState<string | null>(null);
  const [selectedFilePath, setSelectedFilePath] = useState<string | null>(null);
  const diffRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  // Track if user tapped a file on mobile to navigate to full-screen diff view
  const [mobileShowDiffs, setMobileShowDiffs] = useState(false);

  const { data: fullDiff, isLoading: loadingDiff } = useCommitFullDiffQuery(
    workingDirectory,
    expandedHash,
  );

  const parsedDiffs = useMemo(() => {
    if (!fullDiff) return [];
    return parseMultiFileDiff(fullDiff);
  }, [fullDiff]);

  const setDiffRef = useCallback(
    (path: string) => (el: HTMLDivElement | null) => {
      if (el) {
        diffRefs.current.set(path, el);
      } else {
        diffRefs.current.delete(path);
      }
    },
    [],
  );

  const handleToggleCommit = useCallback((hash: string) => {
    setExpandedHash((prev) => {
      if (prev === hash) {
        return null;
      }
      return hash;
    });
    setSelectedFilePath(null);
    setMobileShowDiffs(false);
  }, []);

  const handleFileClick = useCallback(
    (hash: string, file: CommitFile) => {
      setExpandedHash(hash);
      setSelectedFilePath(file.path);

      if (isMobile) {
        setMobileShowDiffs(true);
        // Scroll after render
        requestAnimationFrame(() => {
          const el = diffRefs.current.get(file.path);
          if (el) {
            el.scrollIntoView({ behavior: "smooth", block: "start" });
          }
        });
      } else {
        // Desktop: scroll in the right pane
        requestAnimationFrame(() => {
          const el = diffRefs.current.get(file.path);
          if (el) {
            el.scrollIntoView({ behavior: "smooth", block: "start" });
          }
        });
      }
    },
    [isMobile],
  );

  const stackedDiffs = (
    <div className="space-y-3 p-3">
      {parsedDiffs.map((diff) => {
        const fileName = getDiffFileName(diff);
        return (
          <div key={fileName} ref={setDiffRef(fileName)}>
            <UnifiedDiff diff={diff} fileName={fileName} expanded />
          </div>
        );
      })}
    </div>
  );

  if (isLoading) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
          <History className="mb-2 h-8 w-8 opacity-50" />
          <p className="text-center text-sm">Failed to load commit history</p>
        </div>
      </div>
    );
  }

  if (!commits?.length) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center p-4">
          <History className="mb-2 h-8 w-8 opacity-50" />
          <p className="text-sm">No commits yet</p>
        </div>
      </div>
    );
  }

  // Mobile: full-screen stacked diff view when user taps a file
  if (isMobile && mobileShowDiffs && expandedHash) {
    const commit = commits.find((c) => c.hash === expandedHash);
    return (
      <div className="flex h-full flex-col">
        <div className="bg-muted/30 flex items-center gap-2 p-2">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => setMobileShowDiffs(false)}
          >
            <ArrowLeft className="h-5 w-5" />
          </Button>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">
              {commit?.subject ?? expandedHash.slice(0, 7)}
            </p>
            <p className="text-muted-foreground text-xs">
              {expandedHash.slice(0, 7)}
            </p>
          </div>
        </div>
        <div className="flex-1 overflow-auto">
          {loadingDiff ? (
            <div className="flex h-32 items-center justify-center">
              <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
            </div>
          ) : (
            stackedDiffs
          )}
        </div>
      </div>
    );
  }

  // Mobile: commit list only
  if (isMobile) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        {header}
        <div className="flex-1 overflow-y-auto">
          {commits.map((commit) => (
            <CommitItem
              key={commit.hash}
              commit={commit}
              workingDir={workingDirectory}
              expanded={expandedHash === commit.hash}
              onToggle={() => handleToggleCommit(commit.hash)}
              onFileClick={handleFileClick}
              selectedFile={
                selectedFilePath && expandedHash
                  ? { hash: expandedHash, path: selectedFilePath }
                  : null
              }
            />
          ))}
        </div>
      </div>
    );
  }

  // Desktop: side-by-side layout
  return (
    <div className="flex min-h-0 flex-1">
      {/* Commit list */}
      <div className="flex w-[300px] flex-shrink-0 flex-col">
        {header}
        <div className="flex-1 overflow-y-auto">
          {commits.map((commit) => (
            <CommitItem
              key={commit.hash}
              commit={commit}
              workingDir={workingDirectory}
              expanded={expandedHash === commit.hash}
              onToggle={() => handleToggleCommit(commit.hash)}
              onFileClick={handleFileClick}
              selectedFile={
                selectedFilePath && expandedHash
                  ? { hash: expandedHash, path: selectedFilePath }
                  : null
              }
            />
          ))}
        </div>
      </div>

      {/* Divider */}
      <div className="bg-muted/50 w-1 flex-shrink-0" />

      {/* Diff view - stacked diffs */}
      <div className="bg-muted/20 flex min-w-0 flex-1 flex-col">
        {loadingDiff ? (
          <div className="flex flex-1 items-center justify-center">
            <Loader2 className="text-muted-foreground h-6 w-6 animate-spin" />
          </div>
        ) : expandedHash && parsedDiffs.length > 0 ? (
          <div className="flex-1 overflow-auto">{stackedDiffs}</div>
        ) : (
          <div className="text-muted-foreground flex flex-1 flex-col items-center justify-center">
            <FileCode className="mb-4 h-12 w-12 opacity-50" />
            <p className="text-sm">Select a commit to view changes</p>
          </div>
        )}
      </div>
    </div>
  );
}
```

**Step 2: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`
Expected: Success

**Step 3: Commit**

```bash
git add web/src/components/GitPanel/CommitHistory.tsx
git commit -m "feat(ui): upgrade History tab with stacked diffs and scroll-to-file"
```

---

### Task 9: Backend — Remove `GetCommitFileDiff` and its test

**Files:**
- Modify: `internal/agent/git/history.go`
- Modify: `internal/agent/git/history_test.go`

**Step 1: Remove `GetCommitFileDiff` from `history.go`**

Delete the `GetCommitFileDiff` function (lines 249-260 in the current file).

**Step 2: Remove `TestGetCommitFileDiff` from `history_test.go`**

Delete the `TestGetCommitFileDiff` test function (lines 110-124 in the current file).

**Step 3: Run all Go tests**

Run: `go test ./internal/agent/... -v`
Expected: PASS (all remaining tests still pass)

**Step 4: Commit**

```bash
git add internal/agent/git/history.go internal/agent/git/history_test.go
git commit -m "refactor: remove unused GetCommitFileDiff"
```

---

### Task 10: Full build verification

**Step 1: Run all Go tests**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-compare-mode && go test ./...`
Expected: PASS

**Step 2: Build the frontend**

Run: `cd web && npm run build`
Expected: Success — builds to `internal/web/dist/`

**Step 3: Build the full binary**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-compare-mode && make build`
Expected: Success — produces `bin/argus`

**Step 4: Commit if any fixups needed, otherwise done**
