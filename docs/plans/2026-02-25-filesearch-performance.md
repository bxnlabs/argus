# File Search Performance Optimizations

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Eliminate wasted work in the file search pipeline — double sort, lock-held computation, redundant `filepath.Rel`, per-request ignore compilation, and missing request context propagation.

**Architecture:** Five targeted, independent optimizations to `operations.go`, `types.go`, and `files.go`. Each fix is surgical: no structural changes, no new dependencies, no API contract changes (except adding `context.Context` as first param to `Search`). The response type gains optional metadata fields for partial/timed-out results.

**Tech Stack:** Go stdlib (`context`, `sync`, `strings`, `sort`), `sahilm/fuzzy` (`FindFromNoSort`), `sabhiram/go-gitignore`

---

### Task 1: Eliminate double sort (F1)

`fuzzy.FindFrom` sorts internally, then we sort again with a tiebreaker. Use `FindFromNoSort` and run one sort pass.

**Files:**
- Modify: `internal/agent/filesearch/operations.go:332-341`

**Step 1: Write the failing test**

Add to `internal/agent/filesearch/operations_test.go`:

```go
func TestSearch_SortTiebreaker(t *testing.T) {
	// Two files with names that produce equal fuzzy scores for "main".
	// The shorter-path entry should rank first (tiebreaker).
	root := createTree(t, map[string]string{
		"a/main.go":       "",
		"a/b/c/main.go":   "",
	})

	resp, err := searchInDir(root, "main", "file", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count < 2 {
		t.Fatalf("expected at least 2 results, got %d", resp.Count)
	}
	// Shorter path should come first when scores are equal.
	if len(resp.Results[0].Path) > len(resp.Results[1].Path) {
		t.Errorf("expected shorter path first: %q vs %q", resp.Results[0].Path, resp.Results[1].Path)
	}
}
```

**Step 2: Run test to verify it passes (baseline)**

Run: `go test ./internal/agent/filesearch/... -run TestSearch_SortTiebreaker -v`
Expected: PASS (tiebreaker already works — this test locks the behavior before we change the sort)

**Step 3: Switch to `FindFromNoSort` and keep one sort pass**

In `operations.go`, replace lines 332-341:

```go
// old:
matches := fuzzy.FindFrom(query, entrySource(entries))

// Sort: by score descending (already done by fuzzy.Find),
// then tiebreak by shorter path.
sort.SliceStable(matches, func(i, j int) bool {
	if matches[i].Score != matches[j].Score {
		return matches[i].Score > matches[j].Score
	}
	return len(entries[matches[i].Index].path) < len(entries[matches[j].Index].path)
})
```

With:

```go
// new:
matches := fuzzy.FindFromNoSort(query, entrySource(entries))

// Single sort pass: score descending, shorter path tiebreak.
sort.SliceStable(matches, func(i, j int) bool {
	if matches[i].Score != matches[j].Score {
		return matches[i].Score > matches[j].Score
	}
	return len(entries[matches[i].Index].path) < len(entries[matches[j].Index].path)
})
```

**Step 4: Run all tests**

Run: `go test ./internal/agent/filesearch/... -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/agent/filesearch/operations.go internal/agent/filesearch/operations_test.go
git commit -m "perf(filesearch): use FindFromNoSort to eliminate redundant sort pass"
```

---

### Task 2: Move `filepath.Rel` and `filepath.Base` outside the mutex (F4)

These are pure computations on local variables — no reason to hold the lock.

**Files:**
- Modify: `internal/agent/filesearch/operations.go:284-316`

**Step 1: Write the failing test**

No new test needed — this is a refactor with no behavior change. Existing tests cover correctness. We just confirm they still pass after.

**Step 2: Move computation before `mu.Lock()`**

Replace lines 279-315 (from `typ := "file"` through the end of the append block):

