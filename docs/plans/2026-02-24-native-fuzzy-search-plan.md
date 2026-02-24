# Native Go Fuzzy File Search — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the external `fd` dependency with a pure Go implementation using `fastwalk` + `sahilm/fuzzy` + `go-gitignore`, supporting full gitignore semantics including `!` negation. Add search anchoring by session working directory.

**Architecture:** Walk filesystem with `fastwalk`, filter via `go-gitignore` ignore patterns, fuzzy-match basenames with `sahilm/fuzzy`, return top-N results. Frontend passes session `working_directory` as search anchor.

**Tech Stack:** Go 1.25, TypeScript/React, TanStack Query

---

### Task 1: Add new Go dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add the three new dependencies**

Run:
```bash
cd /Users/jeevb/Workspace/repos/bxnlabs/argus
go get github.com/charlievieth/fastwalk
go get github.com/sahilm/fuzzy
go get github.com/sabhiram/go-gitignore
```

**Step 2: Verify dependencies resolve**

Run: `go mod tidy`
Expected: Clean exit, no errors.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add fastwalk, sahilm/fuzzy, go-gitignore"
```

---

### Task 2: Implement the walk + filter + match engine

This is the core rewrite. Replace `fd` invocation with native Go.

**Files:**
- Delete: `internal/agent/filesearch/exec.go`
- Delete: `internal/agent/filesearch/exec_test.go`
- Rewrite: `internal/agent/filesearch/operations.go`
- Rewrite: `internal/agent/filesearch/operations_test.go`
- Keep unchanged: `internal/agent/filesearch/types.go`

**Step 1: Write failing tests for the new walker**

Replace the contents of `operations_test.go` with tests for the new implementation. Tests should use real temp directory trees (created with `os.MkdirAll` and `os.WriteFile`) rather than mocking.

Test cases needed:

```go
package filesearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper: create a temp directory tree for testing.
// tree is a map of relative paths -> "" (directory, must end in "/") or content (file).
func createTree(t *testing.T, tree map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range tree {
		abs := filepath.Join(root, relPath)
		if strings.HasSuffix(relPath, "/") {
			if err := os.MkdirAll(abs, 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestSearch_BasicFuzzyMatch(t *testing.T) {
	root := createTree(t, map[string]string{
		"foo.txt":        "hello",
		"bar.txt":        "world",
		"src/foobar.go":  "package main",
		"src/baz.go":     "package main",
	})

	resp, err := searchDir(root, "foo", "", 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected at least one result for 'foo'")
	}
	// "foo.txt" should rank higher than "foobar.go" (exact basename prefix)
	if resp.Results[0].Name != "foo.txt" {
		t.Errorf("first result = %q, want foo.txt", resp.Results[0].Name)
	}
}

func TestSearch_TypeFilter(t *testing.T) {
	root := createTree(t, map[string]string{
		"mydir/":     "",
		"myfile.txt": "content",
	})

	// Search for directories only
	resp, err := searchDir(root, "my", "directory", 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if r.Type != "directory" {
			t.Errorf("got type %q for %q, want directory", r.Type, r.Name)
		}
	}

	// Search for files only
	resp, err = searchDir(root, "my", "file", 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if r.Type != "file" {
			t.Errorf("got type %q for %q, want file", r.Type, r.Name)
		}
	}
}

func TestSearch_IgnorePatterns(t *testing.T) {
	root := createTree(t, map[string]string{
		"src/main.go":              "package main",
		"node_modules/pkg/lib.js":  "module.exports = {}",
		".git/HEAD":                "ref: refs/heads/main",
	})

	// Write ignore file that excludes node_modules/ and .git/
	ignoreFile := filepath.Join(t.TempDir(), "ignore")
	os.WriteFile(ignoreFile, []byte("node_modules/\n.git/\n"), 0o644)

	matcher := loadIgnoreMatcher(ignoreFile)
	resp, err := searchDir(root, "main", "", 20, matcher)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range resp.Results {
		if strings.Contains(r.Path, "node_modules") {
			t.Errorf("result %q should have been ignored", r.Path)
		}
		if strings.Contains(r.Path, ".git") {
			t.Errorf("result %q should have been ignored", r.Path)
		}
	}
}

func TestSearch_NegationPatterns(t *testing.T) {
	root := createTree(t, map[string]string{
		"allowed/app.txt":    "data",
		"blocked/secret.txt": "data",
		"other/file.txt":     "data",
	})

	// Ignore everything, then un-ignore "allowed/"
	ignoreFile := filepath.Join(t.TempDir(), "ignore")
	os.WriteFile(ignoreFile, []byte("*\n!allowed/\n"), 0o644)

	matcher := loadIgnoreMatcher(ignoreFile)
	resp, err := searchDir(root, "txt", "", 20, matcher)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range resp.Results {
		if !strings.Contains(r.Path, "allowed") {
			t.Errorf("result %q should have been ignored (only 'allowed' dir is allowed)", r.Path)
		}
	}
	if resp.Count == 0 {
		t.Error("expected at least one result from the 'allowed' directory")
	}
}

func TestSearch_MaxDepth(t *testing.T) {
	// Create a deeply nested structure beyond maxDepth
	root := createTree(t, map[string]string{
		"a/b/c/d/e/f/g/h/i/deep.txt": "deep",
		"shallow.txt":                  "shallow",
	})

	resp, err := searchDir(root, "txt", "", 20, nil)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range resp.Results {
		if strings.Contains(r.Name, "deep") {
			t.Error("should not find files beyond maxDepth")
		}
	}
}

func TestSearch_Limit(t *testing.T) {
	tree := make(map[string]string)
	for i := 0; i < 30; i++ {
		tree[filepath.Join("dir", fmt.Sprintf("file%02d.txt", i))] = "content"
	}
	root := createTree(t, tree)

	resp, err := searchDir(root, "file", "", 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count > 5 {
		t.Errorf("count = %d, want <= 5", resp.Count)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	_, err := searchDir(t.TempDir(), "", "", 20, nil)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestSearch_AbsolutePaths(t *testing.T) {
	root := createTree(t, map[string]string{
		"test.txt": "content",
	})

	resp, err := searchDir(root, "test", "", 20, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if !filepath.IsAbs(r.Path) {
			t.Errorf("path %q is not absolute", r.Path)
		}
	}
}

func TestEnsureIgnoreFile(t *testing.T) {
	t.Run("creates default file when missing", func(t *testing.T) {
		home := t.TempDir()
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".argus", "ignore")
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, want := range []string{".git/", "node_modules/", "__pycache__/", "target/", ".venv/"} {
			if !strings.Contains(content, want) {
				t.Errorf("default file should contain %q", want)
			}
		}
	})

	t.Run("preserves existing file", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".argus")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		custom := "custom-pattern/\n"
		if err := os.WriteFile(filepath.Join(dir, "ignore"), []byte(custom), 0o600); err != nil {
			t.Fatal(err)
		}
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != custom {
			t.Errorf("existing file was overwritten: got %q, want %q", string(data), custom)
		}
		_ = path
	})
}
```

Note: The test functions reference `searchDir()` (a new internal function that accepts `searchDir` path, query, type, limit, and an optional ignore matcher) and `loadIgnoreMatcher()`. These are the implementation targets.

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/filesearch/ -v`
Expected: FAIL — `searchDir` and `loadIgnoreMatcher` are undefined.

**Step 3: Implement the new operations.go**

Replace the contents of `operations.go`. Keep `ensureIgnoreFile()` and `defaultIgnoreContents` as-is. Replace everything else:

```go
package filesearch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/sahilm/fuzzy"
	ignore "github.com/sabhiram/go-gitignore"
)

const (
	defaultLimit  = 20
	maxLimit      = 100
	maxDepth      = 8
	maxCollect    = 100_000 // cap entries to prevent OOM
	searchTimeout = 5 * time.Second
)

const ignoreFileName = "ignore"

// [... keep existing defaultIgnoreContents and ensureIgnoreFile unchanged ...]

// loadIgnoreMatcher compiles a gitignore-syntax file into a matcher.
// Returns nil if the file cannot be read (search proceeds without filtering).
func loadIgnoreMatcher(path string) *ignore.GitIgnore {
	if path == "" {
		return nil
	}
	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return gi
}

// Search searches for files/directories using fuzzy matching against basenames.
// searchType: "file", "directory", or "" for both.
// Results are sorted by fuzzy match score.
func Search(searchDir, query, searchType string, limit int) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	ignoreFile, err := ensureIgnoreFile(home)
	if err != nil {
		return nil, fmt.Errorf("ignore file: %w", err)
	}

	matcher := loadIgnoreMatcher(ignoreFile)
	return searchDir(searchDir, query, searchType, limit, matcher)
}

// searchDir is the internal implementation, testable with custom ignore matchers.
func searchDir(root, query, searchType string, limit int, matcher *ignore.GitIgnore) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	// Phase 1: Walk and collect entries
	type entry struct {
		name    string // basename
		path    string // absolute path
		entType string // "file" or "directory"
	}

	var (
		mu      sync.Mutex
		entries []entry
		done    bool
	)

	conf := fastwalk.Config{
		Follow: false, // don't follow symlinks
	}

	_ = fastwalk.Walk(&conf, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check context cancellation
		if ctx.Err() != nil {
			return filepath.SkipAll
		}

		// Skip the root itself
		if path == root {
			return nil
		}

		// Depth check: count separators relative to root
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(rel, string(filepath.Separator)) + 1
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Ignore check
		if matcher != nil {
			// go-gitignore MatchesPath expects a relative path.
			// Append "/" for directories so patterns like "node_modules/" match.
			matchPath := rel
			if d.IsDir() {
				matchPath = rel + "/"
			}
			if matcher.MatchesPath(matchPath) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Type filter
		isDir := d.IsDir()
		if searchType == "file" && isDir {
			return nil // skip from results but still recurse
		}
		if searchType == "directory" && !isDir {
			return nil
		}

		entType := "file"
		if isDir {
			entType = "directory"
		}

		absPath, _ := filepath.Abs(path)

		mu.Lock()
		if !done {
			entries = append(entries, entry{
				name:    d.Name(),
				path:    absPath,
				entType: entType,
			})
			if len(entries) >= maxCollect {
				done = true
			}
		}
		mu.Unlock()

		if done {
			return filepath.SkipAll
		}
		return nil
	})

	// Phase 2: Fuzzy match against basenames
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}

	matches := fuzzy.Find(strings.ToLower(query), names)

	// Sort by score (descending), then by path length (ascending) as tiebreaker
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return len(entries[matches[i].Index].path) < len(entries[matches[j].Index].path)
	})

	// Phase 3: Build results, cap at limit
	cap := limit
	if cap > len(matches) {
		cap = len(matches)
	}

	results := make([]FileSearchResult, cap)
	for i := 0; i < cap; i++ {
		e := entries[matches[i].Index]
		results[i] = FileSearchResult{
			Name: e.name,
			Path: e.path,
			Type: e.entType,
		}
	}

	return &FileSearchResponse{
		Results: results,
		Query:   query,
		Count:   len(results),
	}, nil
}
```

**Step 4: Delete exec.go and exec_test.go**

Run:
```bash
rm internal/agent/filesearch/exec.go internal/agent/filesearch/exec_test.go
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/agent/filesearch/ -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/agent/filesearch/
git commit -m "feat(filesearch): replace fd with native Go walk + fuzzy match

Use fastwalk for parallel directory traversal, sahilm/fuzzy for
filename matching, and go-gitignore for ignore patterns with full
negation (!) support. Removes external fd dependency."
```

---

### Task 3: Remove `/api/files/search/available` endpoint

**Files:**
- Modify: `internal/agent/api/router.go`
- Modify: `internal/agent/api/files.go`

**Step 1: Remove the route from router.go**

In `internal/agent/api/router.go`, delete line 47:
```go
mux.HandleFunc("GET /api/files/search/available", fh.searchAvailable)
```

**Step 2: Remove the handler from files.go**

In `internal/agent/api/files.go`, delete the `searchAvailable` method (lines 358-363):
```go
// GET /api/files/search/available
func (h *filesHandler) searchAvailable(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]bool{
		"available": filesearch.IsAvailable(),
	})
}
```

Also remove the `filesearch` import if it's no longer used (it is still used by `search()`, so keep it).

**Step 3: Verify build**

Run: `go build ./...`
Expected: Clean build.

**Step 4: Run all tests**

Run: `go test ./internal/agent/api/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/agent/api/router.go internal/agent/api/files.go
git commit -m "feat(api): remove /api/files/search/available endpoint

File search is now native Go — no external dependency to check."
```

---

### Task 4: Update API test

**Files:**
- Modify: `internal/agent/api/files_search_test.go`

**Step 1: Update the router integration test**

The `TestFilesSearch_ViaRouter` test currently accepts `500` as a valid response (for when `fd` is not installed). Since search is now native Go, it should always succeed. Update the test to expect only `200`:

```go
func TestFilesSearch_ViaRouter(t *testing.T) {
	deps := Deps{}
	router := NewRouter(deps)

	req := httptest.NewRequest("GET", "/api/files/search?q=test&type=directory&limit=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp struct {
		Results []json.RawMessage `json:"results"`
		Query   string            `json:"query"`
		Count   int               `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Query != "test" {
		t.Errorf("query = %q, want 'test'", resp.Query)
	}
}
```

**Step 2: Run tests**

Run: `go test ./internal/agent/api/ -v`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add internal/agent/api/files_search_test.go
git commit -m "test(api): update search test — always expect 200 (no fd dependency)"
```

---

### Task 5: Frontend — wire search anchoring

**Files:**
- Modify: `web/src/data/files/keys.ts`
- Modify: `web/src/data/files/queries.ts`
- Modify: `web/src/components/FileBrowser.tsx`
- Modify: `web/src/components/FilePicker/index.tsx`
- Modify: `web/src/components/Workspace/index.tsx`

**Step 1: Update query keys to include searchPath**

In `web/src/data/files/keys.ts`, update the `search` key to include path:

```typescript
search: (query: string, type?: string, searchPath?: string) =>
    [...filesKeys.all, "search", query, type, searchPath] as const,
```

**Step 2: Add searchPath to useFileSearchQuery**

In `web/src/data/files/queries.ts`, update `useFileSearchQuery`:

```typescript
export function useFileSearchQuery(
  query: string,
  options?: { enabled?: boolean; type?: string; limit?: number; searchPath?: string },
) {
  const type = options?.type ?? "directory";
  const limit = options?.limit ?? 20;
  const searchPath = options?.searchPath;

  return useQuery({
    queryKey: filesKeys.search(query, type, searchPath),
    queryFn: () => {
      const params = new URLSearchParams({
        q: query,
        type,
        limit: String(limit),
      });
      if (searchPath) {
        params.set("path", searchPath);
      }
      return apiFetch<FileSearchResponse>(`/agent/api/files/search?${params}`);
    },
    enabled: (options?.enabled ?? true) && query.trim().length > 0,
    staleTime: 30_000,
  });
}
```

**Step 3: Add searchPath prop to FileBrowser**

In `web/src/components/FileBrowser.tsx`, add `searchPath` to the props interface:

```typescript
interface FileBrowserProps {
  open: boolean;
  onSelect: (absolutePath: string) => void;
  onClose: () => void;
  mode: "directory" | "all";
  placeholder?: string;
  initialQuery?: string;
  headerExtra?: React.ReactNode;
  searchPath?: string;  // anchor directory for search queries
}
```

Destructure it in the component and pass to the search query:

```typescript
export function FileBrowser({
  open,
  onSelect,
  onClose,
  mode,
  placeholder,
  initialQuery,
  headerExtra,
  searchPath,
}: FileBrowserProps) {
```

Update the `searchQuery` call to pass `searchPath`:

```typescript
const searchQuery = useFileSearchQuery(debouncedQuery, {
  enabled: open && !isPathMode && !!debouncedQuery,
  type: mode === "directory" ? "directory" : "",
  searchPath,
});
```

**Step 4: Pass searchPath through FilePicker**

In `web/src/components/FilePicker/index.tsx`, add `searchPath` to props:

```typescript
interface FilePickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onPick: (paths: string[]) => void;
  searchPath?: string;
}
```

Destructure and pass to FileBrowser:

```typescript
export function FilePicker({ open, onOpenChange, onPick, searchPath }: FilePickerProps) {
```

```typescript
<FileBrowser
  open={open}
  onSelect={(path) => {
    onPick([path]);
    onOpenChange(false);
  }}
  onClose={() => onOpenChange(false)}
  mode="all"
  placeholder="Search files or type a path..."
  searchPath={searchPath}
  headerExtra={/* ... existing ... */}
/>
```

**Step 5: Pass session working_directory from Workspace**

In `web/src/components/Workspace/index.tsx`, find where `<FilePicker>` is rendered and pass `activeWorkingDirectory`:

```typescript
<FilePicker
  open={showFilePicker}
  onOpenChange={setShowFilePicker}
  onPick={handleFilesPicked}
  searchPath={activeWorkingDirectory}
/>
```

Also check `TerminalToolbar.tsx` — if it renders FilePicker, pass the working directory there too (if available via props).

**Step 6: Verify frontend builds**

Run: `cd web && npm run build`
Expected: Clean build.

**Step 7: Commit**

```bash
git add web/src/data/files/keys.ts web/src/data/files/queries.ts \
  web/src/components/FileBrowser.tsx web/src/components/FilePicker/index.tsx \
  web/src/components/Workspace/index.tsx
git commit -m "feat(frontend): anchor file search to session working directory

FilePicker now accepts a searchPath prop, passed through FileBrowser
to the search API as the 'path' query parameter. Workspace passes
the active session's working_directory."
```

---

### Task 6: Manual verification

**Step 1: Build and run the server**

Run: `make build && ./bin/argus`

**Step 2: Test search from the API directly**

Run:
```bash
# Search from home (default)
curl -s 'http://localhost:3000/agent/api/files/search?q=argus&type=directory' | jq .

# Search from a specific directory
curl -s 'http://localhost:3000/agent/api/files/search?q=main&type=file&path=~/Workspace' | jq .
```

Expected:
- HTTP 200 with fuzzy-matched results
- Results sorted by match quality
- No entries from ignored directories (node_modules, .git, etc.)

**Step 3: Verify negation patterns work**

Ensure `~/.argus/ignore` has the allowlist pattern (`*` + `!Downloads/` + `!Documents/` + `!Workspace/`). Search should only return results from those three directories.

**Step 4: Verify the /available endpoint is gone**

Run: `curl -s 'http://localhost:3000/agent/api/files/search/available'`
Expected: 404 or method not allowed.

**Step 5: Test in the UI**

- Open a new session → use directory picker → search should work from `$HOME`
- Inside a session → open attachments dialog → search should anchor to the session's working directory

**Step 6: Commit (nothing to commit — verification only)**
