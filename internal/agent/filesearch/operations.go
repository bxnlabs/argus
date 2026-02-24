package filesearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultLimit    = 20
	maxLimit        = 100
	maxDepth        = 8
	overFetchFactor = 3 // fetch more results for better relevance sorting
)

const ignoreFileName = "search-ignore"

// defaultIgnoreContents is written to ~/.argus/search-ignore on first use.
var defaultIgnoreContents = strings.TrimSpace(`
# Argus search ignore patterns (gitignore syntax)
# Edit this file to control which directories fd skips during search.
# See: https://git-scm.com/docs/gitignore#_pattern_format

# Version control
.git/

# Package managers & toolchains
node_modules/
.npm/
.nvm/
.cargo/
.rustup/

# IDE & editor state
.vscode/
.cursor/
.claude/

# Caches & local data
.cache/
.local/
.config/

# macOS
Library/
`) + "\n"

// ensureIgnoreFile returns the path to ~/.argus/search-ignore, creating it
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

// Search searches for files/directories using fd.
// searchType: "file", "directory", or "" for both.
// Results are sorted by relevance (exact > prefix > contains, shorter paths first).
func Search(searchDir, query, searchType string, limit int) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), fdTimeout)
	defer cancel()

	args := buildFdArgs(query, searchType, limit)
	output, err := runFd(ctx, searchDir, maxOutputBuffer, args...)
	if err != nil {
		return nil, err
	}

	results := parseOutput(output, query, searchType, limit)

	return &FileSearchResponse{
		Results: results,
		Query:   query,
		Count:   len(results),
	}, nil
}

// hiddenExcludes are noisy hidden directories to skip when searching with --hidden.
// We need --hidden to traverse directories like .worktrees, but these dirs
// produce excessive noise without useful results.
var hiddenExcludes = []string{
	".cache", ".local", ".config", ".cursor", ".claude", ".vscode",
	".git", "node_modules", ".npm", ".nvm", ".cargo", ".rustup",
}

// buildFdArgs constructs fd command arguments.
func buildFdArgs(query, searchType string, limit int) []string {
	args := []string{
		"-i",
		"--hidden",
		"--max-depth", fmt.Sprintf("%d", maxDepth),
		"--max-results", fmt.Sprintf("%d", limit*overFetchFactor),
		"--absolute-path",
	}

	for _, exc := range hiddenExcludes {
		args = append(args, "--exclude", exc)
	}

	switch searchType {
	case "directory":
		args = append(args, "-t", "d")
	case "file":
		args = append(args, "-t", "f")
	}

	args = append(args, query)
	return args
}

// parseOutput parses fd output (one path per line), sorts by relevance, caps at limit.
func parseOutput(output, query, searchType string, limit int) []FileSearchResult {
	if strings.TrimSpace(output) == "" {
		return []FileSearchResult{}
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	results := make([]FileSearchResult, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Determine type from searchType param or trailing slash.
		// fd --absolute-path outputs directories with a trailing "/".
		itemType := searchType
		if itemType == "" {
			if strings.HasSuffix(line, "/") {
				itemType = "directory"
				line = strings.TrimSuffix(line, "/")
			} else {
				itemType = "file"
			}
		}

		results = append(results, FileSearchResult{
			Name: filepath.Base(line),
			Path: line,
			Type: itemType,
		})
	}

	sortByRelevance(results, query)

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}

// sortByRelevance sorts by: exact name > prefix > contains, shorter paths first.
func sortByRelevance(results []FileSearchResult, query string) {
	queryLower := strings.ToLower(query)

	sort.SliceStable(results, func(i, j int) bool {
		scoreI := relevanceScore(strings.ToLower(results[i].Name), queryLower)
		scoreJ := relevanceScore(strings.ToLower(results[j].Name), queryLower)

		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return len(results[i].Path) < len(results[j].Path)
	})
}

// relevanceScore: exact=3, prefix=2, contains=1, other=0.
func relevanceScore(name, query string) int {
	if name == query {
		return 3
	}
	if strings.HasPrefix(name, query) {
		return 2
	}
	if strings.Contains(name, query) {
		return 1
	}
	return 0
}
