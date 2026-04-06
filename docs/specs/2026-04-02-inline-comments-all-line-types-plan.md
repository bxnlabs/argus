# BXN-67: Inline Comments on All Line Types — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Enable inline comments on deletion, context, and expanded context lines in the unified diff viewer, using a side-aware anchor model (`L`/`R`).

**Architecture:** Replace the flat `LineRange { from: number, to: number }` with `DiffCommentRange { from: DiffPosition, to: DiffPosition }` where `DiffPosition = { side: "L" | "R", line: number }`. Backend staleness detection becomes side-aware against immutable commit OIDs. Frontend comment indexing switches from `Map<number>` to `Map<string>` keyed by anchor strings like `"L43"` or `"R12"`.

**Tech Stack:** Go (backend types, staleness, API), TypeScript/React (frontend types, diff viewer, compare view), React Query (data layer)

**Spec:** `docs/specs/2026-04-02-inline-comments-all-line-types-design.md`

---

### Task 1: Backend Data Model — Side-Aware Types + Backward Compatibility

**Files:**
- Modify: `internal/node/git/review/review.go:27-42`
- Test: `internal/node/git/review/review_test.go`

- [ ] **Step 1: Write tests for new types and backward-compatible unmarshaling**

Add to `internal/node/git/review/review_test.go`:

```go
func TestLineRange_UnmarshalJSON_NewFormat(t *testing.T) {
	data := []byte(`{"from":{"side":"L","line":10},"to":{"side":"L","line":10}}`)
	var lr LineRange
	if err := json.Unmarshal(data, &lr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if lr.From.Side != DiffSideLeft || lr.From.Line != 10 {
		t.Errorf("from = %+v, want {L, 10}", lr.From)
	}
	if lr.To.Side != DiffSideLeft || lr.To.Line != 10 {
		t.Errorf("to = %+v, want {L, 10}", lr.To)
	}
}

func TestLineRange_UnmarshalJSON_LegacyFormat(t *testing.T) {
	data := []byte(`{"from":5,"to":5}`)
	var lr LineRange
	if err := json.Unmarshal(data, &lr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if lr.From.Side != DiffSideRight || lr.From.Line != 5 {
		t.Errorf("from = %+v, want {R, 5}", lr.From)
	}
	if lr.To.Side != DiffSideRight || lr.To.Line != 5 {
		t.Errorf("to = %+v, want {R, 5}", lr.To)
	}
}

func TestLineRange_MarshalJSON(t *testing.T) {
	lr := LineRange{
		From: DiffPosition{Side: DiffSideLeft, Line: 43},
		To:   DiffPosition{Side: DiffSideLeft, Line: 43},
	}
	data, err := json.Marshal(lr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Should always write new format
	var parsed map[string]interface{}
	json.Unmarshal(data, &parsed)
	fromMap, ok := parsed["from"].(map[string]interface{})
	if !ok {
		t.Fatalf("from is not object: %s", string(data))
	}
	if fromMap["side"] != "L" {
		t.Errorf("from.side = %v, want L", fromMap["side"])
	}
}

func TestReviewComment_NewFields(t *testing.T) {
	data := []byte(`{
		"id": "rc_1",
		"file": "src/auth.ts",
		"oldPath": "src/old-auth.ts",
		"line": {"from": {"side": "L", "line": 10}, "to": {"side": "L", "line": 10}},
		"snippet": "const x = 1;",
		"snippetContext": "line above\nconst x = 1;\nline below",
		"anchorStatus": "stale",
		"body": "Fix this",
		"submitted": true,
		"createdAt": "2026-04-02T00:00:00Z"
	}`)
	var c ReviewComment
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.OldPath != "src/old-auth.ts" {
		t.Errorf("oldPath = %q, want %q", c.OldPath, "src/old-auth.ts")
	}
	if c.SnippetContext != "line above\nconst x = 1;\nline below" {
		t.Errorf("snippetContext = %q", c.SnippetContext)
	}
	if c.AnchorStatus != AnchorStale {
		t.Errorf("anchorStatus = %q, want %q", c.AnchorStatus, AnchorStale)
	}
}

func TestLegacyReviewFile_Migration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	// Write legacy format directly
	legacy := []byte(`{
		"head": "feat/x", "base": "main",
		"comments": [{
			"id": "rc_1", "file": "src/x.ts",
			"line": {"from": 10, "to": 10},
			"snippet": "const x", "body": "fix", "submitted": false,
			"createdAt": "2026-01-01T00:00:00Z"
		}]
	}`)
	os.WriteFile(path, legacy, 0o644)

	r, err := readReviewFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if r.Comments[0].Line.From.Side != DiffSideRight {
		t.Errorf("expected legacy to migrate to R side, got %q", r.Comments[0].Line.From.Side)
	}
	if r.Comments[0].Line.From.Line != 10 {
		t.Errorf("expected line 10, got %d", r.Comments[0].Line.From.Line)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-67-deletions-inline-comments && go test ./internal/node/git/review/ -run "TestLineRange|TestReviewComment_NewFields|TestLegacyReviewFile" -v`

Expected: FAIL — types `DiffSide`, `DiffPosition`, `AnchorStatus` not defined.

- [ ] **Step 3: Implement new types and backward-compatible unmarshaling**

Replace the type definitions in `internal/node/git/review/review.go` (lines 27-42):

