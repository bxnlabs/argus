package comments

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// encodeBranchName encodes a branch name for use in a filename.
// Slashes are replaced with underscores; existing underscores are doubled.
func encodeBranchName(branch string) string {
	s := strings.ReplaceAll(branch, "_", "__")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

func commentsFilename(branch, baseBranch string) string {
	return encodeBranchName(branch) + "--" + encodeBranchName(baseBranch) + ".json"
}

// LineRange represents a range of lines in a file (1-indexed, inclusive).
type LineRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Comment is a review comment anchored to a snippet of code.
type Comment struct {
	ID        string    `json:"id"`
	File      string    `json:"file"`
	Line      LineRange `json:"line"`
	Snippet   string    `json:"snippet"`
	Body      string    `json:"body"`
	Submitted bool      `json:"submitted"`
	CreatedAt string    `json:"createdAt"`
}

// GeneralComment is a top-level (non-line) comment on a review.
type GeneralComment struct {
	Body      string `json:"body"`
	Submitted bool   `json:"submitted"`
	CreatedAt string `json:"createdAt"`
}

// CommentsFile holds all review comments for a branch/baseBranch pair.
type CommentsFile struct {
	Branch         string          `json:"branch"`
	BaseBranch     string          `json:"baseBranch"`
	Comments       []Comment       `json:"comments"`
	GeneralComment *GeneralComment `json:"generalComment,omitempty"`
}

// readCommentsFile reads and parses the comments JSON file.
// Returns nil, nil if the file does not exist.
func readCommentsFile(path string) (*CommentsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cf CommentsFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	return &cf, nil
}

// writeCommentsFile atomically writes the comments JSON file.
func writeCommentsFile(path string, cf *CommentsFile) error {
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".argus-comments-*")
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
func detectStaleness(repoDir string, comments []Comment) []Comment {
	var result []Comment
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
		lineNum := findSnippet(fileText, c.Snippet, c.Line.From)
		if lineNum == -1 {
			continue
		}
		lineCount := strings.Count(c.Snippet, "\n")
		c.Line = LineRange{From: lineNum, To: lineNum + lineCount}
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

// commentsDir returns the directory where comments files are stored.
func commentsDir(projectDir string) string {
	return filepath.Join(projectDir, "comments")
}

// commentsPath returns the full path to the comments file for a branch pair.
func commentsPath(projectDir, branch, baseBranch string) string {
	return filepath.Join(commentsDir(projectDir), commentsFilename(branch, baseBranch))
}

// Load reads the comments file for the given branch pair, runs staleness detection,
// persists any re-anchoring, and returns the result. Returns nil, nil if no file exists.
func Load(projectDir, repoDir, branch, baseBranch string) (*CommentsFile, error) {
	path := commentsPath(projectDir, branch, baseBranch)
	cf, err := readCommentsFile(path)
	if err != nil {
		return nil, err
	}
	if cf == nil {
		return nil, nil
	}
	cf.Comments = detectStaleness(repoDir, cf.Comments)
	if err := writeCommentsFile(path, cf); err != nil {
		return nil, err
	}
	return cf, nil
}

// Save writes the CommentsFile to disk.
func Save(projectDir string, cf *CommentsFile) error {
	path := commentsPath(projectDir, cf.Branch, cf.BaseBranch)
	return writeCommentsFile(path, cf)
}

// Delete removes the comments file for the given branch pair.
// Returns nil if the file does not exist.
func Delete(projectDir, branch, baseBranch string) error {
	path := commentsPath(projectDir, branch, baseBranch)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
