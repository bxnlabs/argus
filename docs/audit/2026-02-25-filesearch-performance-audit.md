# File Search Performance Audit

**Date:** 2026-02-25
**Scope:** `internal/agent/filesearch/operations.go`, `types.go`, API handler in `internal/agent/api/files.go`
**Method:** Automated review (Codex gpt-5.3-codex, xhigh reasoning, 2 independent reviewers) with manual validation against source

---

## Architecture Overview

The file search pipeline follows: **Walk -> Filter -> Collect -> Fuzzy Match -> Sort -> Return**.

- **Walk:** `charlievieth/fastwalk` (parallel directory traversal, ~2.5x faster than stdlib on macOS)
- **Filter:** `sabhiram/go-gitignore` compiled from `~/.argus/ignore`
- **Collect:** Up to 100K entries into `[]entry` under mutex
- **Match:** `sahilm/fuzzy.FindFrom` (VSCode-style scoring, linear scan of all collected entries)
- **Sort:** Score descending, path length ascending tiebreak
- **Return:** Top `limit` results (default 20, max 100)

Constants: `maxDepth=8`, `maxCollect=100,000`, `searchTimeout=5s`

---

## Findings

### F1. Double sort on fuzzy results — High

**Location:** `operations.go:332`, `operations.go:336`

`fuzzy.FindFrom` internally calls `sort.Stable(matches)` by score descending (`fuzzy.go:98-101`). The code then immediately runs a second `sort.SliceStable` with score descending + path-length tiebreak. The first sort is wasted `O(m log m)` work.

**Fix:** Use `fuzzy.FindFromNoSort` (available at `fuzzy.go:108`) and run one custom sort pass. Better yet, use a top-K heap (`O(m log limit)`) instead of full sort since only `limit` results are needed.

---

### F2. 100K cap drops best matches non-deterministically — High

**Location:** `operations.go:23`, `operations.go:298-304`

The `maxCollect` cap is enforced during the walk phase, before fuzzy matching runs. Since `fastwalk` visits entries concurrently in non-deterministic order, the set of 100K entries that survive the cap is unstable between identical queries. Best matches outside the first 100K visited are silently dropped.

This is especially relevant when the API defaults to `$HOME` as the search root (`files.go:315-321`), which can easily contain millions of entries.

**Fix:** Consider depth-first/breadth-first deterministic ordering, or a two-phase approach (shallow first, then deeper). Alternatively, raise the cap or replace with a streaming top-K approach that keeps high-scoring candidates.

---

### F3. 5s timeout only covers walking, not scoring/sorting — High

**Location:** `operations.go:211`, `operations.go:332-346`

The timeout context created at line 211 is only checked during the walk callback (lines 238, 288-296). After the walk completes, `fuzzy.FindFrom` runs a linear scan over all collected entries (`fuzzy.go:115`: `for i := 0; i < data.Len(); i++`) with no deadline check. Sorting is also unbounded.

On a full 100K-entry collection, the fuzzy+sort phase can add significant latency beyond the 5s budget with no mechanism to abort.

**Fix:** Make the timeout deadline-aware through the entire pipeline. Consider chunked fuzzy matching with periodic context checks.

---

### F4. Lock contention in fastwalk callback — Medium

**Location:** `operations.go:237-245`, `operations.go:284-316`

The walk callback acquires `mu` twice per entry:
1. Lines 237-245: Check `done` flag and `ctx.Err()` (cheap, short hold).
2. Lines 284-316: Compute `filepath.Rel`, `filepath.Base`, and append to `entries` slice (expensive, longer hold with `defer mu.Unlock()`).

The first `filepath.Rel` call for ignore matching (line 254) is correctly outside the lock. However, the second `filepath.Rel(absRoot, path)` at line 306 and `filepath.Base(path)` at line 313 run inside the lock, which is avoidable since they don't depend on shared state.

**Fix:** Compute `relPath` and `basename` before acquiring the second lock. Only hold `mu` for the bounds check + append.

---

### F5. No request context propagation — Medium

**Location:** `files.go:349`, `operations.go:157`, `operations.go:211`

The HTTP handler calls `filesearch.Search(searchPath, query, searchType, limit)` without passing `r.Context()`. The `Search` function creates its own context via `context.WithTimeout(context.Background(), ...)`. If the client disconnects (e.g., user types next character during typeahead), the in-flight search continues running to completion.

**Fix:** Change `Search` signature to accept `context.Context` as the first parameter. Derive the internal timeout from the caller's context:
```go
func Search(ctx context.Context, searchDir, query, searchType string, limit int) (*FileSearchResponse, error)
```
Call with `r.Context()` from the handler.

---

### F6. Two `filepath.Rel` calls per entry — Medium

**Location:** `operations.go:254`, `operations.go:306`

Every entry that passes ignore filtering incurs two `filepath.Rel` calls:
1. `filepath.Rel(absIgnoreRoot, path)` — for ignore pattern matching (root = `$HOME`)
2. `filepath.Rel(absRoot, path)` — for the collected relative path (root = search directory)

These use different roots so they can't be trivially deduplicated, but both can be replaced with `strings.TrimPrefix` fast paths since walked paths are always under their respective roots:
```go
// Fast path: absRoot is always a prefix of path during walk.
relPath := path[len(absRoot)+1:] // skip root + separator
```
With a fallback to `filepath.Rel` for edge cases.

