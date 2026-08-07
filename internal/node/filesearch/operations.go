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
	ignore "github.com/sabhiram/go-gitignore"
	"github.com/sahilm/fuzzy"

	"github.com/bxnlabs/argus/internal/shared"
)

// fastRel computes a relative path when base is known to be a prefix of target.
// Falls back to filepath.Rel for edge cases.
func fastRel(base, target string) string {
	if strings.HasPrefix(target, base) {
		rel := target[len(base):]
		if len(rel) == 0 {
			// target == base; fall through to filepath.Rel.
		} else if rel[0] == filepath.Separator {
			// Clean boundary: base="/a/b", target="/a/b/c" → "c".
			return rel[1:]
		} else {
			// Not a path-segment boundary: base="/a/b", target="/a/bc".
			// Fall through to filepath.Rel for correct result.
		}
	}
	// Fallback for edge cases (symlinks, non-clean paths).
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.Base(target)
	}
	return rel
}

const (
	defaultLimit  = 20
	maxLimit      = 100
	maxDepth      = 8
	maxCollect    = 100_000
	searchTimeout = 5 * time.Second
)

// basenameMatchTier classifies how well a lowercase query matches a filename.
//
//	0 = exact match        (e.g. query "argus" ↔ name "argus")
//	1 = prefix match       (e.g. query "main"  ↔ name "main.go")
//	2 = substring match    (e.g. query "argus" ↔ name ".argus")
//	3 = no basename match  (match is path-only)
func basenameMatchTier(name, lowerQuery string) int {
	name = strings.ToLower(name)
	switch {
	case name == lowerQuery:
		return 0
	case strings.HasPrefix(name, lowerQuery):
		return 1
	case strings.Contains(name, lowerQuery):
		return 2
	default:
		return 3
	}
}

const ignoreFileName = "ignore"

// defaultIgnoreContents is written to ~/.argus/ignore on first use.
var defaultIgnoreContents = strings.TrimSpace(`
# Argus search ignore patterns (gitignore syntax)
# Edit this file to control which directories are skipped during search.
# See: https://git-scm.com/docs/gitignore#_pattern_format

# Version control
.git/

# IDE & editor state
.vscode/
.cursor/
.claude/
.idea/

# Node / npm
node_modules/
.npm/
.nvm/
.pnpm-store/
.yarn/
jspm_packages/
bower_components/
.next/
.nuxt/
.output/
.svelte-kit/
.vite/
.parcel-cache/
.cache/
.eslintcache
.stylelintcache
*.tsbuildinfo

# Python
__pycache__/
*.py[codz]
.venv/
venv/
env/
.eggs/
*.egg-info/
.tox/
.nox/
.mypy_cache/
.ruff_cache/
.pytest_cache/
.hypothesis/
.coverage
htmlcov/
.ipynb_checkpoints/
.pixi/

# Rust
target/
**/*.rs.bk

# Go
*.test
*.out

# Build artifacts
build/
dist/
out/
coverage/
*.lcov

# Environment & secrets
.env
.env.*

# Misc caches & local data
.local/
.config/
.cargo/
.rustup/
`) + "\n"

