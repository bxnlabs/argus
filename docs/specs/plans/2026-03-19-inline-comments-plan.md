# Inline Comments Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add inline code commenting to the git panel's Compare view, with backend storage, staleness detection, and CLI agent retrieval.

**Architecture:** Backend comments package handles JSON file storage and snippet-based staleness detection. Three new API endpoints (GET/POST/DELETE) on the node serve comment data. The frontend adds clickable line numbers to UnifiedDiff with inline forms/cards, plus a summary bar for submission. A CLI command outputs submitted comments as markdown for agents.

**Tech Stack:** Go (backend + CLI, Cobra commands, stdlib HTTP), React + TypeScript + Tailwind CSS + shadcn/ui (frontend), TanStack React Query (data fetching)

**Spec:** `docs/specs/2026-03-18-inline-comments-design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/node/comments/comments.go` | Data types, branch encoding, file read/write, staleness detection |
| `internal/node/comments/comments_test.go` | Unit tests for encoding, staleness, file operations |
| `internal/node/api/comments.go` | HTTP handlers for GET/POST/DELETE `/api/git/comments` |
| `internal/node/api/comments_test.go` | Handler unit tests |
| `web/src/types/comments.ts` | TypeScript types for comment data |
| `web/src/data/comments/keys.ts` | React Query key factory for comments |
| `web/src/data/comments/queries.ts` | React Query hooks (useCommentsQuery, mutation hooks) |
| `web/src/data/comments/index.ts` | Barrel export |
| `web/src/components/GitPanel/CommentSummaryBar.tsx` | Summary bar with comment count, general comment, submit button |
| `web/src/components/DiffViewer/InlineCommentForm.tsx` | Textarea + Add/Cancel buttons for new comments |
| `web/src/components/DiffViewer/InlineCommentCard.tsx` | Rendered comment card with delete button |
| `cmd/argus/cli/comments.go` | `argus comments get` CLI command |
| `cmd/argus/cli/comments_test.go` | CLI unit tests for markdown formatting |

### Modified Files

| File | Change |
|------|--------|
| `internal/node/api/router.go:37-48` | Add comment route registrations |
| `web/src/components/DiffViewer/UnifiedDiff.tsx` | Clickable new-side line numbers, inline comment injection points |
| `web/src/components/GitPanel/CompareView.tsx` | Comment state management, summary bar integration |
| `web/src/types.ts` | Re-export comment types |
| `cmd/argus/main.go:55-60` | Register `cli.NewCommentsCmd()` |

---

## Chunk 1: Backend Data Layer

### Task 1: Branch Name Encoding

**Files:**
- Create: `internal/node/comments/comments.go`
- Test: `internal/node/comments/comments_test.go`

- [ ] **Step 1: Write failing tests for branch name encoding**

```go
// comments_test.go
package comments

import "testing"

func TestEncodeBranchName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "main", want: "main"},
		{name: "slash", input: "feat/auth-system", want: "feat_auth-system"},
		{name: "underscore preserved", input: "my_branch", want: "my__branch"},
		{name: "slash and underscore", input: "feat/my_branch", want: "feat_my__branch"},
		{name: "multiple slashes", input: "feat/sub/deep", want: "feat_sub_deep"},
		{name: "leading underscore", input: "_private", want: "__private"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := encodeBranchName(tt.input)
			if got != tt.want {
				t.Errorf("encodeBranchName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCommentsFilename(t *testing.T) {
	got := commentsFilename("feat/auth-system", "main")
	want := "feat_auth-system--main.json"
	if got != want {
		t.Errorf("commentsFilename() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/jeevb/.argus/projects/--Users--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--git-review-panel && go test ./internal/node/comments/...`
Expected: FAIL — package does not exist

- [ ] **Step 3: Implement branch encoding**

```go
// comments.go
package comments

import "strings"

// encodeBranchName encodes a branch name for safe use as a filename component.
// "/" is replaced with "_", and "_" is escaped as "__". This is reversible.
func encodeBranchName(branch string) string {
	// Escape underscores first, then replace slashes.
	s := strings.ReplaceAll(branch, "_", "__")
	s = strings.ReplaceAll(s, "/", "_")
	return s
}

// commentsFilename returns the JSON filename for a branch comparison.
func commentsFilename(branch, baseBranch string) string {
	return encodeBranchName(branch) + "--" + encodeBranchName(baseBranch) + ".json"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/comments/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/comments/comments.go internal/node/comments/comments_test.go
git commit -m "feat(comments): add branch name encoding for comment filenames"
```

### Task 2: Data Types and File I/O

**Files:**
- Modify: `internal/node/comments/comments.go`
- Test: `internal/node/comments/comments_test.go`

- [ ] **Step 1: Write failing tests for file read/write**

```go
func TestWriteAndReadCommentsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := &CommentsFile{
		Branch:     "feat/auth",
		BaseBranch: "main",
		Comments: []Comment{
			{
				ID:        "rc_123_abc",
				File:      "src/auth.ts",
				Line:      LineRange{From: 10, To: 12},
				Snippet:   "const x = 1;",
				Body:      "Change this",
				Submitted: false,
				CreatedAt: "2026-03-16T10:30:00Z",
			},
		},
	}

	if err := writeCommentsFile(path, data); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readCommentsFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if got.Branch != "feat/auth" {
		t.Errorf("branch = %q, want %q", got.Branch, "feat/auth")
	}
	if len(got.Comments) != 1 {
		t.Fatalf("comments len = %d, want 1", len(got.Comments))
	}
	if got.Comments[0].Body != "Change this" {
		t.Errorf("body = %q, want %q", got.Comments[0].Body, "Change this")
	}
}

func TestReadCommentsFile_NotExist(t *testing.T) {
	got, err := readCommentsFile("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %+v", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/comments/... -run TestWriteAndRead -v`
Expected: FAIL — types undefined

- [ ] **Step 3: Implement data types and file I/O**

Add to `comments.go`:

```go
import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LineRange represents a line range in the branch version of a file.
type LineRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// Comment represents an inline code comment on a diff.
type Comment struct {
	ID        string    `json:"id"`
	File      string    `json:"file"`
	Line      LineRange `json:"line"`
	Snippet   string    `json:"snippet"`
	Body      string    `json:"body"`
	Submitted bool      `json:"submitted"`
	CreatedAt string    `json:"createdAt"`
}

// GeneralComment is an optional cross-cutting comment not tied to a specific line.
type GeneralComment struct {
	Body      string `json:"body"`
	Submitted bool   `json:"submitted"`
	CreatedAt string `json:"createdAt"`
}

// CommentsFile is the top-level JSON structure persisted to disk.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/comments/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/comments/comments.go internal/node/comments/comments_test.go
git commit -m "feat(comments): add data types and atomic file I/O"
```

