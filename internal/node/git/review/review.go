package review

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
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
type AnchorStatus string

const (
	AnchorStatusResolved           AnchorStatus = "resolved"
	AnchorStatusStale              AnchorStatus = "stale"
	AnchorStatusContextUnavailable AnchorStatus = "context_unavailable"
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
	if err := json.Unmarshal(data, &nf); err == nil && nf.From.Line != 0 {
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

// detectStaleness re-anchors submitted comments to their current line positions,
// pruning any that can no longer be found in the file.
// Unsubmitted (draft) comments are always preserved as-is.
func detectStaleness(repoDir string, comments []ReviewComment) []ReviewComment {
	result := make([]ReviewComment, 0)
	for _, c := range comments {
		if !c.Submitted {
			result = append(result, c)
			continue
		}
		if err := ValidateFilePath(repoDir, c.File); err != nil {
			continue
		}
		absPath := filepath.Clean(filepath.Join(repoDir, c.File))
		content, err := os.ReadFile(absPath)
		if err != nil {
			continue
		}
		fileText := string(content)
		lineNum := findSnippet(fileText, c.Snippet, c.Line.From.Line)
		if lineNum == -1 {
			continue
		}
		lineCount := strings.Count(c.Snippet, "\n")
		c.Line = LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: lineNum},
			To:   DiffPosition{Side: DiffSideRight, Line: lineNum + lineCount},
		}
		result = append(result, c)
	}
	return result
}

// findSnippet finds the line number (1-indexed) where snippet appears in fileText,
// preferring the occurrence nearest to priorLine. Returns -1 if not found or
// if the nearest match is more than 50 lines away.
func findSnippet(fileText, snippet string, priorLine int) int {
	if snippet == "" {
		return -1
	}
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
	// A single match is unambiguous — always re-anchor regardless of distance.
	// The distance threshold only disambiguates multiple matches.
	if len(matchLines) == 1 {
		return matchLines[0]
	}
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

// reviewsDir returns the directory where review files are stored.
func reviewsDir(projectDir string) string {
	return filepath.Join(projectDir, "reviews")
}

// reviewPath returns the full path to the review file for a branch pair.
func reviewPath(projectDir, head, base string) string {
	return filepath.Join(reviewsDir(projectDir), reviewFilename(head, base))
}

// Load reads the review file for the given branch pair, runs staleness detection,
// persists any re-anchoring, and returns the result. Returns nil, nil if no file exists.
func Load(projectDir, repoDir, head, base string) (*Review, error) {
	path := reviewPath(projectDir, head, base)
	r, err := readReviewFile(path)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	r.Comments = detectStaleness(repoDir, r.Comments)
	if err := writeReviewFile(path, r); err != nil {
		return nil, err
	}
	return r, nil
}

// Save writes the Review to disk.
func Save(projectDir string, r *Review) error {
	path := reviewPath(projectDir, r.Head, r.Base)
	return writeReviewFile(path, r)
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