```go
// old (lines 279-315):
typ := "file"
if isDir {
	typ = "directory"
}

mu.Lock()
defer mu.Unlock()

// Check context and cap under the lock.
select {
case <-ctx.Done():
	done = true
	if d.IsDir() {
		return filepath.SkipDir
	}
	return fastwalk.ErrSkipFiles
default:
}

if len(entries) >= maxCollect {
	done = true
	if d.IsDir() {
		return filepath.SkipDir
	}
	return fastwalk.ErrSkipFiles
}

relPath, _ := filepath.Rel(absRoot, path)
if relPath == "" {
	relPath = filepath.Base(path)
}
entries = append(entries, entry{
	path: path,
	rel:  relPath,
	name: filepath.Base(path),
	typ:  typ,
})
return nil
```

With:

```go
// new:
typ := "file"
if isDir {
	typ = "directory"
}

// Compute outside the lock — these are pure functions of local state.
relPath, _ := filepath.Rel(absRoot, path)
if relPath == "" {
	relPath = filepath.Base(path)
}
baseName := filepath.Base(path)

mu.Lock()
defer mu.Unlock()

// Check context and cap under the lock.
select {
case <-ctx.Done():
	done = true
	if d.IsDir() {
		return filepath.SkipDir
	}
	return fastwalk.ErrSkipFiles
default:
}

if len(entries) >= maxCollect {
	done = true
	if d.IsDir() {
		return filepath.SkipDir
	}
	return fastwalk.ErrSkipFiles
}

entries = append(entries, entry{
	path: path,
	rel:  relPath,
	name: baseName,
	typ:  typ,
})
return nil
```

**Step 3: Run all tests**

Run: `go test ./internal/agent/filesearch/... -v`
Expected: all PASS

**Step 4: Commit**

```bash
git add internal/agent/filesearch/operations.go
git commit -m "perf(filesearch): move filepath.Rel and filepath.Base outside mutex"
```

---

### Task 3: Replace `filepath.Rel` with `strings.TrimPrefix` fast path (F6)

Both `filepath.Rel` calls can be replaced with string slicing since walked paths are always under their respective roots.

**Files:**
- Modify: `internal/agent/filesearch/operations.go:252-267` (ignore-root rel), `operations.go:306` (search-root rel)

**Step 1: Write the failing test**

Add to `operations_test.go`:

```go
func TestSearch_RelativePathsCorrect(t *testing.T) {
	// Ensure results have correct relative paths used for fuzzy matching,
	// even with nested search roots.
	root := createTree(t, map[string]string{
		"src/main.go":            "",
		"src/lib/utils.go":      "",
		"src/lib/deep/inner.go": "",
	})

	resp, err := searchInDir(root, "utils", "file", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected results")
	}
	// The result path should be absolute and contain the expected file.
	found := false
	for _, r := range resp.Results {
		if r.Name == "utils.go" {
			found = true
			if !strings.HasPrefix(r.Path, root) {
				t.Errorf("result path %q should start with root %q", r.Path, root)
			}
		}
	}
	if !found {
		t.Error("expected to find utils.go")
	}
}
```

**Step 2: Run test to verify it passes (baseline)**

Run: `go test ./internal/agent/filesearch/... -run TestSearch_RelativePathsCorrect -v`
Expected: PASS

**Step 3: Add `fastRel` helper and replace both `filepath.Rel` calls**

Add helper near the top of `operations.go` (after imports, before constants):

```go
// fastRel computes a relative path when base is known to be a prefix of target.
// Falls back to filepath.Rel for edge cases.
func fastRel(base, target string) string {
	if strings.HasPrefix(target, base) {
		rel := target[len(base):]
		if len(rel) > 0 && rel[0] == filepath.Separator {
			rel = rel[1:]
		}
		if rel != "" {
			return rel
		}
	}
	// Fallback for edge cases (symlinks, non-clean paths).
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.Base(target)
	}
	return rel
}
```

Then replace the ignore-root `filepath.Rel` call (line 254):

