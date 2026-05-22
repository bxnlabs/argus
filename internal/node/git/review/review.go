package review

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// encodeBranchName encodes a branch name for use in a filename.
// Underscores are doubled, hyphens are escaped to "-_", and slashes become "_".
// This guarantees the separator "--" cannot appear inside any encoded segment,
// preventing collisions such as ("a--b","c") vs ("a","b--c").
func encodeBranchName(branch string) string {
	s := strings.ReplaceAll(branch, "_", "__")
	s = strings.ReplaceAll(s, "-", "-_")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func reviewFilename(head, base string) string {
	return encodeBranchName(head) + "--" + encodeBranchName(base) + ".json"
}

// DiffSide identifies which side of a unified diff a line belongs to.
type DiffSide string

const (
	DiffSideLeft  DiffSide = "L"
	DiffSideRight DiffSide = "R"
)

// AnchorStatus describes the staleness state of a submitted comment's anchor.
// "" means healthy (the snippet re-anchored cleanly).
type AnchorStatus string

const (
	// AnchorStale: the snippet was found but the match is ambiguous, or an
	// L-side deletion was restored on the head side. Rendered inline at the
	// best-guess line with a "may have moved" badge.
	AnchorStale AnchorStatus = "stale"
	// AnchorUnanchored: the comment could not be re-anchored at all — its file
	// or snippet is no longer present in the relevant ref. It has no honest
	// inline location, so the UI surfaces it in the unanchored section for
	// read/prune rather than rendering it inline on unrelated code.
	AnchorUnanchored AnchorStatus = "unanchored"
)

// DiffPosition identifies a specific line on a specific side of the diff.
type DiffPosition struct {
	Side DiffSide `json:"side"`
	Line int      `json:"line"`
}

// LineRange represents a range of lines in a diff (1-indexed, inclusive).
// It supports both the new format {"from":{"side":"R","line":5},"to":{"side":"R","line":5}}
// and the legacy format {"from":5,"to":5} (migrated to R side on read).
type LineRange struct {
	From DiffPosition `json:"from"`
	To   DiffPosition `json:"to"`
}

// UnmarshalJSON implements backward-compatible deserialization for LineRange.
// It accepts the new object format {"from":{"side":"R","line":N},"to":{"side":"R","line":N}}
// and the legacy numeric format {"from":N,"to":N}, migrating the latter to R side.
func (lr *LineRange) UnmarshalJSON(data []byte) error {
	// Try new format first.
	type newFormat struct {
		From DiffPosition `json:"from"`
		To   DiffPosition `json:"to"`
	}
	var nf newFormat
	if err := json.Unmarshal(data, &nf); err == nil && nf.From.Side != "" {
		lr.From = nf.From
		lr.To = nf.To
		return nil
	}

	// Fall back to legacy format {"from": N, "to": N}.
	var legacy struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	lr.From = DiffPosition{Side: DiffSideRight, Line: legacy.From}
	lr.To = DiffPosition{Side: DiffSideRight, Line: legacy.To}
	return nil
}

// ReviewComment is a review comment anchored to a snippet of code.
type ReviewComment struct {
	ID             string       `json:"id"`
	File           string       `json:"file"`
	OldPath        string       `json:"oldPath,omitempty"`
	Line           LineRange    `json:"line"`
	Snippet        string       `json:"snippet"`
	SnippetContext string       `json:"snippetContext,omitempty"`
	Body           string       `json:"body"`
	Submitted      bool         `json:"submitted"`
	CreatedAt      string       `json:"createdAt"`
	AnchorStatus   AnchorStatus `json:"anchorStatus,omitempty"`
}

// ReviewBody is the top-level review feedback (not anchored to a line).
type ReviewBody struct {
	Body      string `json:"body"`
	Submitted bool   `json:"submitted"`
	CreatedAt string `json:"createdAt"`
}

// Review holds all review comments and body for a head/base branch pair.
type Review struct {
	Head     string          `json:"head"`
	Base     string          `json:"base"`
	Body     *ReviewBody     `json:"body,omitempty"`
	Comments []ReviewComment `json:"comments"`
}

// readReviewFile reads and parses the review JSON file.
// Returns nil, nil if the file does not exist.
func readReviewFile(path string) (*Review, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r Review
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// writeReviewFile atomically writes the review JSON file.
func writeReviewFile(path string, r *Review) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".argus-review-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// ValidateFilePath checks that a relative file path stays within a directory.
func ValidateFilePath(dir, file string) error {
	if filepath.IsAbs(file) {
		return fmt.Errorf("file path escapes directory")
	}
	abs := filepath.Clean(filepath.Join(dir, file))
	cleanDir := filepath.Clean(dir)
	if !strings.HasPrefix(abs, cleanDir+string(filepath.Separator)) {
		return fmt.Errorf("file path escapes directory")
	}
	return nil
}

// getFileContent returns the content of filePath at the given git ref using
// "git show ref:filePath". Returns an error if the file does not exist in that ref.
func getFileContent(repoDir, ref, filePath string) (string, error) {
	cmd := exec.Command("git", "show", ref+":"+filePath)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// detectStaleness re-anchors submitted comments to their current line positions
// using immutable git refs rather than the working tree, and otherwise
// preserves them. Drafts are always preserved as-is.
//
// R-side comments are resolved against headRef; L-side comments against baseRef.
// L-side comments with a non-empty OldPath use OldPath for the file lookup.
// When the file is missing in the ref or the snippet cannot be located, the
// comment is preserved with AnchorStatus=AnchorUnanchored so the UI routes it
// to the read/prune section rather than rendering it inline on unrelated code.
//
// Comments with paths that escape the repo are dropped defensively (they
// cannot be rendered and the POST handler would reject them anyway).
//
// A comment that matches ambiguously and whose snippetContext cannot
// disambiguate is kept with AnchorStatus=AnchorStale.
func detectStaleness(repoDir, headRef, baseRef string, comments []ReviewComment) []ReviewComment {
	result := make([]ReviewComment, 0, len(comments))
	for _, c := range comments {
		if !c.Submitted {
			result = append(result, c)
			continue
		}

		side := c.Line.From.Side

		// Determine which ref and path to use.
		ref := headRef
		lookupPath := c.File
		if side == DiffSideLeft {
			ref = baseRef
			if c.OldPath != "" {
				lookupPath = c.OldPath
			}
		}

		if err := ValidateFilePath(repoDir, lookupPath); err != nil {
			// Defensive: drop comments whose path escapes the repo. Cannot be
			// rendered (no anchor file) and would 400 on save anyway.
			continue
		}

		// If the ref is empty the caller hasn't provided enough context to check
		// staleness; preserve the comment as-is.
		if ref == "" {
			result = append(result, c)
			continue
		}

		fileText, err := getFileContent(repoDir, ref, lookupPath)
		if err != nil {
			// File not present in this ref — preserve but mark unanchored so the
			// UI routes it to the read/prune section instead of rendering it
			// inline at a stale line.
			c.AnchorStatus = AnchorUnanchored
			result = append(result, c)
			continue
		}

		lineNum := findSnippetWithContext(fileText, c.Snippet, c.SnippetContext, c.Line.From.Line)
		switch lineNum {
		case -1:
			// Not found — preserve but mark unanchored so the UI routes it to the
			// read/prune section instead of rendering it inline on unrelated code.
			c.AnchorStatus = AnchorUnanchored
			result = append(result, c)
			continue
		case -2:
			// Ambiguous — mark stale and keep.
			c.AnchorStatus = AnchorStale
			result = append(result, c)
		default:
			lineCount := strings.Count(c.Snippet, "\n")
			c.Line = LineRange{
				From: DiffPosition{Side: side, Line: lineNum},
				To:   DiffPosition{Side: side, Line: lineNum + lineCount},
			}
			c.AnchorStatus = ""

			// For L-side comments, the snippet was deleted. If it has been
			// restored in the head ref the comment is stale.
			if side == DiffSideLeft && headRef != "" {
				if err := ValidateFilePath(repoDir, c.File); err == nil {
					if headText, err := getFileContent(repoDir, headRef, c.File); err == nil {
						if strings.Contains(headText, c.Snippet) {
							c.AnchorStatus = AnchorStale
						}
					}
				}
			}

			result = append(result, c)
		}
	}
	return result
}

// findSnippetWithContext finds the 1-indexed line number where snippet appears in fileText.
// It extends findSnippet by using snippetContext to disambiguate multiple matches.
//
// Return values:
//   - >=1   exact (or disambiguated) match line number
//   - -1    not found (or nearest > 50 lines away with no context)
//   - -2    ambiguous: multiple matches and snippetContext did not resolve them
func findSnippetWithContext(fileText, snippet, snippetContext string, priorLine int) int {
	if snippet == "" {
		return -1
	}

	// Collect all match line numbers.
	var matchLines []int
	startIdx := 0
	for {
		idx := strings.Index(fileText[startIdx:], snippet)
		if idx == -1 {
			break
		}
		absIdx := startIdx + idx
		line := strings.Count(fileText[:absIdx], "\n") + 1
		matchLines = append(matchLines, line)
		startIdx = absIdx + 1
	}

	if len(matchLines) == 0 {
		return -1
	}

	// Single match — always re-anchor regardless of distance (unambiguous).
	if len(matchLines) == 1 {
		return matchLines[0]
	}

	// Multiple matches: try to disambiguate using snippetContext.
	if snippetContext != "" {
		var contextMatches []int
		for _, line := range matchLines {
			if contextMatchesLine(fileText, line, snippetContext) {
				contextMatches = append(contextMatches, line)
			}
		}
		if len(contextMatches) == 1 {
			return contextMatches[0]
		}
		// Multiple context matches — pick nearest to prior line if it's
		// uniquely closest. This handles common snippets (e.g. "}") that
		// appear multiple times within the context window.
		if len(contextMatches) > 1 {
			best := contextMatches[0]
			bestDist := int(math.Abs(float64(best - priorLine)))
			tied := false
			for _, line := range contextMatches[1:] {
				dist := int(math.Abs(float64(line - priorLine)))
				if dist < bestDist {
					bestDist = dist
					best = line
					tied = false
				} else if dist == bestDist {
					tied = true
				}
			}
			if tied {
				return -2
			}
			return best
		}
		// Context provided but matched no candidates — ambiguous.
		return -2
	}

	// No context provided: fall back to nearest match with distance threshold.
	best := -1
	bestDist := math.MaxInt
	for _, line := range matchLines {
		dist := line - priorLine
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			best = line
		}
	}
	if bestDist > 50 {
		return -1
	}
	return best
}

// contextMatchesLine checks whether snippetContext appears in a window of lines
// surrounding the match at matchLine in fileText.
func contextMatchesLine(fileText string, matchLine int, snippetContext string) bool {
	lines := strings.Split(fileText, "\n")
	// Use a window of ±10 lines around the match.
	const windowSize = 10
	from := matchLine - windowSize - 1
	if from < 0 {
		from = 0
	}
	to := matchLine + windowSize
	if to > len(lines) {
		to = len(lines)
	}
	window := strings.Join(lines[from:to], "\n")
	return strings.Contains(window, snippetContext)
}

// findSnippet finds the line number (1-indexed) where snippet appears in fileText,
// preferring the occurrence nearest to priorLine. Returns -1 if not found or
// if the nearest match is more than 50 lines away.
//
// Deprecated: prefer findSnippetWithContext for new call sites.
func findSnippet(fileText, snippet string, priorLine int) int {
	n := findSnippetWithContext(fileText, snippet, "", priorLine)
	if n == -2 {
		return -1
	}
	return n
}

// reviewsDir returns the directory where review files are stored.
func reviewsDir(projectDir string) string {
	return filepath.Join(projectDir, "reviews")
}

// reviewPath returns the full path to the review file for a branch pair.
func reviewPath(projectDir, head, base string) string {
	return filepath.Join(reviewsDir(projectDir), reviewFilename(head, base))
}

// Load reads the review file for the given branch pair and returns it with
// submitted comments re-anchored against the current refs. The result is
// in-memory only — Load does not persist changes. Persistence is the write
// path's responsibility (Save, called via POST /api/git/review).
//
// headRef is the commit OID for the head side (R-side); baseRef is for the
// base side (L-side). Either may be empty, in which case comments on that
// side are skipped during staleness detection.
// Returns nil, nil if no file exists.
func Load(projectDir, repoDir, head, base, headRef, baseRef string) (*Review, error) {
	path := reviewPath(projectDir, head, base)
	r, err := readReviewFile(path)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	r.Comments = detectStaleness(repoDir, headRef, baseRef, r.Comments)
	return r, nil
}

// Save writes the Review to disk. AnchorStatus is a transient field derived at
// Load time by detectStaleness and must never be persisted; it is cleared on a
// copy here so this view-state cannot round-trip onto disk via the full-review
// POST (which echoes back the loaded Review verbatim).
func Save(projectDir string, r *Review) error {
	path := reviewPath(projectDir, r.Head, r.Base)
	out := *r
	out.Comments = make([]ReviewComment, len(r.Comments))
	for i, c := range r.Comments {
		c.AnchorStatus = ""
		out.Comments[i] = c
	}
	return writeReviewFile(path, &out)
}

// Delete removes the review file for the given branch pair.
// Returns nil if the file does not exist.
func Delete(projectDir, head, base string) error {
	path := reviewPath(projectDir, head, base)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