### Task 3: Staleness Detection

**Files:**
- Modify: `internal/node/comments/comments.go`
- Test: `internal/node/comments/comments_test.go`

- [ ] **Step 1: Write failing tests for staleness detection**

Note: These tests use `fmt`, `os`, `path/filepath`, and `strings` — add them to the test file's import block alongside `"testing"`.

```go
func TestDetectStaleness_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "src", "auth.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	// Snippet is on line 3 (0-indexed line 2)
	os.WriteFile(filePath, []byte("line1\nline2\nconst TOKEN = 1800;\nline4\n"), 0o644)

	comments := []Comment{
		{
			ID:        "rc_1",
			File:      "src/auth.ts",
			Line:      LineRange{From: 10, To: 10}, // old line, will be re-anchored
			Snippet:   "const TOKEN = 1800;",
			Body:      "Change to 3600",
			Submitted: true,
		},
	}

	result := detectStaleness(dir, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].Line.From != 3 || result[0].Line.To != 3 {
		t.Errorf("expected line 3, got %d-%d", result[0].Line.From, result[0].Line.To)
	}
}

func TestDetectStaleness_NoMatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "src", "auth.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	os.WriteFile(filePath, []byte("completely different content\n"), 0o644)

	comments := []Comment{
		{
			ID:      "rc_1",
			File:    "src/auth.ts",
			Line:    LineRange{From: 5, To: 5},
			Snippet: "const TOKEN = 1800;",
			Body:    "Change to 3600",
			Submitted: true,
		},
	}

	result := detectStaleness(dir, comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (pruned), got %d", len(result))
	}
}

func TestDetectStaleness_FileDeleted(t *testing.T) {
	dir := t.TempDir()
	// Do not create the file

	comments := []Comment{
		{
			ID:      "rc_1",
			File:    "src/deleted.ts",
			Line:    LineRange{From: 1, To: 1},
			Snippet: "anything",
			Body:    "Comment on deleted file",
			Submitted: true,
		},
	}

	result := detectStaleness(dir, comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (file deleted), got %d", len(result))
	}
}

func TestDetectStaleness_MultipleMatches_NearestWins(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "src", "util.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)

	// "return true;" appears on lines 5 and 20
	var lines []string
	for i := 1; i <= 25; i++ {
		if i == 5 || i == 20 {
			lines = append(lines, "return true;")
		} else {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	comments := []Comment{
		{
			ID:      "rc_1",
			File:    "src/util.ts",
			Line:    LineRange{From: 6, To: 6}, // closer to line 5
			Snippet: "return true;",
			Body:    "Should return false",
			Submitted: true,
		},
	}

	result := detectStaleness(dir, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].Line.From != 5 {
		t.Errorf("expected nearest match line 5, got %d", result[0].Line.From)
	}
}

func TestDetectStaleness_SkipsUnsubmitted(t *testing.T) {
	dir := t.TempDir()

	comments := []Comment{
		{
			ID:        "rc_1",
			File:      "src/nonexistent.ts",
			Line:      LineRange{From: 1, To: 1},
			Snippet:   "anything",
			Body:      "Draft comment",
			Submitted: false,
		},
	}

	// Draft comments are kept as-is (not validated for staleness)
	result := detectStaleness(dir, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (draft preserved), got %d", len(result))
	}
}

func TestDetectStaleness_PathTraversal(t *testing.T) {
	dir := t.TempDir()

	comments := []Comment{
		{
			ID:      "rc_1",
			File:    "../etc/passwd",
			Line:    LineRange{From: 1, To: 1},
			Snippet: "root:x:0:0",
			Body:    "Traversal attempt",
			Submitted: true,
		},
	}

	result := detectStaleness(dir, comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (path traversal rejected), got %d", len(result))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/comments/... -run TestDetectStaleness -v`
Expected: FAIL — `detectStaleness` undefined

- [ ] **Step 3: Implement staleness detection**

Add to `comments.go`:

```go
import (
	"fmt"
	"math"
	"path/filepath"
)

// ValidateFilePath checks that a relative file path stays within a directory.
// Rejects absolute paths, path traversal, and symlink escapes.
// This mirrors the logic in api.sanitizeFilePath but lives in a shared location
// so staleness detection can use it without importing the api package.
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

// detectStaleness runs the staleness detection algorithm on submitted comments.
// It validates file paths, checks if snippets still exist in the files, and
// re-anchors or prunes comments accordingly. Unsubmitted (draft) comments are
// passed through unchanged.
func detectStaleness(repoDir string, comments []Comment) []Comment {
	var result []Comment

	for _, c := range comments {
		if !c.Submitted {
			result = append(result, c)
			continue
		}

		// Validate file path stays within repo.
		if err := ValidateFilePath(repoDir, c.File); err != nil {
			continue
		}

		// Read file content.
		absPath := filepath.Clean(filepath.Join(repoDir, c.File))
		content, err := os.ReadFile(absPath)
		if err != nil {
			// File deleted or unreadable — prune.
			continue
		}

		fileText := string(content)
		lineNum := findSnippet(fileText, c.Snippet, c.Line.From)
		if lineNum == -1 {
			// Snippet not found — agent changed the code. Prune.
			continue
		}

		// Re-anchor to new line position.
		lineCount := strings.Count(c.Snippet, "\n")
		c.Line = LineRange{From: lineNum, To: lineNum + lineCount}
		result = append(result, c)
	}

	return result
}

// findSnippet searches for snippet as a substring in fileText and returns the
// 1-based line number of the best match. Returns -1 if no match found.
// When multiple matches exist, prefers the one nearest to priorLine.
// If no match is within 50 lines of priorLine, returns -1 (stale).
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

	if len(matchLines) == 1 {
		return matchLines[0]
	}

	// Multiple matches — prefer nearest to prior position.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/comments/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/comments/comments.go internal/node/comments/comments_test.go
git commit -m "feat(comments): implement snippet-based staleness detection"
```

### Task 4: Public API for Comments Package

**Files:**
- Modify: `internal/node/comments/comments.go`
- Test: `internal/node/comments/comments_test.go`

- [ ] **Step 1: Write failing tests for public Load/Save/Delete functions**