```go
// DiffSide indicates which side of the diff a position refers to.
type DiffSide string

const (
	DiffSideLeft  DiffSide = "L"
	DiffSideRight DiffSide = "R"
)

// AnchorStatus indicates whether a comment's anchor is still valid.
type AnchorStatus string

const (
	AnchorResolved           AnchorStatus = "resolved"
	AnchorStale              AnchorStatus = "stale"
	AnchorContextUnavailable AnchorStatus = "context_unavailable"
)

// DiffPosition is a line number on a specific side of the diff.
type DiffPosition struct {
	Side DiffSide `json:"side"`
	Line int      `json:"line"`
}

// LineRange represents a range of positions in a diff (1-indexed, inclusive).
type LineRange struct {
	From DiffPosition `json:"from"`
	To   DiffPosition `json:"to"`
}

// UnmarshalJSON handles both legacy {"from": N, "to": N} and new
// {"from": {"side": "R", "line": N}, "to": {"side": "R", "line": N}} formats.
func (lr *LineRange) UnmarshalJSON(data []byte) error {
	// Try new format first
	type lineRangeNew struct {
		From DiffPosition `json:"from"`
		To   DiffPosition `json:"to"`
	}
	var newFmt lineRangeNew
	if err := json.Unmarshal(data, &newFmt); err == nil && newFmt.From.Side != "" {
		lr.From = newFmt.From
		lr.To = newFmt.To
		return nil
	}
	// Fall back to legacy format
	type lineRangeLegacy struct {
		From int `json:"from"`
		To   int `json:"to"`
	}
	var legacy lineRangeLegacy
	if err := json.Unmarshal(data, &legacy); err != nil {
		return fmt.Errorf("invalid line range: %w", err)
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
	AnchorStatus   AnchorStatus `json:"anchorStatus,omitempty"`
	Body           string       `json:"body"`
	Submitted      bool         `json:"submitted"`
	CreatedAt      string       `json:"createdAt"`
}
```

- [ ] **Step 4: Fix existing tests that reference the old LineRange format**

Update `TestWriteAndReadReviewFile` and other tests in `review_test.go` that create `LineRange{From: 10, To: 12}` to use the new format: `LineRange{From: DiffPosition{Side: DiffSideRight, Line: 10}, To: DiffPosition{Side: DiffSideRight, Line: 12}}`.

Update all `LineRange{From: N, To: N}` literals in the test file similarly.

- [ ] **Step 5: Run all tests to verify they pass**