```go
// old:
ignoreRel, relErr := filepath.Rel(absIgnoreRoot, path)
if relErr == nil && !strings.HasPrefix(ignoreRel, "..") {
```

With:

```go
// new:
ignoreRel := fastRel(absIgnoreRoot, path)
if !strings.HasPrefix(ignoreRel, "..") {
```

And replace the search-root `filepath.Rel` call (the block added in Task 2):

```go
// old:
relPath, _ := filepath.Rel(absRoot, path)
if relPath == "" {
	relPath = filepath.Base(path)
}
```

With:

```go
// new:
relPath := fastRel(absRoot, path)
```

**Step 4: Run all tests**

Run: `go test ./internal/agent/filesearch/... -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/agent/filesearch/operations.go internal/agent/filesearch/operations_test.go
git commit -m "perf(filesearch): replace filepath.Rel with string prefix slicing"
```

---

### Task 4: Cache the compiled ignore matcher (F7)

Avoid recompiling `~/.argus/ignore` on every request. Cache with mtime-based invalidation.

**Files:**
- Modify: `internal/agent/filesearch/operations.go:127-135` (loadIgnoreMatcher), `operations.go:171-176` (Search caller)

**Step 1: Write the failing test**

Add to `operations_test.go`:

```go
func TestCachedIgnoreMatcher(t *testing.T) {
	home := t.TempDir()
	path, err := ensureIgnoreFile(home)
	if err != nil {
		t.Fatal(err)
	}

	// First load — should compile and cache.
	m1 := loadIgnoreMatcher(path)
	if m1 == nil {
		t.Fatal("expected non-nil matcher")
	}

	// Second load — should return cached (same pointer).
	m2 := loadIgnoreMatcher(path)
	if m2 != m1 {
		t.Error("expected cached matcher (same pointer) on second load")
	}

	// Modify the file — should recompile.
	if err := os.WriteFile(path, []byte("newpattern/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m3 := loadIgnoreMatcher(path)
	if m3 == m1 {
		t.Error("expected new matcher after file modification")
	}

	// Reset cache for other tests.
	resetMatcherCache()
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/filesearch/... -run TestCachedIgnoreMatcher -v`
Expected: FAIL — `resetMatcherCache` undefined, and pointer comparison fails

**Step 3: Implement mtime-based caching**

Replace the `loadIgnoreMatcher` function and add cache state:

```go
var (
	matcherCache    *ignore.GitIgnore
	matcherPath     string
	matcherModTime  time.Time
	matcherMu       sync.RWMutex
)

// resetMatcherCache clears the cache (for testing).
func resetMatcherCache() {
	matcherMu.Lock()
	defer matcherMu.Unlock()
	matcherCache = nil
	matcherPath = ""
	matcherModTime = time.Time{}
}

// loadIgnoreMatcher returns a compiled ignore matcher, using a cached
// version if the file hasn't been modified since last compilation.
// Returns nil on error so that search proceeds unfiltered.
func loadIgnoreMatcher(path string) *ignore.GitIgnore {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	modTime := info.ModTime()

	// Fast path: check cache under read lock.
	matcherMu.RLock()
	if matcherPath == path && matcherModTime.Equal(modTime) && matcherCache != nil {
		cached := matcherCache
		matcherMu.RUnlock()
		return cached
	}
	matcherMu.RUnlock()

	// Slow path: recompile under write lock.
	matcherMu.Lock()
	defer matcherMu.Unlock()

	// Double-check after acquiring write lock.
	if matcherPath == path && matcherModTime.Equal(modTime) && matcherCache != nil {
		return matcherCache
	}

	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	matcherCache = gi
	matcherPath = path
	matcherModTime = modTime
	return gi
}
```

**Step 4: Run all tests**

Run: `go test ./internal/agent/filesearch/... -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/agent/filesearch/operations.go internal/agent/filesearch/operations_test.go
git commit -m "perf(filesearch): cache compiled ignore matcher with mtime invalidation"
```