```go
func TestLoadSaveDelete(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := t.TempDir()

	// Create a file for staleness to check against
	os.MkdirAll(filepath.Join(repoDir, "src"), 0o755)
	os.WriteFile(
		filepath.Join(repoDir, "src", "auth.ts"),
		[]byte("line1\nconst TOKEN = 1800;\nline3\n"),
		0o644,
	)

	cf := &CommentsFile{
		Branch:     "feat/auth",
		BaseBranch: "main",
		Comments: []Comment{
			{
				ID:        "rc_1",
				File:      "src/auth.ts",
				Line:      LineRange{From: 2, To: 2},
				Snippet:   "const TOKEN = 1800;",
				Body:      "Change to 3600",
				Submitted: true,
			},
		},
	}

	// Save
	if err := Save(projectDir, cf); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load (triggers staleness detection)
	loaded, err := Load(projectDir, repoDir, "feat/auth", "main")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded data")
	}
	if len(loaded.Comments) != 1 {
		t.Fatalf("expected 1 comment after staleness, got %d", len(loaded.Comments))
	}

	// Delete
	if err := Delete(projectDir, "feat/auth", "main"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Load after delete should return nil
	loaded, err = Load(projectDir, repoDir, "feat/auth", "main")
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after delete, got %+v", loaded)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/comments/... -run TestLoadSaveDelete -v`
Expected: FAIL — `Load`, `Save`, `Delete` undefined

- [ ] **Step 3: Implement public API**

Add to `comments.go`:

```go
// commentsDir returns the path to the comments directory for a project.
func commentsDir(projectDir string) string {
	return filepath.Join(projectDir, "comments")
}

// commentsPath returns the full path to the comments file for a branch comparison.
func commentsPath(projectDir, branch, baseBranch string) string {
	return filepath.Join(commentsDir(projectDir), commentsFilename(branch, baseBranch))
}

// Load reads the comments file for a branch comparison, runs staleness detection
// against the repo, and writes the pruned result back. Returns nil if no file exists.
func Load(projectDir, repoDir, branch, baseBranch string) (*CommentsFile, error) {
	path := commentsPath(projectDir, branch, baseBranch)
	cf, err := readCommentsFile(path)
	if err != nil {
		return nil, err
	}
	if cf == nil {
		return nil, nil
	}

	// Run staleness detection on submitted comments.
	cf.Comments = detectStaleness(repoDir, cf.Comments)

	// Write the pruned result back.
	if err := writeCommentsFile(path, cf); err != nil {
		return nil, err
	}

	return cf, nil
}

// Save writes the comments file for the branch comparison specified in the data.
func Save(projectDir string, cf *CommentsFile) error {
	path := commentsPath(projectDir, cf.Branch, cf.BaseBranch)
	return writeCommentsFile(path, cf)
}

// Delete removes the comments file for a branch comparison.
func Delete(projectDir, branch, baseBranch string) error {
	path := commentsPath(projectDir, branch, baseBranch)
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/comments/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/comments/comments.go internal/node/comments/comments_test.go
git commit -m "feat(comments): add public Load/Save/Delete API with staleness on read"
```

---

## Chunk 2: Backend API Handlers

### Task 5: Comment API Handlers

**Files:**
- Create: `internal/node/api/comments.go`
- Create: `internal/node/api/comments_test.go`
- Modify: `internal/node/api/router.go`

- [ ] **Step 1: Write failing tests for comment handlers**

Note: The GET handler accepts `branch` as an explicit query param (the frontend knows the current branch from `useGitStatusQuery`). This avoids the handler needing to call `git.GetStatus()` and makes testing straightforward without requiring a real git repo. The handler derives `projectDir` server-side from the `path` parameter using `source.ParentKeyFromPath()`.

Create `internal/node/api/comments_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/node/comments"
	"github.com/bxnlabs/argus/internal/source"
)

// testProjectDir returns the projectDir that the handler will compute for a given repo path.
func testProjectDir(t *testing.T, repoDir string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}
	return filepath.Join(home, ".argus", "projects", source.ParentKeyFromPath(repoDir))
}

func TestCommentsHandler_GetEmpty(t *testing.T) {
	repoDir := t.TempDir()

	// Override projectDir to a temp location for testing
	h := &commentsHandler{projectDirOverride: t.TempDir()}
	req := httptest.NewRequest("GET",
		"/api/git/comments?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	w := httptest.NewRecorder()

	h.get(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp comments.CommentsFile
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Comments) != 0 {
		t.Errorf("expected empty comments, got %d", len(resp.Comments))
	}
}

func TestCommentsHandler_PostAndGet(t *testing.T) {
	repoDir := t.TempDir()
	overrideDir := t.TempDir()

	// Create a file for staleness to validate against
	os.MkdirAll(filepath.Join(repoDir, "src"), 0o755)
	os.WriteFile(filepath.Join(repoDir, "src", "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644)

	h := &commentsHandler{projectDirOverride: overrideDir}

	payload := comments.CommentsFile{
		Branch:     "feat/test",
		BaseBranch: "main",
		Comments: []comments.Comment{
			{
				ID:        "rc_1_abc",
				File:      "src/main.go",
				Line:      comments.LineRange{From: 1, To: 1},
				Snippet:   "package main",
				Body:      "Add copyright header",
				Submitted: true,
				CreatedAt: "2026-03-16T10:30:00Z",
			},
		},
	}

	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest("POST",
		"/api/git/comments?path="+repoDir,
		bytes.NewReader(body))
	postW := httptest.NewRecorder()
	h.post(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("POST expected 200, got %d: %s", postW.Code, postW.Body.String())
	}

	// GET should return the comment
	getReq := httptest.NewRequest("GET",
		"/api/git/comments?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	getW := httptest.NewRecorder()
	h.get(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d", getW.Code)
	}

	var resp comments.CommentsFile
	json.Unmarshal(getW.Body.Bytes(), &resp)
	if len(resp.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(resp.Comments))
	}
}

func TestCommentsHandler_PostRejectsTraversal(t *testing.T) {
	repoDir := t.TempDir()

	h := &commentsHandler{projectDirOverride: t.TempDir()}

	payload := comments.CommentsFile{
		Branch:     "feat/test",
		BaseBranch: "main",
		Comments: []comments.Comment{
			{
				ID:   "rc_1",
				File: "../etc/passwd",
				Body: "Traversal attempt",
			},
		},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST",
		"/api/git/comments?path="+repoDir,
		bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.post(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for path traversal, got %d", w.Code)
	}
}

func TestCommentsHandler_Delete(t *testing.T) {
	repoDir := t.TempDir()
	overrideDir := t.TempDir()

	h := &commentsHandler{projectDirOverride: overrideDir}

	// Save a comment first
	payload := comments.CommentsFile{
		Branch:     "feat/test",
		BaseBranch: "main",
		Comments:   []comments.Comment{{ID: "rc_1", File: "x.go", Body: "test"}},
	}
	body, _ := json.Marshal(payload)
	postReq := httptest.NewRequest("POST",
		"/api/git/comments?path="+repoDir,
		bytes.NewReader(body))
	postW := httptest.NewRecorder()
	h.post(postW, postReq)

	// Delete
	delReq := httptest.NewRequest("DELETE",
		"/api/git/comments?path="+repoDir+"&branch=feat/test&base=main",
		nil)
	delW := httptest.NewRecorder()
	h.delete(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Errorf("DELETE expected 200, got %d", delW.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/api/... -run TestCommentsHandler -v`
