package search

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Search searches for query in dir using ripgrep.
// The query is treated as a regular expression (ripgrep syntax).
// Returns SearchResult with matches capped at maxResults (max 100).
// Creates its own timeout context (10s).
func Search(dir, query string, maxResults int) (*SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if maxResults < 1 || maxResults > maxResultsHardLimit {
		maxResults = maxResultsHardLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	args := []string{
		"--json",
		"--max-count", strconv.Itoa(maxResults),
		"--ignore-case",
		"--",
		query,
		".",
	}

	output, err := runRipgrep(ctx, dir, maxOutputBuffer, args...)
	if err != nil {
		return nil, err
	}

	matches, parseErr := parseRgJSON(output, dir, maxResults)

	// Return partial results even if the scanner failed mid-stream
	// (e.g., a single JSON line exceeded the 1MB buffer).
	return &SearchResult{
		Results: matches,
		Query:   query,
		Path:    dir,
		Count:   len(matches),
	}, parseErr
}

// IsAvailable checks if ripgrep is installed and runnable.
func IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "rg", "--version")
	return cmd.Run() == nil
}

// parseRgJSON parses line-delimited JSON from ripgrep --json output.
// Only processes type:"match" entries; skips all others.
// Caps results at maxResults.
func parseRgJSON(output, searchDir string, maxResults int) ([]SearchMatch, error) {
	if strings.TrimSpace(output) == "" {
		return []SearchMatch{}, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(output))
	// 1MB token size to handle long lines (minified JS, etc.)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)

	matches := []SearchMatch{}

	for scanner.Scan() {
		if len(matches) >= maxResults {
			break
		}

		var msg struct {
			Type string `json:"type"`
			Data struct {
				Path struct {
					Text string `json:"text"`
				} `json:"path"`
				Lines struct {
					Text string `json:"text"`
				} `json:"lines"`
				LineNumber int `json:"line_number"`
				Submatches []struct {
					Match struct {
						Text string `json:"text"`
					} `json:"match"`
					Start int `json:"start"`
				} `json:"submatches"`
			} `json:"data"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue // skip malformed lines
		}

		if msg.Type != "match" {
			continue
		}

		column := 0
		matchText := ""
		if len(msg.Data.Submatches) > 0 {
			column = msg.Data.Submatches[0].Start
			matchText = msg.Data.Submatches[0].Match.Text
		}

		matches = append(matches, SearchMatch{
			File:      makeRelative(msg.Data.Path.Text, searchDir),
			Line:      msg.Data.LineNumber,
			Column:    column,
			MatchText: matchText,
			LineText:  strings.TrimSuffix(msg.Data.Lines.Text, "\n"),
		})
	}

	if err := scanner.Err(); err != nil {
		return matches, fmt.Errorf("scan rg output: %w", err)
	}

	return matches, nil
}

// makeRelative converts an absolute path to relative to baseDir.
func makeRelative(path, baseDir string) string {
	if rel, err := filepath.Rel(baseDir, path); err == nil {
		return rel
	}
	return path
}