// ensureIgnoreFile returns the path to <stateDir>/ignore, creating it
// with sensible defaults if it doesn't already exist.
func ensureIgnoreFile(stateDir string) (string, error) {
	path := filepath.Join(stateDir, ignoreFileName)

	if _, err := os.Stat(path); err == nil {
		return path, nil // already exists
	}

	if err := shared.EnsureSecureDir(stateDir); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", stateDir, err)
	}
	if err := os.WriteFile(path, []byte(defaultIgnoreContents), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

var (
	matcherCache   *ignore.GitIgnore
	matcherPath    string
	matcherModTime time.Time
	matcherMu      sync.RWMutex
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

// entry holds a collected filesystem entry during the walk phase.
type entry struct {
	path string // absolute path
	rel  string // path relative to search root (used for fuzzy matching)
	name string // basename
	typ  string // "file" or "directory"
}

// entrySource implements fuzzy.Source for fuzzy matching.
// Case-insensitive matching is handled natively by sahilm/fuzzy via equalFold.
// Returning the relative path lets queries span directory components
// (e.g. "internalsearch" matches "internal/node/filesearch/operations.go").
type entrySource []entry

func (s entrySource) String(i int) string { return s[i].rel }
func (s entrySource) Len() int            { return len(s) }

// Search searches for files/directories matching a fuzzy query.
// searchType: "file", "directory", or "" for both.
// Results are sorted by fuzzy match quality, with shorter paths as tiebreaker.
func Search(ctx context.Context, searchDir, query, searchType string, limit int) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}

	stateDir, err := shared.StateDir()
	if err != nil {
		return nil, fmt.Errorf("state dir: %w", err)
	}
	ignoreFile, err := ensureIgnoreFile(stateDir)
	if err != nil {
		return nil, fmt.Errorf("ignore file: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	matcher := loadIgnoreMatcher(ignoreFile)
	return searchInDir(ctx, searchDir, query, searchType, limit, matcher, home)
}

// searchInDir is the internal testable search function.
// It walks the directory tree, filters entries, then runs fuzzy matching.
// ignoreRoot is the directory that ignore patterns are relative to (typically $HOME).
// If empty, patterns are relative to root.
func searchInDir(ctx context.Context, root, query, searchType string, limit int, matcher *ignore.GitIgnore, ignoreRoot string) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}

	// Make root absolute for consistent relative-path computation.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}

	// Determine the base for ignore-pattern matching. Ignore patterns in
	// ~/.argus/ignore use paths relative to $HOME (e.g. "!Workspace/").
	// When searching a subdirectory, we still need to compute paths relative
	// to $HOME so scope patterns (like * + !Workspace/) work correctly.
	absIgnoreRoot := absRoot
	if ignoreRoot != "" {
		if ir, err := filepath.Abs(ignoreRoot); err == nil {
			absIgnoreRoot = ir
		}
	}

	ctx, cancel := context.WithTimeout(ctx, searchTimeout)
	defer cancel()

	var (
		mu       sync.Mutex
		entries  []entry
		done     bool
		capped   bool // true if maxCollect was reached
		timedOut bool // true if context deadline/cancellation stopped the walk
	)

	conf := &fastwalk.Config{
		Follow:   false,
		MaxDepth: maxDepth,
	}

	walkErr := fastwalk.Walk(conf, absRoot, func(path string, d fs.DirEntry, walkError error) error {
		if walkError != nil {
			// Skip entries we cannot read.
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Fast exit if another goroutine already triggered early stop,
		// or if the context has been cancelled.
		// NOTE: done is read under mu — no data race.
		mu.Lock()
		if done || ctx.Err() != nil {
			mu.Unlock()
			if d.IsDir() {
				return filepath.SkipDir
			}
			return fastwalk.ErrSkipFiles
		}
		mu.Unlock()

		// Skip the root directory itself.
		if path == absRoot {
			return nil
		}

		// Check ignore patterns using paths relative to ignoreRoot.
		if matcher != nil {
			ignoreRel := fastRel(absIgnoreRoot, path)
			if !strings.HasPrefix(ignoreRel, "..") {
				matchPath := ignoreRel
				if d.IsDir() {
					matchPath = ignoreRel + "/"
				}
				if matcher.MatchesPath(matchPath) {
					if d.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}

		// Filter by type.
		isDir := d.IsDir()
		if searchType == "file" && isDir {
			// Don't skip the directory, just don't collect it.
			return nil
		}
		if searchType == "directory" && !isDir {
			return nil
		}

		typ := "file"
		if isDir {
			typ = "directory"
		}

		// Compute outside the lock — these are pure functions of local state.
		relPath := fastRel(absRoot, path)
		baseName := filepath.Base(path)

		mu.Lock()
		defer mu.Unlock()

		// Check context and cap under the lock.
		select {
		case <-ctx.Done():
			done = true
			timedOut = true
			if d.IsDir() {
				return filepath.SkipDir
			}
			return fastwalk.ErrSkipFiles
		default:
		}

		if len(entries) >= maxCollect {
			done = true
			capped = true
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
	})

	// Ignore walk errors caused by our early-stop signals.
	// filepath.SkipDir can propagate when fastwalk's readDir fails and the
	// callback returns SkipDir for the error re-invocation. ErrSkipFiles is
	// consumed internally by fastwalk but is checked here defensively.
	if walkErr != nil && walkErr != filepath.SkipDir && walkErr != fastwalk.ErrSkipFiles {
		// Only fail for actual errors, not context cancellation (we still
		// want to return partial results).
		if ctx.Err() == nil {
			return nil, fmt.Errorf("walk: %w", walkErr)
		}
	}

	// If the context has been cancelled (e.g. client disconnected during
	// typeahead), skip the fuzzy+sort work entirely and return what we have.
	if ctx.Err() != nil {
		timedOut = true
		return &FileSearchResponse{
			Results:  nil,
			Query:    query,
			Count:    0,
			Partial:  true,
			TimedOut: true,
			Scanned:  len(entries),
		}, nil
	}

	// Run case-insensitive fuzzy matching via entrySource.
	matches := fuzzy.FindFromNoSort(query, entrySource(entries))

	// Pre-compute a basename-match tier for each matched entry.
	// The fuzzy library's greedy algorithm can mis-align characters across
	// path components (e.g. matching 'a' from "Workspace" and 'r' from
	// "repos" instead of the contiguous "argus" in a deeper component).
	// Tiering by basename match quality corrects this.
	lowerQuery := strings.ToLower(query)
	tier := make(map[int]int, len(matches))
	for _, m := range matches {
		tier[m.Index] = basenameMatchTier(entries[m.Index].name, lowerQuery)
	}

	// Pre-compute depth (number of path separators in rel) for sort tiebreaking.
	// Depth is a better proximity signal than absolute path length, which
	// conflates long directory names with deep nesting.
	depth := make(map[int]int, len(matches))
	for _, m := range matches {
		depth[m.Index] = strings.Count(entries[m.Index].rel, string(filepath.Separator))
	}

	// Sort by: basename tier (ascending) → fuzzy score (descending) →
	// depth (ascending) → lexical rel path (ascending, deterministic tiebreak).
	sort.SliceStable(matches, func(i, j int) bool {
		ii, jj := matches[i].Index, matches[j].Index
		if tier[ii] != tier[jj] {
			return tier[ii] < tier[jj]
		}
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if depth[ii] != depth[jj] {
			return depth[ii] < depth[jj]
		}
		return entries[ii].rel < entries[jj].rel
	})

	// Take top `limit` results.
	if len(matches) > limit {
		matches = matches[:limit]
	}

	results := make([]FileSearchResult, len(matches))
	for i, m := range matches {
		e := entries[m.Index]
		results[i] = FileSearchResult{
			Name: e.name,
			Path: e.path,
			Type: e.typ,
		}
	}

	return &FileSearchResponse{
		Results:  results,
		Query:    query,
		Count:    len(results),
		Partial:  capped || timedOut,
		TimedOut: timedOut,
		Scanned:  len(entries),
	}, nil
}