Expected: FAIL — `commentsHandler` undefined

- [ ] **Step 3: Implement comment handlers**

Create `internal/node/api/comments.go`:

```go
package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/node/comments"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
)

type commentsHandler struct {
	projectDirOverride string // for testing — bypasses home dir derivation
}

// resolveProjectDir computes the project directory from the repo path.
// Uses the override if set (for testing).
func (h *commentsHandler) resolveProjectDir(expandedPath string) (string, error) {
	if h.projectDirOverride != "" {
		return h.projectDirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	parentKey := source.ParentKeyFromPath(expandedPath)
	return filepath.Join(home, ".argus", "projects", parentKey), nil
}

// GET /api/git/comments?path=...&branch=...&base=...
func (h *commentsHandler) get(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")
	if repoPath == "" || branch == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path, branch, and base parameters are required")
		return
	}

	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	cf, err := comments.Load(projectDir, expandedPath, branch, base)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	if cf == nil {
		cf = &comments.CommentsFile{
			Branch:     branch,
			BaseBranch: base,
			Comments:   []comments.Comment{},
		}
	}

	respondJSON(w, http.StatusOK, cf)
}

// POST /api/git/comments?path=...
func (h *commentsHandler) post(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	if repoPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	var cf comments.CommentsFile
	if err := parseBody(w, r, &cf); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate all file paths in comments
	for _, c := range cf.Comments {
		if _, err := sanitizeFilePath(expandedPath, c.File); err != nil {
			respondError(w, http.StatusBadRequest, "invalid file path in comment: "+c.File)
			return
		}
	}

	if err := comments.Save(projectDir, &cf); err != nil {
		respondInternalError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/git/comments?path=...&branch=...&base=...
func (h *commentsHandler) delete(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")

	if repoPath == "" || branch == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path, branch, and base parameters are required")
		return
	}

	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	if err := comments.Delete(projectDir, branch, base); err != nil {
		respondInternalError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/node/api/... -run TestCommentsHandler -v`
Expected: PASS

- [ ] **Step 5: Register routes in router**

Modify `internal/node/api/router.go` — add after the existing git routes block (after line 48):

```go
	// Comment routes
	ch := &commentsHandler{}
	mux.HandleFunc("GET /api/git/comments", ch.get)
	mux.HandleFunc("POST /api/git/comments", ch.post)
	mux.HandleFunc("DELETE /api/git/comments", ch.delete)
```

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/node/... -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/node/api/comments.go internal/node/api/comments_test.go internal/node/api/router.go
git commit -m "feat(comments): add GET/POST/DELETE API endpoints for inline comments"
```

---

## Chunk 3: Frontend Data Layer

### Task 6: TypeScript Types

**Files:**
- Create: `web/src/types/comments.ts`
- Modify: `web/src/types.ts`

- [ ] **Step 1: Create comment types**

Create `web/src/types/comments.ts`:

```typescript
export interface LineRange {
  from: number;
  to: number;
}

export interface InlineComment {
  id: string;
  file: string;
  line: LineRange;
  snippet: string;
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface GeneralComment {
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface CommentsFile {
  branch: string;
  baseBranch: string;
  comments: InlineComment[];
  generalComment?: GeneralComment;
}
```

- [ ] **Step 2: Re-export from types.ts**

Add to the end of `web/src/types.ts`:

```typescript
export type {
  LineRange,
  InlineComment,
  GeneralComment,
  CommentsFile,
} from "./types/comments";
```

- [ ] **Step 3: Commit**

```bash
git add web/src/types/comments.ts web/src/types.ts
git commit -m "feat(comments): add TypeScript types for inline comments"
```

### Task 7: React Query Hooks

**Files:**
- Create: `web/src/data/comments/keys.ts`
- Create: `web/src/data/comments/queries.ts`
- Create: `web/src/data/comments/index.ts`

- [ ] **Step 1: Create query key factory**

Create `web/src/data/comments/keys.ts`:

```typescript
export const commentKeys = {
  all: ["comments"] as const,
  forComparison: (path: string, base: string) =>
    [...commentKeys.all, path, base] as const,
};
```

- [ ] **Step 2: Create React Query hooks**

Create `web/src/data/comments/queries.ts`:

```typescript
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import type { CommentsFile } from "@/types";
import { commentKeys } from "./keys";

export function useCommentsQuery(
  path: string,
  branch: string | undefined,
  baseBranch: string | null,
) {
  return useQuery({
    queryKey: commentKeys.forComparison(path, baseBranch ?? ""),
    queryFn: async () => {
      const params = new URLSearchParams({
        path,
        branch: branch!,
        base: baseBranch!,
      });
      return apiFetch<CommentsFile>(`/node/api/git/comments?${params}`);
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!branch && !!baseBranch,
  });
}

export function useSaveCommentsMutation(path: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: CommentsFile) => {
      const params = new URLSearchParams({ path });
      return apiFetch<{ status: string }>(`/node/api/git/comments?${params}`, {
        method: "POST",
        body: JSON.stringify(data),
      });
    },
    onSuccess: (_data, variables) => {
      queryClient.setQueryData(
        commentKeys.forComparison(path, variables.baseBranch),
        variables,
      );
    },
  });
}

export function useDeleteCommentsMutation(path: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({
      branch,
      baseBranch,
    }: {
      branch: string;
      baseBranch: string;
    }) => {
      const params = new URLSearchParams({
        path,
        branch,
        base: baseBranch,
      });
      return apiFetch<{ status: string }>(
        `/node/api/git/comments?${params}`,
        { method: "DELETE" },
      );
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: commentKeys.forComparison(path, variables.baseBranch),
      });
    },
  });
}
```

- [ ] **Step 3: Create barrel export**

Create `web/src/data/comments/index.ts`:

```typescript
export { commentKeys } from "./keys";
export {
  useCommentsQuery,
  useSaveCommentsMutation,
  useDeleteCommentsMutation,
} from "./queries";
```

- [ ] **Step 4: Commit**

```bash
git add web/src/data/comments/
git commit -m "feat(comments): add React Query hooks for comments API"
```

---

## Chunk 4: Frontend Inline Comments in UnifiedDiff

### Task 8: InlineCommentForm Component

**Files:**
- Create: `web/src/components/DiffViewer/InlineCommentForm.tsx`

- [ ] **Step 1: Create the inline comment form**

```tsx
import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";