Run: `go test ./internal/node/git/review/ -v`

Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/node/git/review/review.go internal/node/git/review/review_test.go
git commit -m "feat(review): add side-aware DiffPosition types with backward-compatible unmarshaling (BXN-67)"
```

---

### Task 2: Backend Staleness Detection — Side-Aware with Immutable Refs

**Files:**
- Modify: `internal/node/git/review/review.go:116-189` (detectStaleness, findSnippet)
- Modify: `internal/node/git/review/review.go:201-217` (Load)
- Test: `internal/node/git/review/review_test.go`

- [ ] **Step 1: Write tests for side-aware staleness detection**

Add to `review_test.go`:

```go
func TestDetectStaleness_RSideUsesHeadRef(t *testing.T) {
	dir := t.TempDir()
	// Create a git repo with a file at HEAD
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "checkout", "-b", "main")
	filePath := filepath.Join(dir, "src", "auth.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	os.WriteFile(filePath, []byte("line1\nconst TOKEN = 1800;\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := strings.TrimSpace(runCmdOutput(t, dir, "git", "rev-parse", "HEAD"))

	comments := []ReviewComment{{
		ID: "rc_1", File: "src/auth.ts",
		Line: LineRange{From: DiffPosition{Side: DiffSideRight, Line: 10}, To: DiffPosition{Side: DiffSideRight, Line: 10}},
		Snippet: "const TOKEN = 1800;",
		Body: "Change to 3600", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].Line.From.Line != 2 {
		t.Errorf("expected re-anchored to line 2, got %d", result[0].Line.From.Line)
	}
}

func TestDetectStaleness_LSideUsesBaseRef(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "checkout", "-b", "main")
	filePath := filepath.Join(dir, "src", "auth.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	os.WriteFile(filePath, []byte("old line1\nconst OLD = 100;\nold line3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "base")
	baseRef := strings.TrimSpace(runCmdOutput(t, dir, "git", "rev-parse", "HEAD"))

	// Modify file on HEAD (L-side comment should anchor against baseRef)
	os.WriteFile(filePath, []byte("new content\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "head")

	comments := []ReviewComment{{
		ID: "rc_1", File: "src/auth.ts",
		Line: LineRange{From: DiffPosition{Side: DiffSideLeft, Line: 5}, To: DiffPosition{Side: DiffSideLeft, Line: 5}},
		Snippet: "const OLD = 100;",
		Body: "Old code", Submitted: true,
	}}
	result := detectStaleness(dir, "", baseRef, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].Line.From.Line != 2 {
		t.Errorf("expected re-anchored to line 2, got %d", result[0].Line.From.Line)
	}
}

func TestDetectStaleness_LSideRenamedFile(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "checkout", "-b", "main")
	filePath := filepath.Join(dir, "src", "old.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	os.WriteFile(filePath, []byte("line1\nconst X = 1;\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "base")
	baseRef := strings.TrimSpace(runCmdOutput(t, dir, "git", "rev-parse", "HEAD"))

	// Rename file
	runCmd(t, dir, "git", "mv", "src/old.ts", "src/new.ts")
	runCmd(t, dir, "git", "commit", "-m", "rename")

	comments := []ReviewComment{{
		ID: "rc_1", File: "src/new.ts", OldPath: "src/old.ts",
		Line: LineRange{From: DiffPosition{Side: DiffSideLeft, Line: 5}, To: DiffPosition{Side: DiffSideLeft, Line: 5}},
		Snippet: "const X = 1;",
		Body: "Old code", Submitted: true,
	}}
	result := detectStaleness(dir, "", baseRef, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
}

func TestDetectStaleness_StaleWhenAmbiguous(t *testing.T) {
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "checkout", "-b", "main")
	filePath := filepath.Join(dir, "src", "util.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	// Multiple "}" at lines 5, 10, 15, 20, 25
	var lines []string
	for i := 1; i <= 25; i++ {
		if i%5 == 0 {
			lines = append(lines, "}")
		} else {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := strings.TrimSpace(runCmdOutput(t, dir, "git", "rev-parse", "HEAD"))

	comments := []ReviewComment{{
		ID: "rc_1", File: "src/util.ts",
		Line: LineRange{From: DiffPosition{Side: DiffSideRight, Line: 6}, To: DiffPosition{Side: DiffSideRight, Line: 6}},
		Snippet: "}",
		SnippetContext: "unique context that does not match",
		Body: "Close brace", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (marked stale), got %d", len(result))
	}
	if result[0].AnchorStatus != AnchorStale {
		t.Errorf("expected anchorStatus=stale, got %q", result[0].AnchorStatus)
	}
}
```

Also add helper functions at the top of the test file:

```go
import "os/exec"

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

func runCmdOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v failed: %v", name, args, err)
	}
	return string(out)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/git/review/ -run "TestDetectStaleness_(RSide|LSide|StaleWhen)" -v`

Expected: FAIL — `detectStaleness` signature doesn't match.

- [ ] **Step 3: Implement side-aware staleness detection**

Update `detectStaleness` in `review.go`:

```go
// getFileContent retrieves file content from a git ref using "git show ref:path".
func getFileContent(repoDir, ref, filePath string) (string, error) {
	cmd := exec.Command("git", "show", ref+":"+filePath)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// detectStaleness re-anchors submitted comments against immutable commit refs.
// R-side comments are checked against headRef, L-side against baseRef.
// Unsubmitted (draft) comments are always preserved as-is.
func detectStaleness(repoDir, headRef, baseRef string, comments []ReviewComment) []ReviewComment {
	result := make([]ReviewComment, 0)
	for _, c := range comments {
		if !c.Submitted {
			result = append(result, c)
			continue
		}

		side := c.Line.From.Side
		if side == "" {
			side = DiffSideRight
		}

		// Determine which ref and path to search
		var ref, path string
		if side == DiffSideLeft {
			ref = baseRef
			path = c.OldPath
			if path == "" {
				path = c.File
			}
		} else {
			ref = headRef
			path = c.File
		}

		if ref == "" {
			// No ref available — cannot re-anchor, preserve as-is
			result = append(result, c)
			continue
		}

		if err := ValidateFilePath(repoDir, path); err != nil {
			continue
		}

		fileText, err := getFileContent(repoDir, ref, path)
		if err != nil {
			continue
		}

		lineNum := findSnippetWithContext(fileText, c.Snippet, c.SnippetContext, c.Line.From.Line)
		if lineNum == -1 {
			continue
		}
		if lineNum == -2 {
			// Ambiguous — mark stale
			c.AnchorStatus = AnchorStale
			result = append(result, c)
			continue
		}
		lineCount := strings.Count(c.Snippet, "\n")
		c.Line = LineRange{
			From: DiffPosition{Side: side, Line: lineNum},
			To:   DiffPosition{Side: side, Line: lineNum + lineCount},
		}
		c.AnchorStatus = ""
		result = append(result, c)
	}
	return result
}
```

Add `"os/exec"` to imports.

- [ ] **Step 4: Implement enhanced findSnippet with context disambiguation**

Add `findSnippetWithContext` to `review.go`:

```go
// findSnippetWithContext extends findSnippet with context-based disambiguation.
// Returns -1 if not found, -2 if ambiguous (multiple matches, context doesn't help).
func findSnippetWithContext(fileText, snippet, snippetContext string, priorLine int) int {
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

	// Multiple matches — try context disambiguation if available
	if snippetContext != "" {
		fileLines := strings.Split(fileText, "\n")
		for _, line := range matchLines {
			// Build context window around this match
			contextStart := line - 2
			if contextStart < 1 {
				contextStart = 1
			}
			contextEnd := line + strings.Count(snippet, "\n") + 2
			if contextEnd > len(fileLines) {
				contextEnd = len(fileLines)
			}
			window := strings.Join(fileLines[contextStart-1:contextEnd], "\n")
			if strings.Contains(window, snippetContext) || strings.Contains(snippetContext, strings.TrimSpace(window)) {
				return line
			}
		}
		// Context didn't match any candidate — ambiguous
		return -2
	}

	// No context available — fall back to nearest match with distance threshold
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

- [ ] **Step 5: Update Load() to pass refs through**

Update `Load` signature and body in `review.go`:

```go
// Load reads the review file for the given branch pair, runs staleness detection,
// persists any re-anchoring, and returns the result. Returns nil, nil if no file exists.
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
	if err := writeReviewFile(path, r); err != nil {
		return nil, err
	}
	return r, nil
}
```

- [ ] **Step 6: Run all review tests**

Run: `go test ./internal/node/git/review/ -v`

Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/node/git/review/review.go internal/node/git/review/review_test.go
git commit -m "feat(review): side-aware staleness detection with immutable refs (BXN-67)"
```

---

### Task 3: Backend API — Validation + Review Load with Refs

**Files:**
- Modify: `internal/node/api/review.go:29-61,63-96`
- Test: `internal/node/api/review_test.go`

- [ ] **Step 1: Write tests for DiffPosition validation and ref passthrough**

Add to `internal/node/api/review_test.go`:

```go
func TestReviewHandler_PostRejectsInvalidSide(t *testing.T) {
	dir := t.TempDir()
	h := &reviewHandler{projectDirOverride: dir}
	body := `{"head":"feat","base":"main","comments":[{
		"id":"rc_1","file":"src/x.ts",
		"line":{"from":{"side":"X","line":1},"to":{"side":"X","line":1}},
		"snippet":"x","body":"y","submitted":false,"createdAt":"2026-01-01T00:00:00Z"
	}]}`
	req := httptest.NewRequest("POST", "/api/git/review?path="+dir, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.post(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestReviewHandler_PostRejectsNonPositiveLine(t *testing.T) {
	dir := t.TempDir()
	h := &reviewHandler{projectDirOverride: dir}
	body := `{"head":"feat","base":"main","comments":[{
		"id":"rc_1","file":"src/x.ts",
		"line":{"from":{"side":"R","line":0},"to":{"side":"R","line":0}},
		"snippet":"x","body":"y","submitted":false,"createdAt":"2026-01-01T00:00:00Z"
	}]}`
	req := httptest.NewRequest("POST", "/api/git/review?path="+dir, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.post(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestReviewHandler_PostRejectsOldPathTraversal(t *testing.T) {
	dir := t.TempDir()
	h := &reviewHandler{projectDirOverride: dir}
	body := `{"head":"feat","base":"main","comments":[{
		"id":"rc_1","file":"src/x.ts","oldPath":"../etc/passwd",
		"line":{"from":{"side":"L","line":1},"to":{"side":"L","line":1}},
		"snippet":"x","body":"y","submitted":false,"createdAt":"2026-01-01T00:00:00Z"
	}]}`
	req := httptest.NewRequest("POST", "/api/git/review?path="+dir, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.post(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/api/ -run "TestReviewHandler_PostRejects(InvalidSide|NonPositive|OldPath)" -v`

Expected: FAIL

- [ ] **Step 3: Add validation to POST handler and update GET handler**

Update `internal/node/api/review.go`:

In the `post` method, after the existing file path validation loop (line 85-90), add:

```go
	for _, c := range rv.Comments {
		if _, err := sanitizeFilePath(expandedPath, c.File); err != nil {
			respondError(w, http.StatusBadRequest, "invalid file path in comment: "+c.File)
			return
		}
		if c.OldPath != "" {
			if _, err := sanitizeFilePath(expandedPath, c.OldPath); err != nil {
				respondError(w, http.StatusBadRequest, "invalid oldPath in comment: "+c.OldPath)
				return
			}
		}
		if err := validateCommentLine(c.Line); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
```

Add the validation function:

```go
func validateCommentLine(lr review.LineRange) error {
	if lr.From.Side != review.DiffSideLeft && lr.From.Side != review.DiffSideRight {
		return fmt.Errorf("invalid line.from.side: %q", lr.From.Side)
	}
	if lr.To.Side != review.DiffSideLeft && lr.To.Side != review.DiffSideRight {
		return fmt.Errorf("invalid line.to.side: %q", lr.To.Side)
	}
	if lr.From.Line <= 0 {
		return fmt.Errorf("line.from.line must be > 0")
	}
	if lr.To.Line <= 0 {
		return fmt.Errorf("line.to.line must be > 0")
	}
	if lr.From.Side != lr.To.Side || lr.From.Line != lr.To.Line {
		return fmt.Errorf("line.from must equal line.to (single-line comments only)")
	}
	return nil
}
```

Update the `get` method to accept optional `headRef`/`baseRef` query params:

```go
func (h *reviewHandler) get(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")
	headRef := r.URL.Query().Get("headRef")
	baseRef := r.URL.Query().Get("baseRef")
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
	rv, err := review.Load(projectDir, expandedPath, branch, base, headRef, baseRef)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if rv == nil {
		rv = &review.Review{
			Head:     branch,
			Base:     base,
			Comments: []review.ReviewComment{},
		}
	}
	respondJSON(w, http.StatusOK, rv)
}
```

Add `"fmt"` to imports.

- [ ] **Step 4: Fix existing API tests for updated Load signature**

Update `TestReviewHandler_PostAndGet` and other tests that call `review.Load` to pass empty strings for the new `headRef`/`baseRef` parameters (or update the test to provide valid refs).

- [ ] **Step 5: Run all API tests**

Run: `go test ./internal/node/api/ -v`

Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/node/api/review.go internal/node/api/review_test.go
git commit -m "feat(api): add DiffPosition validation and ref passthrough for review loading (BXN-67)"
```

---

### Task 4: Frontend Types — Side-Aware ReviewComment

**Files:**
- Modify: `web/src/types/review.ts`

- [ ] **Step 1: Update TypeScript types**

Replace the contents of `web/src/types/review.ts`:

```ts
export type DiffSide = "L" | "R";

export interface DiffPosition {
  side: DiffSide;
  line: number;
}

export interface LineRange {
  from: DiffPosition;
  to: DiffPosition;
}

export type AnchorStatus = "resolved" | "stale" | "context_unavailable";

export interface ReviewComment {
  id: string;
  file: string;
  oldPath?: string;
  line: LineRange;
  snippet: string;
  snippetContext?: string;
  anchorStatus?: AnchorStatus;
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface ReviewBody {
  body: string;
  submitted: boolean;
  createdAt: string;
}

export interface Review {
  head: string;
  base: string;
  body?: ReviewBody;
  comments: ReviewComment[];
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit 2>&1 | head -50`

Expected: Type errors in files that reference `line.from` as a number (CompareView, UnifiedDiff, etc.). This is expected — we'll fix these in subsequent tasks.

- [ ] **Step 3: Commit**

```bash
git add web/src/types/review.ts
git commit -m "feat(types): add side-aware DiffPosition and AnchorStatus types (BXN-67)"
```

---

### Task 5: Frontend Comment Anchoring — Side-Aware Indexing in UnifiedDiff

**Files:**
- Modify: `web/src/components/DiffViewer/UnifiedDiff.tsx:19-20,63-77,209-286,288-356`

- [ ] **Step 1: Update UnifiedDiff props and comment indexing**

In `UnifiedDiff.tsx`, change `activeCommentLine` type and `onLineClick` signature:

```ts
import type { ReviewComment, DiffPosition } from "@/types";

interface UnifiedDiffProps {
  // ... existing props ...
  activeCommentLine?: { position: DiffPosition } | null; // was { from: number; to: number }
  onLineClick?: (position: DiffPosition) => void; // was (line: number) => void
  // ... rest unchanged ...
}
```

Update `commentsByLine` to use string keys:

```ts
const commentsByLine = useMemo(() => {
  if (effectiveComments.length === 0) return null;
  const map = new Map<string, ReviewComment[]>();
  for (const c of effectiveComments) {
    const key = `${c.line.to.side}${c.line.to.line}`;
    const arr = map.get(key);
    if (arr) arr.push(c);
    else map.set(key, [c]);
  }
  return map;
}, [effectiveComments]);
```

- [ ] **Step 2: Update Hunk component for side-aware comment lookup**

Update `Hunk` props and body:

```ts
function Hunk({
  hunk,
  wrapLines,
  commentsByLine,
  activeCommentLine,
  onLineClick,
  onAddComment,
  onCancelComment,
  onDeleteComment,
  onCommentRef,
  commentingEnabled,
}: {
  hunk: DiffHunk;
  wrapLines: boolean;
  commentsByLine: Map<string, ReviewComment[]> | null;
  activeCommentLine: { position: DiffPosition } | null;
  onLineClick?: (position: DiffPosition) => void;
  onAddComment?: (body: string) => void;
  onCancelComment?: () => void;
  onDeleteComment?: (id: string) => void;
  onCommentRef?: (id: string, el: HTMLElement | null) => void;
  commentingEnabled: boolean;
}) {
  return (
    <div className="min-w-full">
      <div className="border-border border-y bg-blue-500/10 px-3 py-1 text-xs text-blue-400">
        {hunk.header}
      </div>
      {hunk.lines.map((line, index) => {
        // Determine if this line is the active comment target
        const isInActiveRange =
          activeCommentLine != null &&
          ((activeCommentLine.position.side === "L" && line.oldLineNumber === activeCommentLine.position.line) ||
           (activeCommentLine.position.side === "R" && line.newLineNumber === activeCommentLine.position.line));

        // Look up comments by anchor keys for this line
        let lineComments: ReviewComment[] = EMPTY_COMMENTS;
        if (commentsByLine) {
          const lKey = line.oldLineNumber != null ? commentsByLine.get(`L${line.oldLineNumber}`) : undefined;
          const rKey = line.newLineNumber != null ? commentsByLine.get(`R${line.newLineNumber}`) : undefined;
          if (lKey && rKey) {
            lineComments = [...lKey, ...rKey];
          } else {
            lineComments = lKey ?? rKey ?? EMPTY_COMMENTS;
          }
        }

        // Show form after the active comment's anchor line
        const showForm = activeCommentLine != null &&
          ((activeCommentLine.position.side === "L" && line.oldLineNumber === activeCommentLine.position.line) ||
           (activeCommentLine.position.side === "R" && line.newLineNumber === activeCommentLine.position.line));

        return (
          <Fragment key={index}>
            <DiffLineRow
              line={line}
              wrapLines={wrapLines}
              isInActiveRange={isInActiveRange}
              onLineClick={onLineClick}
              commentingEnabled={commentingEnabled}
            />
            {lineComments.map((c) => (
              <div
                key={c.id}
                ref={(el) => onCommentRef?.(c.id, el)}
                className={cn(!wrapLines && "sticky left-0")}
                style={!wrapLines ? { width: "calc(100vw - 0.75rem * 2 - 2px)" } : undefined}
              >
                <InlineCommentCard comment={c} onDelete={onDeleteComment!} />
              </div>
            ))}
            {showForm && onAddComment && onCancelComment && (
              <div className={cn(!wrapLines && "sticky left-0")}
                style={!wrapLines ? { width: "calc(100vw - 0.75rem * 2 - 2px)" } : undefined}>
                <InlineCommentForm
                  onSubmit={onAddComment}
                  onCancel={onCancelComment}
                />
              </div>
            )}
          </Fragment>
        );
      })}
    </div>
  );
}
```

- [ ] **Step 3: Update DiffLineRow to make all line types commentable**

```ts
function DiffLineRow({
  line,
  wrapLines,
  isInActiveRange,
  onLineClick,
  commentingEnabled,
}: {
  line: DiffLine;
  wrapLines: boolean;
  isInActiveRange: boolean;
  onLineClick?: (position: DiffPosition) => void;
  commentingEnabled: boolean;
}) {
  if (line.type === "header") return null;

  const bgColor =
    line.type === "addition"
      ? "bg-green-500/10"
      : line.type === "deletion"
        ? "bg-red-500/10"
        : "";

  const textColor =
    line.type === "addition"
      ? "text-green-400"
      : line.type === "deletion"
        ? "text-red-400"
        : "text-foreground";

  const marker =
    line.type === "addition" ? "+" : line.type === "deletion" ? "-" : "";

  // All non-header lines with at least one line number and non-empty content are commentable
  const hasLineNumber = line.oldLineNumber != null || line.newLineNumber != null;
  const isCommentable = commentingEnabled && hasLineNumber && line.content.trim() !== "";

  // Determine which position to pass on click
  const clickPosition: DiffPosition | null = isCommentable
    ? line.type === "deletion"
      ? { side: "L", line: line.oldLineNumber! }
      : { side: "R", line: line.newLineNumber! }
    : null;

  return (
    <div className={cn("flex hover:bg-muted/30", bgColor, isInActiveRange && "bg-blue-500/10")}>
      <div className={cn(
        "text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none",
        isCommentable && line.type === "deletion" && "cursor-pointer hover:bg-blue-500/20 hover:text-blue-400",
      )}
        onClick={
          isCommentable && line.type === "deletion" && clickPosition
            ? () => onLineClick?.(clickPosition)
            : undefined
        }
      >
        {line.oldLineNumber ?? ""}
      </div>
      <div
        className={cn(
          "text-muted-foreground border-border/50 w-10 shrink-0 border-r px-2 py-0.5 text-right tabular-nums select-none",
          isCommentable && line.type !== "deletion" && "cursor-pointer hover:bg-blue-500/20 hover:text-blue-400",
        )}
        onClick={
          isCommentable && line.type !== "deletion" && clickPosition
            ? () => onLineClick?.(clickPosition)
            : undefined
        }
      >
        {line.newLineNumber ?? ""}
      </div>
      <div className={cn("w-5 shrink-0 px-1 py-0.5 text-center select-none", textColor)}>
        {marker}
      </div>
      <div className={cn(
        "min-w-0 flex-1 px-2 py-0.5",
        wrapLines ? "whitespace-pre-wrap break-words" : "whitespace-pre",
        textColor,
      )}>
        {line.content || " "}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Verify TypeScript compiles for UnifiedDiff**

Run: `cd web && npx tsc --noEmit 2>&1 | grep -i "UnifiedDiff" | head -10`

Expected: No errors in UnifiedDiff.tsx (errors may remain in CompareView which is next task).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/DiffViewer/UnifiedDiff.tsx
git commit -m "feat(diff): side-aware comment anchoring and all-line-type commenting (BXN-67)"
```

---

### Task 6: Frontend CompareView — Side-Aware Comment Creation + Snippet Extraction

**Files:**
- Modify: `web/src/components/GitPanel/CompareView.tsx:48-52,162-171,232-234,253-260,262-266,276-307,561-576`
- Modify: `web/src/components/DiffViewer/ExpandableUnifiedDiff.tsx` (prop type updates)

- [ ] **Step 1: Update activeComment state and handleLineClick**

In `CompareView.tsx`, update the state type and handler:

```ts
const [activeComment, setActiveComment] = useState<{
  file: string;
  position: DiffPosition;
} | null>(null);
```

Update `handleLineClick`:

```ts
const handleLineClick = useCallback((file: string, position: DiffPosition) => {
  setActiveComment({ file, position });
}, []);
```

Update `fileLineClickHandlers`:

```ts
const fileLineClickHandlers = useMemo(() => {
  const map = new Map<string, (position: DiffPosition) => void>();
  for (const diff of parsedDiffs) {
    const pathKey = getDiffPathKey(diff);
    map.set(pathKey, (position: DiffPosition) => handleLineClick(pathKey, position));
  }
  return map;
}, [parsedDiffs, handleLineClick]);
```

Update `activeCommentLine`:

```ts
const activeCommentLine = useMemo(
  () => activeComment ? { position: activeComment.position } : null,
  [activeComment?.position?.side, activeComment?.position?.line],
);
```

- [ ] **Step 2: Update snippet extraction and comment creation**

Update `handleAddComment`:

```ts
const handleAddComment = useCallback((body: string) => {
  if (!activeComment) return;

  const hunks = getHunksForFile(activeComment.file);
  const pos = activeComment.position;
  let snippet = "";
  let snippetContext = "";

  // Find the anchor line's content
  for (const hunk of hunks) {
    for (const line of hunk.lines) {
      const match = pos.side === "L"
        ? line.oldLineNumber === pos.line
        : line.newLineNumber === pos.line;
      if (match) {
        snippet = line.content;
        break;
      }
    }
    if (snippet) break;
  }

  // Extract same-side snippet context (1-2 lines above/below)
  if (snippet) {
    const sameSideLines: string[] = [];
    let anchorIdx = -1;
    for (const hunk of hunks) {
      for (const line of hunk.lines) {
        const lineNum = pos.side === "L" ? line.oldLineNumber : line.newLineNumber;
        if (lineNum != null) {
          sameSideLines.push(line.content);
          if (lineNum === pos.line) {
            anchorIdx = sameSideLines.length - 1;
          }
        }
      }
    }
    if (anchorIdx >= 0) {
      const start = Math.max(0, anchorIdx - 2);
      const end = Math.min(sameSideLines.length, anchorIdx + 3);
      snippetContext = sameSideLines.slice(start, end).join("\n");
    }
  }

  // Determine oldPath for renames
  let oldPath: string | undefined;
  if (pos.side === "L") {
    const diff = parsedDiffs.find((d) => getDiffPathKey(d) === activeComment.file);
    if (diff?.isRenamed) {
      oldPath = diff.oldFile;
    }
  }

  const newComment: ReviewComment = {
    id: `rc_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
    file: activeComment.file,
    ...(oldPath ? { oldPath } : {}),
    line: { from: pos, to: pos },
    snippet,
    ...(snippetContext ? { snippetContext } : {}),
    body,
    submitted: false,
    createdAt: new Date().toISOString(),
  };

  saveAndUpdate((prev) => ({
    ...prev,
    comments: [...prev.comments, newComment],
  }));
  setActiveComment(null);
}, [activeComment, getHunksForFile, saveAndUpdate, parsedDiffs]);
```

- [ ] **Step 3: Update sortedComments for unified row order**

```ts
const sortedComments = useMemo(() => {
  if (!comments.length || !parsedDiffs.length) return [];
  const fileOrder = parsedDiffs.map((d) => getDiffPathKey(d));

  // Build a line position index for each file
  const linePositions = new Map<string, Map<string, number>>();
  for (const diff of parsedDiffs) {
    const pathKey = getDiffPathKey(diff);
    const posMap = new Map<string, number>();
    let rowIdx = 0;
    for (const hunk of diff.hunks) {
      for (const line of hunk.lines) {
        if (line.oldLineNumber != null) posMap.set(`L${line.oldLineNumber}`, rowIdx);
        if (line.newLineNumber != null) posMap.set(`R${line.newLineNumber}`, rowIdx);
        rowIdx++;
      }
    }
    linePositions.set(pathKey, posMap);
  }

  return [...comments].sort((a, b) => {
    const ai = fileOrder.indexOf(a.file);
    const bi = fileOrder.indexOf(b.file);
    if (ai !== bi) return ai - bi;
    const posA = linePositions.get(a.file);
    const posB = linePositions.get(b.file);
    const keyA = `${a.line.from.side}${a.line.from.line}`;
    const keyB = `${b.line.from.side}${b.line.from.line}`;
    const idxA = posA?.get(keyA) ?? a.line.from.line;
    const idxB = posB?.get(keyB) ?? b.line.from.line;
    return idxA - idxB;
  });
}, [comments, parsedDiffs]);
```

- [ ] **Step 4: Update MobileCommentSheet active lines**

Update the `activeLines` computation (around line 561-576):

```ts
activeLines={activeComment ? (() => {
  const hunks = getHunksForFile(activeComment.file);
  const pos = activeComment.position;
  const lines: DiffLine[] = [];
  for (const hunk of hunks) {
    for (const line of hunk.lines) {
      const match = pos.side === "L"
        ? line.oldLineNumber === pos.line
        : line.newLineNumber === pos.line;
      if (match) {
        lines.push(line);
      }
    }
  }
  return lines;
})() : []}
```

- [ ] **Step 5: Update review query to pass refs**

In `web/src/data/review/queries.ts`, update `useReviewQuery`:

```ts
export function useReviewQuery(
  path: string,
  head: string | undefined,
  base: string | null,
  headRef?: string,
  baseRef?: string,
) {
  return useQuery({
    queryKey: reviewKeys.forComparison(path, head ?? "", base ?? ""),
    queryFn: async () => {
      const params = new URLSearchParams({
        path,
        branch: head!,
        base: base!,
      });
      if (headRef) params.set("headRef", headRef);
      if (baseRef) params.set("baseRef", baseRef);
      return apiFetch<Review>(`/node/api/git/review?${params}`);
    },
    staleTime: 30_000,
    enabled: path.trim().length > 0 && !!head && !!base,
  });
}
```

In `CompareView.tsx`, update the `useReviewQuery` call:

```ts
const {
  data: reviewData,
} = useReviewQuery(workingDirectory, currentBranch, baseBranch, compareData?.headRef, compareData?.baseRef);
```

- [ ] **Step 6: Verify TypeScript compiles cleanly**

Run: `cd web && npx tsc --noEmit`

Expected: No errors.

- [ ] **Step 7: Commit**

```bash
git add web/src/components/GitPanel/CompareView.tsx web/src/data/review/queries.ts web/src/components/DiffViewer/ExpandableUnifiedDiff.tsx
git commit -m "feat(compare): side-aware comment creation, snippet extraction, and ref passthrough (BXN-67)"
```

---

### Task 7: Frontend InlineCommentCard — Anchor Status Visual Treatment

**Files:**
- Modify: `web/src/components/DiffViewer/InlineCommentCard.tsx`

- [ ] **Step 1: Add anchor status visual treatment**

Update `InlineCommentCard.tsx` to show stale/unavailable status:

```tsx
import type { ReviewComment } from "@/types";
import { cn } from "@/lib/utils";
import { X, AlertTriangle } from "lucide-react";

interface InlineCommentCardProps {
  comment: ReviewComment;
  onDelete: (id: string) => void;
}

export function InlineCommentCard({ comment, onDelete }: InlineCommentCardProps) {
  const isDraft = !comment.submitted;
  const isStale = comment.anchorStatus === "stale";
  const isUnavailable = comment.anchorStatus === "context_unavailable";

  return (
    <div
      className={cn(
        "border-border mx-3 my-1 rounded-md border px-3 py-2 text-sm",
        isDraft ? "bg-yellow-500/5 border-yellow-500/30" : "bg-muted/30",
        isStale && "border-yellow-500/50 bg-yellow-500/10",
        isUnavailable && "border-red-500/30 bg-red-500/5",
      )}
    >
      <div className="mb-1 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            className={cn(
              "rounded-full px-2 py-0.5 text-xs font-medium",
              isDraft
                ? "bg-yellow-500/20 text-yellow-500"
                : "bg-green-500/20 text-green-500",
            )}
          >
            {isDraft ? "Pending" : "Submitted"}
          </span>
          {isStale && (
            <span className="flex items-center gap-1 text-xs text-yellow-500">
              <AlertTriangle className="h-3 w-3" />
              Anchor may have moved
            </span>
          )}
          {isUnavailable && (
            <span className="flex items-center gap-1 text-xs text-red-400">
              <AlertTriangle className="h-3 w-3" />
              Context unavailable
            </span>
          )}
        </div>
        {isDraft && (
          <button
            onClick={() => onDelete(comment.id)}
            aria-label="Delete comment"
            className="text-muted-foreground hover:text-foreground -mr-1 rounded p-1 transition-colors"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        )}
      </div>
      <div className="text-foreground whitespace-pre-wrap text-xs">
        {comment.body}
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit 2>&1 | grep "InlineCommentCard"`

Expected: No errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/DiffViewer/InlineCommentCard.tsx
git commit -m "feat(diff): add stale/unavailable anchor status indicators to comment cards (BXN-67)"
```

---

### Task 8: Frontend Sparse Comment Hunks — ensureCommentVisible

This task implements the sparse hunk system for comments outside visible diff context. This is the most complex frontend task.

**Files:**
- Create: `web/src/hooks/useCommentVisibility.ts`
- Modify: `web/src/hooks/useExpandableDiff.ts` (add INSERT_SYNTHETIC action)
- Modify: `web/src/components/DiffViewer/ExpandableUnifiedDiff.tsx` (expose ensureCommentVisible)
- Modify: `web/src/components/GitPanel/CompareView.tsx` (call on load + navigation)

- [ ] **Step 1: Add INSERT_SYNTHETIC action to useExpandableDiff reducer**

In `web/src/hooks/useExpandableDiff.ts`, extend the `ExpandAction` type:

```ts
type ExpandAction =
  | { type: "EXPAND_UP"; hunkIndex: number; lines: DiffLine[] }
  | { type: "EXPAND_DOWN"; hunkIndex: number; lines: DiffLine[] }
  | { type: "RESET"; hunks: DiffHunk[]; totalLines: number }
  | { type: "INSERT_SYNTHETIC"; hunk: DiffHunk; insertIndex: number };
```

Add the case to the reducer:

```ts
case "INSERT_SYNTHETIC": {
  const { hunk, insertIndex } = action;
  const hunks = [...state.hunks];
  // Check for duplicate — don't insert if a hunk with overlapping range already exists
  const isDuplicate = hunks.some((h) =>
    h.newStart <= hunk.newStart + hunk.newCount &&
    h.newStart + h.newCount >= hunk.newStart
  );
  if (isDuplicate) return state;
  hunks.splice(insertIndex, 0, hunk);
  return { hunks, totalLines: state.totalLines, generation: state.generation + 1 };
}
```

- [ ] **Step 2: Create useCommentVisibility hook**

Create `web/src/hooks/useCommentVisibility.ts`:

```ts
import { useCallback, useRef } from "react";
import type { DiffHunk, DiffLine } from "@/lib/diff-parser";
import type { DiffPosition, ReviewComment } from "@/types";
import { fetchFileLines } from "@/data/git/file-lines";

const CONTEXT_WINDOW = 3;

interface CommentVisibilityOptions {
  repoPath: string;
  filePath: string;
  headRef?: string;
  baseRef?: string;
  hunks: DiffHunk[];
  onInsertSynthetic: (hunk: DiffHunk, insertIndex: number) => void;
  onCommentStatusChange?: (commentId: string, status: "context_unavailable") => void;
}

function isPositionVisible(hunks: DiffHunk[], position: DiffPosition): boolean {
  for (const hunk of hunks) {
    for (const line of hunk.lines) {
      if (position.side === "L" && line.oldLineNumber === position.line) return true;
      if (position.side === "R" && line.newLineNumber === position.line) return true;
    }
  }
  return false;
}

function findInsertIndex(hunks: DiffHunk[], lineNum: number, side: DiffPosition["side"]): number {
  for (let i = 0; i < hunks.length; i++) {
    const hunkLine = side === "L" ? hunks[i].oldStart : hunks[i].newStart;
    if (hunkLine > lineNum) return i;
  }
  return hunks.length;
}

export function useCommentVisibility(options: CommentVisibilityOptions) {
  const pendingRef = useRef<Set<string>>(new Set());

  const ensureCommentVisible = useCallback(async (comment: ReviewComment): Promise<void> => {
    const pos = comment.line.to;
    if (isPositionVisible(options.hunks, pos)) return;

    const key = `${pos.side}${pos.line}`;
    if (pendingRef.current.has(key)) return;
    pendingRef.current.add(key);

    const ref = pos.side === "L" ? options.baseRef : options.headRef;
    const file = pos.side === "L" && comment.oldPath ? comment.oldPath : comment.file;
    const start = Math.max(1, pos.line - CONTEXT_WINDOW);
    const end = pos.line + CONTEXT_WINDOW;

    try {
      const result = await fetchFileLines({
        path: options.repoPath,
        file,
        start,
        end,
        ref,
      });

      // Build synthetic hunk
      const lines: DiffLine[] = result.lines.map((content, i) => ({
        type: "context" as const,
        content,
        newLineNumber: start + i,
        oldLineNumber: start + i, // Approximate — context lines have both
      }));

      const syntheticHunk: DiffHunk = {
        header: `@@ -${start},${lines.length} +${start},${lines.length} @@`,
        oldStart: start,
        oldCount: lines.length,
        newStart: start,
        newCount: lines.length,
        lines,
      };

      const insertIdx = findInsertIndex(options.hunks, start, pos.side);
      options.onInsertSynthetic(syntheticHunk, insertIdx);
    } catch {
      options.onCommentStatusChange?.(comment.id, "context_unavailable");
    } finally {
      pendingRef.current.delete(key);
    }
  }, [options]);

  const ensureAllVisible = useCallback(async (comments: ReviewComment[]) => {
    const hidden = comments.filter((c) => !isPositionVisible(options.hunks, c.line.to));
    await Promise.all(hidden.map((c) => ensureCommentVisible(c)));
  }, [options.hunks, ensureCommentVisible]);

  return { ensureCommentVisible, ensureAllVisible };
}
```

- [ ] **Step 3: Wire ensureCommentVisible into CompareView**

This is a complex integration step. In `CompareView.tsx`, after comments load:

1. For each file's comments, check if any are outside visible hunks
2. Call `ensureCommentVisible` for hidden comments
3. Before `scrollToComment`, call `ensureCommentVisible` for the target comment

The exact wiring depends on the existing hook architecture. The key integration point is:

- `ExpandableUnifiedDiff` needs to expose a dispatch for `INSERT_SYNTHETIC`
- `CompareView` needs to call `ensureAllVisible` after initial diff + review data loads
- `scrollToComment` needs to await `ensureCommentVisible` before scrolling

Add to `ExpandableUnifiedDiff` props:

```ts
onInsertSyntheticHunk?: (hunk: DiffHunk, insertIndex: number) => void;
```

And in the component body, expose the dispatch:

```ts
const handleInsertSynthetic = useCallback((hunk: DiffHunk, insertIndex: number) => {
  dispatch({ type: "INSERT_SYNTHETIC", hunk, insertIndex });
}, []);
```

Pass `handleInsertSynthetic` to the parent via `onInsertSyntheticHunk` callback.

- [ ] **Step 4: Verify TypeScript compiles**

Run: `cd web && npx tsc --noEmit`

Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/hooks/useCommentVisibility.ts web/src/hooks/useExpandableDiff.ts web/src/components/DiffViewer/ExpandableUnifiedDiff.tsx web/src/components/GitPanel/CompareView.tsx
git commit -m "feat(diff): add sparse comment hunk system with ensureCommentVisible (BXN-67)"
```

---

### Task 9: Integration Testing + Final Verification

**Files:**
- All modified files

- [ ] **Step 1: Run all backend tests**

Run: `go test ./internal/... -v`

Expected: ALL PASS

- [ ] **Step 2: Run all frontend type checks**

Run: `cd web && npx tsc --noEmit`

Expected: No errors.

- [ ] **Step 3: Run frontend linting**

Run: `cd web && npx eslint src/ --ext .ts,.tsx 2>&1 | tail -20`

Expected: No new errors.

- [ ] **Step 4: Manual smoke test checklist**

Verify these scenarios work:

1. Click on a deletion line — comment form appears below it
2. Click on a context line — comment form appears below it
3. Click on an addition line — still works as before
4. Create a comment on a deletion line — comment card appears with correct snippet
5. Create a comment on a context line — comment card appears
6. Navigate comments with prev/next — order matches screen position
7. Delete a comment on a deletion line — comment disappears
8. Submit review with comments on all line types — all marked as submitted
9. Reload page — comments persist and appear on correct lines
10. Legacy comments (from before this change) — still render correctly on R-side lines

- [ ] **Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: address integration issues from testing (BXN-67)"
```

---

## File Structure Summary

| File | Action | Purpose |
|------|--------|---------|
| `internal/node/git/review/review.go` | Modify | New types, backward compat, side-aware staleness |
| `internal/node/git/review/review_test.go` | Modify | Tests for all backend changes |
| `internal/node/api/review.go` | Modify | Validation, ref passthrough |
| `internal/node/api/review_test.go` | Modify | Validation tests |
| `web/src/types/review.ts` | Modify | Side-aware TypeScript types |
| `web/src/components/DiffViewer/UnifiedDiff.tsx` | Modify | Side-aware indexing, all-line commenting |
| `web/src/components/DiffViewer/InlineCommentCard.tsx` | Modify | Anchor status indicators |
| `web/src/components/DiffViewer/ExpandableUnifiedDiff.tsx` | Modify | Synthetic hunk insertion |
| `web/src/components/GitPanel/CompareView.tsx` | Modify | Side-aware creation, snippet, nav, refs |
| `web/src/data/review/queries.ts` | Modify | Pass headRef/baseRef to API |
| `web/src/hooks/useExpandableDiff.ts` | Modify | INSERT_SYNTHETIC action |
| `web/src/hooks/useCommentVisibility.ts` | Create | ensureCommentVisible hook |