---

### F7. Ignore matcher recompiled every request — Medium

**Location:** `operations.go:171-176`

Every call to `Search()` runs `ensureIgnoreFile(home)` (stat + possible write) then `loadIgnoreMatcher(ignoreFile)` which calls `ignore.CompileIgnoreFile(path)` (file read + regex compilation). With debounced UI search (~300ms), this is repeated work.

**Fix:** Cache the compiled `*ignore.GitIgnore` at package level with mtime-based invalidation:
```go
var (
    cachedMatcher     *ignore.GitIgnore
    cachedMtime       time.Time
    cachedIgnorePath  string
    matcherMu         sync.RWMutex
)
```
Check file mtime before recompiling. Saves ~1-10ms per request depending on ignore file complexity.

---

### F8. Entry struct is memory-heavy for 100K cap — Low

**Location:** `operations.go:137-143`

Each `entry` stores 4 strings:
```go
type entry struct {
    path string // absolute path
    rel  string // relative path (often a substring of path)
    name string // filepath.Base(path) — derivable from path
    typ  string // "file" or "directory" — could be a bool
}
```

At 100K entries, this is 400K string headers (each 16 bytes on 64-bit = 6.4MB of headers alone) plus string data. The `name` field is always derivable from `path`, and `typ` could be a single `bool`.

**Fix:** Slim to `(rel string, isDir bool)` or `(rel string, path string, isDir bool)`. Derive `name` and `typ` only for the final top-`limit` results.

---

### F9. Ranking heuristic is narrow — Low

**Location:** `operations.go:336-341`

Sort is by fuzzy score descending, then absolute path length ascending as tiebreaker. This misses useful ranking signals:
- Exact basename match (e.g., query "main.go" should strongly prefer a file literally named `main.go`)
- Basename prefix match
- Path depth (shallower results are often more relevant)
- Deterministic lexical tiebreak (avoids result flicker between equal-scoring entries)

**Fix:** Add composite scoring that boosts exact/prefix basename matches and penalizes depth. Add a final lexical tiebreak on `rel` for determinism.

---

### F10. No filesystem caching or indexing — Low (architectural)

**Location:** `operations.go:225-317`

Every `Search()` call runs a full `fastwalk.Walk` from scratch. For interactive typeahead with ~300ms debounce, this means re-walking the entire directory tree on every keystroke that passes the debounce threshold.

**Fix (if warranted by profiling):** Introduce a per-root entry snapshot cache with TTL-based invalidation:
- Key: `(absRoot, searchType, ignoreFileVersion)`
- TTL: 10-30s
- Background refresh on cache miss
- Prefix reuse: if new query extends old query, search prior candidate set only

This is a larger architectural change. Profile first to determine if walk time dominates latency for typical search roots before investing.

---

## Response Type Gap

**Location:** `types.go:10-15`

`FileSearchResponse` has no signal for degraded results. When the walk hits the 100K cap or the 5s timeout, the client receives results indistinguishable from a complete search.

**Fix:** Extend the response:
```go
type FileSearchResponse struct {
    Results  []FileSearchResult `json:"results"`
    Query    string             `json:"query"`
    Count    int                `json:"count"`
    Partial  bool               `json:"partial,omitempty"`
    TimedOut bool               `json:"timedOut,omitempty"`
    Scanned  int                `json:"scanned,omitempty"`
}
```

---

## Testing Gaps

- No benchmarks exist in `operations_test.go`. The following benchmarks would support optimization work and prevent regressions:
  - Large tree (100K entries) — measures walk + collect + match end-to-end
  - High match-rate query — stresses fuzzy scoring and sorting
  - Cap-hit scenario — validates behavior when `maxCollect` is reached
  - Ignore matcher compilation — measures repeated request overhead
- No tests for timeout behavior or partial result correctness
- No tests for request context cancellation (requires the `context.Context` parameter fix)

---

## Summary

| # | Finding | Severity | Validated | Status |
|---|---------|----------|-----------|--------|
| F1 | Double sort on fuzzy results | High | Yes | **Fixed** — `FindFromNoSort` + single sort pass |
| F2 | 100K cap non-deterministic | High | Yes | Open |
| F3 | Timeout doesn't cover scoring/sorting | High | Yes | **Fixed** — `ctx.Err()` check after walk bails out before fuzzy+sort when request is stale; response metadata added |
| F4 | Lock contention in walk callback | Medium | Partially — first `filepath.Rel` is outside lock | **Fixed** — `filepath.Rel` and `filepath.Base` moved outside mutex |
| F5 | No request context propagation | Medium | Yes | **Fixed** — `context.Context` threaded through `Search`/`searchInDir`; handler passes `r.Context()` |
| F6 | Two `filepath.Rel` calls per entry | Medium | Yes — different roots, both replaceable with prefix slicing | **Fixed** — replaced with `fastRel` string prefix slicing (with path-segment boundary check) |
| F7 | Ignore matcher recompiled each request | Medium | Yes | **Fixed** — mtime-based cache with `sync.RWMutex` double-checked locking |
| F8 | Entry struct memory-heavy | Low | Yes | Open |
| F9 | Narrow ranking heuristic | Low | Yes | **Fixed** — tiebreaker changed from path length to depth + lexical rel path for determinism |
| F10 | No filesystem caching/indexing | Low | Yes | Open |