interface InlineCommentFormProps {
  onSubmit: (body: string) => void;
  onCancel: () => void;
}

export function InlineCommentForm({ onSubmit, onCancel }: InlineCommentFormProps) {
  const [body, setBody] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    textareaRef.current?.focus();
  }, []);

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      if (body.trim()) onSubmit(body.trim());
    }
    if (e.key === "Escape") {
      e.preventDefault();
      onCancel();
    }
  };

  return (
    <div className="border-border bg-muted/30 border-t p-3">
      <textarea
        ref={textareaRef}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Leave a comment..."
        rows={3}
        className="bg-background border-border w-full resize-y rounded border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500"
      />
      <div className="mt-2 flex items-center gap-2">
        <Button
          size="sm"
          onClick={() => body.trim() && onSubmit(body.trim())}
          disabled={!body.trim()}
        >
          Add comment
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
        <span className="text-muted-foreground ml-auto text-xs">
          {/Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘" : "Ctrl"}+Enter to add
        </span>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/DiffViewer/InlineCommentForm.tsx
git commit -m "feat(comments): add InlineCommentForm component"
```

### Task 9: InlineCommentCard Component

**Files:**
- Create: `web/src/components/DiffViewer/InlineCommentCard.tsx`

- [ ] **Step 1: Create the inline comment card**

```tsx
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { InlineComment } from "@/types";

interface InlineCommentCardProps {
  comment: InlineComment;
  onDelete: (id: string) => void;
}

export function InlineCommentCard({ comment, onDelete }: InlineCommentCardProps) {
  return (
    <div
      className={cn(
        "border-border bg-muted/20 border-t px-3 py-2",
        comment.submitted && "border-l-2 border-l-blue-500/50",
      )}
    >
      <div className="flex items-start gap-2">
        <p className="flex-1 whitespace-pre-wrap text-sm">{comment.body}</p>
        <Button
          size="icon-sm"
          variant="ghost"
          onClick={() => onDelete(comment.id)}
          aria-label="Delete comment"
          className="text-muted-foreground hover:text-foreground flex-shrink-0"
        >
          <X className="h-3.5 w-3.5" />
        </Button>
      </div>
      {comment.submitted && (
        <span className="text-muted-foreground mt-1 block text-xs">Submitted</span>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/DiffViewer/InlineCommentCard.tsx
git commit -m "feat(comments): add InlineCommentCard component"
```

### Task 10: Clickable Line Numbers and Comment Injection in UnifiedDiff

**Files:**
- Modify: `web/src/components/DiffViewer/UnifiedDiff.tsx`

This is the largest frontend change. The `UnifiedDiff` component gains:
1. New props for comment state callbacks
2. Clickable new-side line numbers (additions + context lines)
3. Shift-click multi-line selection
4. Inline form/card rows injected after commented lines

- [ ] **Step 1: Update UnifiedDiff props interface**

Add new props to the component:

```typescript
import type { InlineComment } from "@/types";
import { InlineCommentForm } from "./InlineCommentForm";
import { InlineCommentCard } from "./InlineCommentCard";

interface UnifiedDiffProps {
  diff: ParsedDiff;
  fileName: string;
  expanded?: boolean;
  onToggle?: () => void;
  // Comment props (optional — when absent, commenting is disabled)
  comments?: InlineComment[];
  activeCommentLine?: { from: number; to: number } | null;
  onLineClick?: (line: number, shiftKey: boolean) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
}
```

- [ ] **Step 2: Make new-side line number cells clickable**

In `DiffLineRow`, update the new-side line number `<td>` to be clickable when `onLineClick` is provided. Add cursor pointer and hover background on new-side numbers for non-deletion lines.

The `DiffLineRow` component needs to accept these new props:

```typescript
function DiffLineRow({
  line,
  isInActiveRange,
  onLineClick,
  commentingEnabled,
}: {
  line: DiffLine;
  isInActiveRange: boolean;
  onLineClick?: (line: number, shiftKey: boolean) => void;
  commentingEnabled: boolean;
}) {
  // ... existing rendering logic ...

  const isCommentable = commentingEnabled && line.type !== "deletion" && line.newLineNumber != null;

  return (
    <tr className={cn("hover:bg-muted/30", bgColor, isInActiveRange && "bg-blue-500/10")}>
      <td className="text-muted-foreground border-border/50 w-12 border-r px-2 py-0.5 text-right tabular-nums select-none">
        {line.oldLineNumber ?? ""}
      </td>
      <td
        className={cn(
          "text-muted-foreground border-border/50 w-12 border-r px-2 py-0.5 text-right tabular-nums select-none",
          isCommentable && "cursor-pointer hover:bg-blue-500/20 hover:text-blue-400",
        )}
        onClick={
          isCommentable
            ? (e) => onLineClick?.(line.newLineNumber!, e.shiftKey)
            : undefined
        }
      >
        {line.newLineNumber ?? ""}
      </td>
      {/* ... rest unchanged ... */}
    </tr>
  );
}
```

- [ ] **Step 3: Inject comment form and card rows after relevant lines**

In the `Hunk` component, after each `DiffLineRow`, check if:
1. The current new-side line number matches the end of the `activeCommentLine` range → render `InlineCommentForm`
2. There are comments anchored to this line → render `InlineCommentCard` for each

```typescript
function Hunk({
  hunk,
  comments,
  activeCommentLine,
  onLineClick,
  onAddComment,
  onCancelComment,
  onDeleteComment,
  commentingEnabled,
}: {
  hunk: DiffHunk;
  comments: InlineComment[];
  activeCommentLine: { from: number; to: number } | null;
  onLineClick?: (line: number, shiftKey: boolean) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  commentingEnabled: boolean;
}) {
  return (
    <div className="min-w-full">
      <div className="border-border border-y bg-blue-500/10 px-3 py-1 text-xs text-blue-400">
        {hunk.header}
      </div>
      <table className="min-w-full border-collapse">
        <tbody>
          {hunk.lines.map((line, index) => {
            const newLine = line.newLineNumber;
            const isInActiveRange =
              activeCommentLine != null &&
              newLine != null &&
              newLine >= activeCommentLine.from &&
              newLine <= activeCommentLine.to;

            // Comments anchored to this line (show after the line's to-range)
            const lineComments = newLine != null
              ? comments.filter((c) => c.line.to === newLine)
              : [];

            // Show form after the last line in the active range
            const showForm =
              activeCommentLine != null &&
              newLine === activeCommentLine.to;

            return (
              <Fragment key={index}>
                <DiffLineRow
                  line={line}
                  isInActiveRange={isInActiveRange}
                  onLineClick={onLineClick}
                  commentingEnabled={commentingEnabled}
                />
                {lineComments.map((c) => (
                  <tr key={c.id}>
                    <td colSpan={4}>
                      <InlineCommentCard comment={c} onDelete={onDeleteComment!} />
                    </td>
                  </tr>
                ))}
                {showForm && onAddComment && onCancelComment && (
                  <tr>
                    <td colSpan={4}>
                      <InlineCommentForm
                        onSubmit={onAddComment}
                        onCancel={onCancelComment}
                      />
                    </td>
                  </tr>
                )}
              </Fragment>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
```

- [ ] **Step 4: Update the UnifiedDiff component to pass comment props through**

Wire the new props through from `UnifiedDiff` → `Hunk` → `DiffLineRow`.

- [ ] **Step 5: Verify the component compiles**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 6: Commit**

```bash
git add web/src/components/DiffViewer/UnifiedDiff.tsx
git commit -m "feat(comments): add clickable line numbers and inline comment injection to UnifiedDiff"
```

---

## Chunk 5: Frontend Comment State and Summary Bar

### Task 11: CommentSummaryBar Component

**Files:**
- Create: `web/src/components/GitPanel/CommentSummaryBar.tsx`

- [ ] **Step 1: Create the summary bar**

```tsx
import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { ChevronDown, ChevronRight, MessageSquare } from "lucide-react";
import { cn } from "@/lib/utils";

interface CommentSummaryBarProps {
  pendingCount: number;
  generalComment: string;
  generalCommentSubmitted: boolean;
  onGeneralCommentChange: (body: string) => void;
  onSubmit: () => void;
  hasUnsubmitted: boolean;
}

export function CommentSummaryBar({
  pendingCount,
  generalComment,
  generalCommentSubmitted,
  onGeneralCommentChange,
  onSubmit,
  hasUnsubmitted,
}: CommentSummaryBarProps) {
  const [expanded, setExpanded] = useState(false);
  // Local state for the textarea — saves on blur, not on every keystroke
  const [localGeneralComment, setLocalGeneralComment] = useState(generalComment);

  // Sync from prop when it changes externally (e.g., after submit)
  useEffect(() => {
    setLocalGeneralComment(generalComment);
  }, [generalComment]);

  return (
    <div className="border-border bg-muted/30 border-t">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          onClick={() => setExpanded(!expanded)}
          className="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs"
        >
          {expanded ? (
            <ChevronDown className="h-3.5 w-3.5" />
          ) : (
            <ChevronRight className="h-3.5 w-3.5" />
          )}
          <MessageSquare className="h-3.5 w-3.5" />
          General feedback
        </button>

        {pendingCount > 0 && (
          <span className="text-muted-foreground text-xs">
            {pendingCount} pending comment{pendingCount !== 1 ? "s" : ""}
          </span>
        )}

        <div className="ml-auto">
          <Button
            size="sm"
            onClick={onSubmit}
            disabled={!hasUnsubmitted}
          >
            Submit comments
          </Button>
        </div>
      </div>

      {expanded && (
        <div className="px-3 pb-3">
          <textarea
            value={localGeneralComment}
            onChange={(e) => setLocalGeneralComment(e.target.value)}
            onBlur={() => {
              if (localGeneralComment !== generalComment) {
                onGeneralCommentChange(localGeneralComment);
              }
            }}
            placeholder="General feedback..."
            rows={3}
            className={cn(
              "bg-background border-border w-full resize-y rounded border px-3 py-2 text-sm focus:outline-none focus:ring-1 focus:ring-blue-500",
              generalCommentSubmitted && "border-blue-500/30",
            )}
          />
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/components/GitPanel/CommentSummaryBar.tsx
git commit -m "feat(comments): add CommentSummaryBar component"
```

### Task 12: Integrate Comments into CompareView

**Files:**
- Modify: `web/src/components/GitPanel/CompareView.tsx`

This task integrates everything together. The CompareView needs to:
1. Fetch comments via `useCommentsQuery`
2. Manage active comment form state (which line, which file)
3. Handle add/delete/submit flows via `useSaveCommentsMutation`
4. Pass comment props to each `UnifiedDiff`
5. Render `CommentSummaryBar`

- [ ] **Step 1: Add comment state management to CompareView**

Add imports and state:

```typescript
import { useCommentsQuery, useSaveCommentsMutation } from "@/data/comments";
import { CommentSummaryBar } from "./CommentSummaryBar";
import type { InlineComment, CommentsFile } from "@/types";

// Inside CompareView, add state:
const [activeComment, setActiveComment] = useState<{
  file: string;
  from: number;
  to: number;
} | null>(null);

```

No new props needed — `projectDir` is derived server-side from the `path` parameter.

- [ ] **Step 2: Wire up comment queries and mutations**

```typescript
const {
  data: commentsData,
} = useCommentsQuery(workingDirectory, currentBranch, baseBranch);

const saveComments = useSaveCommentsMutation(workingDirectory);
```

- [ ] **Step 3: Implement comment action handlers**

```typescript
const handleLineClick = useCallback((file: string, line: number, shiftKey: boolean) => {
  if (shiftKey && activeComment && activeComment.file === file) {
    // Extend range
    const from = Math.min(activeComment.from, line);
    const to = Math.max(activeComment.to, line);
    setActiveComment({ file, from, to });
  } else {
    setActiveComment({ file, from: line, to: line });
  }
}, [activeComment]);

const handleAddComment = useCallback((body: string) => {
  if (!activeComment || !commentsData || !currentBranch || !baseBranch) return;

  // Extract snippet from diff lines
  const diff = parsedDiffs.find((d) => getDiffPathKey(d) === activeComment.file);
  let snippet = "";
  if (diff) {
    const snippetLines: string[] = [];
    for (const hunk of diff.hunks) {
      for (const line of hunk.lines) {
        if (
          line.newLineNumber != null &&
          line.newLineNumber >= activeComment.from &&
          line.newLineNumber <= activeComment.to
        ) {
          snippetLines.push(line.content);
        }
      }
    }
    snippet = snippetLines.join("\n");
  }

  const newComment: InlineComment = {
    id: `rc_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
    file: activeComment.file,
    line: { from: activeComment.from, to: activeComment.to },
    snippet,
    body,
    submitted: false,
    createdAt: new Date().toISOString(),
  };

  const updated: CommentsFile = {
    ...commentsData,
    branch: currentBranch,
    baseBranch,
    comments: [...commentsData.comments, newComment],
  };

  saveComments.mutate(updated);
  setActiveComment(null);
}, [activeComment, commentsData, currentBranch, baseBranch, parsedDiffs, saveComments]);

const handleDeleteComment = useCallback((id: string) => {
  if (!commentsData || !currentBranch || !baseBranch) return;

  const updated: CommentsFile = {
    ...commentsData,
    comments: commentsData.comments.filter((c) => c.id !== id),
  };

  saveComments.mutate(updated);
}, [commentsData, currentBranch, baseBranch, saveComments]);

const handleSubmitComments = useCallback(() => {
  if (!commentsData || !currentBranch || !baseBranch) return;

  const updated: CommentsFile = {
    ...commentsData,
    comments: commentsData.comments.map((c) => ({ ...c, submitted: true })),
    generalComment: commentsData.generalComment
      ? { ...commentsData.generalComment, submitted: true }
      : undefined,
  };

  saveComments.mutate(updated);
}, [commentsData, currentBranch, baseBranch, saveComments]);

const handleGeneralCommentChange = useCallback((body: string) => {
  if (!commentsData || !currentBranch || !baseBranch) return;

  const updated: CommentsFile = {
    ...commentsData,
    generalComment: {
      body,
      submitted: false,
      createdAt: commentsData.generalComment?.createdAt ?? new Date().toISOString(),
    },
  };

  saveComments.mutate(updated);
}, [commentsData, currentBranch, baseBranch, saveComments]);
```

- [ ] **Step 4: Pass comment props to each UnifiedDiff**

Update the diff rendering in the diff pane to pass comment-related props:

```typescript
{parsedDiffs.map((diff) => {
  const pathKey = getDiffPathKey(diff);
  const fileName = getDiffFileName(diff);
  const fileComments = commentsData?.comments.filter((c) => c.file === pathKey) ?? [];
  const activeForFile = activeComment?.file === pathKey ? activeComment : null;

  return (
    <div key={pathKey} ref={setDiffRef(pathKey)}>
      <UnifiedDiff
        diff={diff}
        fileName={fileName}
        expanded
        comments={fileComments}
        activeCommentLine={activeForFile ? { from: activeForFile.from, to: activeForFile.to } : null}
        onLineClick={(line, shiftKey) => handleLineClick(pathKey, line, shiftKey)}
        onAddComment={handleAddComment}
        onCancelComment={() => setActiveComment(null)}
        onDeleteComment={handleDeleteComment}
      />
    </div>
  );
})}
```

- [ ] **Step 5: Add CommentSummaryBar to the layout**

Add the summary bar at the bottom of the diff pane (both desktop and mobile layouts), after the scrollable diff content:

```typescript
const pendingCount = commentsData?.comments.filter((c) => !c.submitted).length ?? 0;
const hasUnsubmitted =
  pendingCount > 0 ||
  (!!commentsData?.generalComment?.body && !commentsData.generalComment.submitted);

// Add after the diff pane content, inside the right pane:
{baseBranch && (
  <CommentSummaryBar
    pendingCount={pendingCount}
    generalComment={commentsData?.generalComment?.body ?? ""}
    generalCommentSubmitted={commentsData?.generalComment?.submitted ?? false}
    onGeneralCommentChange={handleGeneralCommentChange}
    onSubmit={handleSubmitComments}
    hasUnsubmitted={hasUnsubmitted}
  />
)}
```

- [ ] **Step 6: Verify compilation**

Run: `cd web && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 7: Commit**

```bash
git add web/src/components/GitPanel/CompareView.tsx
git commit -m "feat(comments): integrate inline comments and summary bar into CompareView"
```

---

## Chunk 6: CLI Command

### Task 13: `argus comments get` CLI Command

**Files:**
- Create: `cmd/argus/cli/comments.go`
- Create: `cmd/argus/cli/comments_test.go`
- Modify: `cmd/argus/main.go`

- [ ] **Step 1: Write failing test for markdown formatting**

Create `cmd/argus/cli/comments_test.go`:

```go
package cli

import (
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/comments"
)

func TestFormatCommentsMarkdown(t *testing.T) {
	cf := &comments.CommentsFile{
		Branch:     "feat/auth-system",
		BaseBranch: "main",
		Comments: []comments.Comment{
			{
				ID:        "rc_1",
				File:      "src/auth.ts",
				Line:      comments.LineRange{From: 52, To: 52},
				Snippet:   "const TOKEN_EXPIRY = 1800;",
				Body:      "Token expiry should be 3600 not 1800",
				Submitted: true,
			},
			{
				ID:        "rc_2",
				File:      "src/auth.ts",
				Line:      comments.LineRange{From: 12, To: 15},
				Snippet:   "function validateToken(token) {\n  if (!token) return false;\n}",
				Body:      "Missing signature check",
				Submitted: false, // Should be excluded
			},
			{
				ID:        "rc_3",
				File:      "src/utils.ts",
				Line:      comments.LineRange{From: 1, To: 3},
				Snippet:   "import { hash } from 'crypto';",
				Body:      "Use a proper hashing library",
				Submitted: true,
			},
		},
		GeneralComment: &comments.GeneralComment{
			Body:      "Auth looks mostly good, but token handling needs hardening",
			Submitted: true,
		},
	}

	output := formatCommentsMarkdown(cf)

	// Check structure
	if !strings.Contains(output, "## Comments") {
		t.Error("missing ## Comments header")
	}
	if !strings.Contains(output, "Branch: feat/auth-system vs main") {
		t.Error("missing branch line")
	}
	if !strings.Contains(output, "### src/auth.ts") {
		t.Error("missing file header for src/auth.ts")
	}
	if !strings.Contains(output, "**Lines 52-52:**") {
		t.Error("missing line range")
	}
	if !strings.Contains(output, "> const TOKEN_EXPIRY = 1800;") {
		t.Error("missing quoted snippet")
	}
	if !strings.Contains(output, "Token expiry should be 3600") {
		t.Error("missing comment body")
	}

	// Draft comment should be excluded
	if strings.Contains(output, "Missing signature check") {
		t.Error("draft comment should not appear in output")
	}

	// General comment
	if !strings.Contains(output, "### General") {
		t.Error("missing general section")
	}
	if !strings.Contains(output, "token handling needs hardening") {
		t.Error("missing general comment body")
	}
}

func TestFormatCommentsMarkdown_Empty(t *testing.T) {
	cf := &comments.CommentsFile{
		Branch:     "feat/test",
		BaseBranch: "main",
		Comments:   []comments.Comment{},
	}

	output := formatCommentsMarkdown(cf)
	if !strings.Contains(output, "No submitted comments") {
		t.Error("expected empty message")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/argus/cli/... -run TestFormatComments -v`
Expected: FAIL — `formatCommentsMarkdown` undefined

- [ ] **Step 3: Implement the CLI command**

Create `cmd/argus/cli/comments.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bxnlabs/argus/internal/node/comments"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/spf13/cobra"
)

// NewCommentsCmd returns the "comments" parent command.
func NewCommentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Manage inline review comments",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(newCommentsGetCmd())
	return cmd
}

func newCommentsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get submitted inline comments for the current branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve current directory to project
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine working directory: %w", err)
			}

			resolved, err := source.Resolve(cwd)
			if err != nil {
				return fmt.Errorf("cannot resolve project: %w", err)
			}

			repoDir := resolved.LocalPath
			parentKey := resolved.ParentKey()

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			projectDir := filepath.Join(home, ".argus", "projects", parentKey)

			// Determine current branch and base branch via node API
			dp, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(dp)
			if err != nil {
				return err
			}

			// Get git status for current branch
			body, err := c.get(fmt.Sprintf("/api/git/status?path=%s", repoDir))
			if err != nil {
				return fmt.Errorf("get git status: %w", err)
			}

			var statusResp struct {
				Status struct {
					Branch string `json:"branch"`
				} `json:"status"`
			}
			if err := json.Unmarshal(body, &statusResp); err != nil {
				return fmt.Errorf("parse status: %w", err)
			}
			branch := statusResp.Status.Branch

			// Get default base branch
			body, err = c.get(fmt.Sprintf("/api/git/compare/branches?path=%s", repoDir))
			if err != nil {
				return fmt.Errorf("get branches: %w", err)
			}

			var branchResp struct {
				DefaultBase string `json:"defaultBase"`
			}
			if err := json.Unmarshal(body, &branchResp); err != nil {
				return fmt.Errorf("parse branches: %w", err)
			}
			baseBranch := branchResp.DefaultBase

			// Load comments (runs staleness detection)
			cf, err := comments.Load(projectDir, repoDir, branch, baseBranch)
			if err != nil {
				return fmt.Errorf("load comments: %w", err)
			}

			if cf == nil {
				cf = &comments.CommentsFile{
					Branch:     branch,
					BaseBranch: baseBranch,
				}
			}

			fmt.Print(formatCommentsMarkdown(cf))
			return nil
		},
	}
}

// formatCommentsMarkdown formats submitted comments as structured markdown.
func formatCommentsMarkdown(cf *comments.CommentsFile) string {
	var b strings.Builder

	// Filter to submitted only
	var submitted []comments.Comment
	for _, c := range cf.Comments {
		if c.Submitted {
			submitted = append(submitted, c)
		}
	}

	hasGeneral := cf.GeneralComment != nil && cf.GeneralComment.Submitted && cf.GeneralComment.Body != ""

	if len(submitted) == 0 && !hasGeneral {
		b.WriteString("No submitted comments.\n")
		return b.String()
	}

	b.WriteString("## Comments\n")
	b.WriteString(fmt.Sprintf("Branch: %s vs %s\n", cf.Branch, cf.BaseBranch))

	// Group by file
	byFile := make(map[string][]comments.Comment)
	var fileOrder []string
	for _, c := range submitted {
		if _, seen := byFile[c.File]; !seen {
			fileOrder = append(fileOrder, c.File)
		}
		byFile[c.File] = append(byFile[c.File], c)
	}
	sort.Strings(fileOrder)

	for _, file := range fileOrder {
		b.WriteString(fmt.Sprintf("\n### %s\n\n", file))
		for _, c := range byFile[file] {
			b.WriteString(fmt.Sprintf("**Lines %d-%d:**\n", c.Line.From, c.Line.To))
			// Quote snippet lines
			for _, line := range strings.Split(c.Snippet, "\n") {
				b.WriteString("> " + line + "\n")
			}
			b.WriteString("\n" + c.Body + "\n")
		}
	}

	if hasGeneral {
		b.WriteString("\n### General\n\n")
		b.WriteString(cf.GeneralComment.Body + "\n")
	}

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/argus/cli/... -run TestFormatComments -v`
Expected: PASS

- [ ] **Step 5: Register the command in main.go**

Modify `cmd/argus/main.go` — add `cli.NewCommentsCmd()` to the root command's `AddCommand` call (after line 59):

```go
	rootCmd.AddCommand(
		newServerCmd(),
		newNodeCmd(),
		cli.NewSessionCmd(),
		cli.NewInternalCmd(),
		cli.NewCommentsCmd(),
	)
```

- [ ] **Step 6: Build and verify**

Run: `go build ./cmd/argus/...`
Expected: Builds successfully

- [ ] **Step 7: Commit**

```bash
git add cmd/argus/cli/comments.go cmd/argus/cli/comments_test.go cmd/argus/main.go
git commit -m "feat(comments): add 'argus comments get' CLI command with markdown output"
```

---

## Chunk 7: Integration and Polish

Note: Task 14 was removed — the handler derives `projectDir` server-side from the start (see Task 5). No frontend refactoring needed.

### Task 15: End-to-End Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test ./... -v`
Expected: All tests pass

- [ ] **Step 2: Run frontend type check**

Run: `cd web && npx tsc --noEmit`
Expected: No errors

- [ ] **Step 3: Run frontend build**

Run: `cd web && npm run build`
Expected: Builds successfully

- [ ] **Step 4: Manual smoke test checklist**

1. Start Argus, open a session on a branch with changes
2. Open Git panel > Compare tab
3. Verify new-side line numbers show hover state (pointer cursor, highlight)
4. Click a new-side line number — inline form appears below that line
5. Type a comment, click "Add comment" — card appears inline
6. Click another line number — form moves to new location
7. Shift-click to select a range — range highlights, form at end of range
8. Delete a comment via X button — card disappears
9. Summary bar shows pending count
10. Expand general comment, type feedback
11. Click "Submit comments" — all comments marked submitted
12. Run `argus comments get` in the terminal — see structured markdown output
13. Make a new commit that changes a commented line
14. Reload Compare view — stale comment is pruned, surviving comments re-anchored

- [ ] **Step 5: Final commit with any fixes from manual testing**

```bash
git add -A
git commit -m "fix(comments): address issues found during manual testing"
```
