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
)

const (
	defaultLimit  = 20
	maxLimit      = 100
	maxDepth      = 8
	maxCollect    = 100_000
	searchTimeout = 5 * time.Second
)

const ignoreFileName = "ignore"

// defaultIgnoreContents is written to ~/.argus/ignore on first use.
var defaultIgnoreContents = strings.TrimSpace(`
# Argus search ignore patterns (gitignore syntax)
# Edit this file to control which directories fd skips during search.
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

// ensureIgnoreFile returns the path to ~/.argus/ignore, creating it
// with sensible defaults if it doesn't already exist.
func ensureIgnoreFile(home string) (string, error) {
	dir := filepath.Join(home, ".argus")
	path := filepath.Join(dir, ignoreFileName)

	if _, err := os.Stat(path); err == nil {
		return path, nil // already exists
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(defaultIgnoreContents), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}

// loadIgnoreMatcher compiles an ignore file into a matcher.
// Returns nil on error so that search proceeds unfiltered.
func loadIgnoreMatcher(path string) *ignore.GitIgnore {
	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return gi
}

// entry holds a collected filesystem entry during the walk phase.
type entry struct {
	path string // absolute path
	name string // basename
	typ  string // "file" or "directory"
}

// entrySource implements fuzzy.Source for case-insensitive matching.
// It returns lowercased basenames so fuzzy.FindFrom matches case-insensitively,
// while the original entry data (name, path) preserves original casing.
type entrySource []entry

func (s entrySource) String(i int) string { return strings.ToLower(s[i].name) }
func (s entrySource) Len() int            { return len(s) }

// Search searches for files/directories matching a fuzzy query.
// searchType: "file", "directory", or "" for both.
// Results are sorted by fuzzy match quality, with shorter paths as tiebreaker.
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
	return searchInDir(searchDir, query, searchType, limit, matcher)
}

// searchInDir is the internal testable search function.
// It walks the directory tree, filters entries, then runs fuzzy matching.
func searchInDir(root, query, searchType string, limit int, matcher *ignore.GitIgnore) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}

	// Make root absolute for consistent relative-path computation.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("abs root: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	var (
		mu      sync.Mutex
		entries []entry
		done    bool
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

		// Check context cancellation / collection cap.
		mu.Lock()
		if done {
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

		// Compute relative path for ignore matching.
		rel, relErr := filepath.Rel(absRoot, path)
		if relErr != nil {
			return nil
		}

		// Check ignore patterns.
		if matcher != nil {
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
			name: filepath.Base(path),
			typ:  typ,
		})
		return nil
	})

	// Ignore walk errors caused by our early-stop signals.
	if walkErr != nil && walkErr != filepath.SkipDir && walkErr != fastwalk.ErrSkipFiles {
		// Only fail for actual errors, not context cancellation (we still
		// want to return partial results).
		if ctx.Err() == nil {
			return nil, fmt.Errorf("walk: %w", walkErr)
		}
	}

	// Run case-insensitive fuzzy matching via entrySource.
	matches := fuzzy.FindFrom(strings.ToLower(query), entrySource(entries))

	// Sort: by score descending (already done by fuzzy.Find),
	// then tiebreak by shorter path.
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return len(entries[matches[i].Index].path) < len(entries[matches[j].Index].path)
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
		Results: results,
		Query:   query,
		Count:   len(results),
	}, nil
}