---

### Task 5: Propagate request context (F5)

Pass `r.Context()` from the HTTP handler so cancelled requests stop in-flight searches.

**Files:**
- Modify: `internal/agent/filesearch/operations.go:154-178` (Search signature + searchInDir)
- Modify: `internal/agent/api/files.go:349` (caller)
- Modify: `internal/agent/filesearch/operations_test.go` (all `searchInDir` calls)

**Step 1: Write the failing test**

Add to `operations_test.go`:

```go
func TestSearch_RespectsContextCancellation(t *testing.T) {
	// Create enough files to make the walk non-trivial.
	tree := map[string]string{}
	for i := 0; i < 100; i++ {
		tree[filepath.Join("dir", fmt.Sprintf("file%03d.txt", i))] = ""
	}
	root := createTree(t, tree)

	// Cancel context immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := searchInDir(ctx, root, "file", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// With a cancelled context, we should get few or zero results.
	// The exact count depends on timing, but it should complete without error.
	_ = resp
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/filesearch/... -run TestSearch_RespectsContextCancellation -v`
Expected: FAIL — `searchInDir` doesn't accept `context.Context`

**Step 3: Add `context.Context` to `searchInDir` and `Search`**

Update `Search` signature (line 157):

```go
// old:
func Search(searchDir, query, searchType string, limit int) (*FileSearchResponse, error) {
```

```go
// new:
func Search(ctx context.Context, searchDir, query, searchType string, limit int) (*FileSearchResponse, error) {
```

Update `Search` body — pass `ctx` to `searchInDir` (line 177):

```go
// old:
return searchInDir(searchDir, query, searchType, limit, matcher, home)
```

```go
// new:
return searchInDir(ctx, searchDir, query, searchType, limit, matcher, home)
```

Update `searchInDir` signature (line 184):

```go
// old:
func searchInDir(root, query, searchType string, limit int, matcher *ignore.GitIgnore, ignoreRoot string) (*FileSearchResponse, error) {
```

```go
// new:
func searchInDir(ctx context.Context, root, query, searchType string, limit int, matcher *ignore.GitIgnore, ignoreRoot string) (*FileSearchResponse, error) {
```

Update context creation (line 211) — derive timeout from caller's context:

```go
// old:
ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
defer cancel()
```

```go
// new:
ctx, cancel := context.WithTimeout(ctx, searchTimeout)
defer cancel()
```

Update the API handler call site in `files.go` (line 349):

```go
// old:
result, err := filesearch.Search(searchPath, query, searchType, limit)
```

```go
// new:
result, err := filesearch.Search(r.Context(), searchPath, query, searchType, limit)
```

**Step 4: Update all `searchInDir` calls in tests**

Every `searchInDir(...)` call in `operations_test.go` needs `context.Background()` as the first argument. Add `"context"` and `"fmt"` to the test file imports.

Pattern — for every existing call like:

```go
resp, err := searchInDir(root, "ctrl", "", 20, nil, "")
```

Change to:

```go
resp, err := searchInDir(context.Background(), root, "ctrl", "", 20, nil, "")
```

Apply this to all 18 `searchInDir` calls in the test file.

**Step 5: Run all tests (filesearch + API)**

Run: `go test ./internal/agent/filesearch/... ./internal/agent/api/... -v`
Expected: all PASS

**Step 6: Commit**

```bash
git add internal/agent/filesearch/operations.go internal/agent/filesearch/operations_test.go internal/agent/api/files.go
git commit -m "perf(filesearch): propagate request context to cancel abandoned searches"
```

---

### Task 6: Run full test suite and verify

**Step 1: Run all project tests**

Run: `go test ./... -count=1`
Expected: all PASS

**Step 2: Verify no regressions in API tests**

Run: `go test ./internal/agent/api/... -v -count=1`
Expected: all PASS, including `TestFilesSearch_ViaRouter`
