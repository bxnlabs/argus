package review

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runCmd runs a command in dir, failing the test on error.
func runCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %v failed: %v\n%s", args, err, out)
	}
}

// runCmdOutput runs a command in dir and returns its stdout, failing the test on error.
func runCmdOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("command %v failed: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}

// initTestRepo creates a minimal git repo with a configured identity for testing.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	return dir
}

func TestEncodeBranchName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "main", want: "main"},
		{name: "slash", input: "feat/auth-system", want: "feat_auth-_system"},
		{name: "underscore preserved", input: "my_branch", want: "my__branch"},
		{name: "slash and underscore", input: "feat/my_branch", want: "feat_my__branch"},
		{name: "multiple slashes", input: "feat/sub/deep", want: "feat_sub_deep"},
		{name: "leading underscore", input: "_private", want: "__private"},
		{name: "double hyphen", input: "a--b", want: "a-_-_b"},
		{name: "hyphen", input: "feat-auth", want: "feat-_auth"},
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

func TestReviewFilename(t *testing.T) {
	got := reviewFilename("feat/auth-system", "main")
	want := "feat_auth-_system--main.json"
	if got != want {
		t.Errorf("reviewFilename() = %q, want %q", got, want)
	}
}

func TestReviewFilename_NoCollision(t *testing.T) {
	// ("a--b", "c") and ("a", "b--c") must not collide.
	f1 := reviewFilename("a--b", "c")
	f2 := reviewFilename("a", "b--c")
	if f1 == f2 {
		t.Errorf("collision: reviewFilename(%q,%q) == reviewFilename(%q,%q) == %q", "a--b", "c", "a", "b--c", f1)
	}
}

func TestWriteAndReadReviewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := &Review{
		Head: "feat/auth", Base: "main",
		Comments: []ReviewComment{{
			ID: "rc_123_abc", File: "src/auth.ts",
			Line: LineRange{
				From: DiffPosition{Side: DiffSideRight, Line: 10},
				To:   DiffPosition{Side: DiffSideRight, Line: 12},
			}, Snippet: "const x = 1;",
			Body: "Change this", Submitted: false, CreatedAt: "2026-03-16T10:30:00Z",
		}},
	}
	if err := writeReviewFile(path, data); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := readReviewFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Head != "feat/auth" {
		t.Errorf("head = %q, want %q", got.Head, "feat/auth")
	}
	if len(got.Comments) != 1 {
		t.Fatalf("comments len = %d, want 1", len(got.Comments))
	}
	if got.Comments[0].Body != "Change this" {
		t.Errorf("body = %q, want %q", got.Comments[0].Body, "Change this")
	}
}

func TestReadReviewFile_NotExist(t *testing.T) {
	got, err := readReviewFile("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %+v", got)
	}
}

func TestDetectStaleness_SingleMatch(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "auth.ts"), []byte("line1\nline2\nconst TOKEN = 1800;\nline4\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	comments := []ReviewComment{{
		ID: "rc_1", File: "src/auth.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 10},
			To:   DiffPosition{Side: DiffSideRight, Line: 10},
		}, Snippet: "const TOKEN = 1800;",
		Body: "Change to 3600", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].Line.From.Line != 3 || result[0].Line.To.Line != 3 {
		t.Errorf("expected line 3, got %d-%d", result[0].Line.From.Line, result[0].Line.To.Line)
	}
}

func TestDetectStaleness_NoMatch(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	os.WriteFile(filepath.Join(dir, "src", "auth.ts"), []byte("completely different content\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	comments := []ReviewComment{{
		ID: "rc_1", File: "src/auth.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 5},
			To:   DiffPosition{Side: DiffSideRight, Line: 5},
		}, Snippet: "const TOKEN = 1800;",
		Body: "Change to 3600", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (pruned), got %d", len(result))
	}
}

func TestDetectStaleness_FileDeleted(t *testing.T) {
	dir := initTestRepo(t)
	// Commit a placeholder file so the repo is valid, but src/deleted.ts is not committed.
	os.WriteFile(filepath.Join(dir, "README"), []byte("placeholder\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	comments := []ReviewComment{{
		ID: "rc_1", File: "src/deleted.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 1},
			To:   DiffPosition{Side: DiffSideRight, Line: 1},
		}, Snippet: "anything",
		Body: "Comment on deleted file", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (file deleted), got %d", len(result))
	}
}

func TestDetectStaleness_MultipleMatches_NearestWins(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	var lines []string
	for i := 1; i <= 25; i++ {
		if i == 5 || i == 20 {
			lines = append(lines, "return true;")
		} else {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	os.WriteFile(filepath.Join(dir, "src", "util.ts"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	comments := []ReviewComment{{
		ID: "rc_1", File: "src/util.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 6},
			To:   DiffPosition{Side: DiffSideRight, Line: 6},
		}, Snippet: "return true;",
		Body: "Should return false", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].Line.From.Line != 5 {
		t.Errorf("expected nearest match line 5, got %d", result[0].Line.From.Line)
	}
}

func TestDetectStaleness_SkipsUnsubmitted(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "README"), []byte("placeholder\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	comments := []ReviewComment{{
		ID: "rc_1", File: "src/nonexistent.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 1},
			To:   DiffPosition{Side: DiffSideRight, Line: 1},
		}, Snippet: "anything",
		Body: "Draft comment", Submitted: false,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (draft preserved), got %d", len(result))
	}
}

func TestDetectStaleness_PathTraversal(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "README"), []byte("placeholder\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")
	comments := []ReviewComment{{
		ID: "rc_1", File: "../etc/passwd",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 1},
			To:   DiffPosition{Side: DiffSideRight, Line: 1},
		}, Snippet: "root:x:0:0",
		Body: "Traversal attempt", Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (path traversal rejected), got %d", len(result))
	}
}

func TestFindSnippet_SingleMatch_ReAnchors(t *testing.T) {
	// Single match on line 100, prior line was 1 — should re-anchor since it's unambiguous.
	var lines []string
	for i := 1; i <= 100; i++ {
		if i == 100 {
			lines = append(lines, "target line")
		} else {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	fileText := strings.Join(lines, "\n") + "\n"
	result := findSnippet(fileText, "target line", 1)
	if result != 100 {
		t.Errorf("expected 100 for single unambiguous match, got %d", result)
	}
}

func TestLoadSaveDelete(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := initTestRepo(t)
	os.MkdirAll(filepath.Join(repoDir, "src"), 0o755)
	os.WriteFile(
		filepath.Join(repoDir, "src", "auth.ts"),
		[]byte("line1\nconst TOKEN = 1800;\nline3\n"),
		0o644,
	)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, repoDir, "git", "rev-parse", "HEAD")
	r := &Review{
		Head: "feat/auth", Base: "main",
		Comments: []ReviewComment{{
			ID: "rc_1", File: "src/auth.ts",
			Line: LineRange{
				From: DiffPosition{Side: DiffSideRight, Line: 2},
				To:   DiffPosition{Side: DiffSideRight, Line: 2},
			}, Snippet: "const TOKEN = 1800;",
			Body: "Change to 3600", Submitted: true,
		}},
	}
	if err := Save(projectDir, r); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(projectDir, repoDir, "feat/auth", "main", headRef, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded data")
	}
	if len(loaded.Comments) != 1 {
		t.Fatalf("expected 1 comment after staleness, got %d", len(loaded.Comments))
	}
	if err := Delete(projectDir, "feat/auth", "main"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	loaded, err = Load(projectDir, repoDir, "feat/auth", "main", headRef, "")
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after delete, got %+v", loaded)
	}
}

func TestLineRange_UnmarshalJSON_NewFormat(t *testing.T) {
	input := `{"from":{"side":"L","line":10},"to":{"side":"L","line":10}}`
	var lr LineRange
	if err := json.Unmarshal([]byte(input), &lr); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if lr.From.Side != DiffSideLeft {
		t.Errorf("From.Side = %q, want %q", lr.From.Side, DiffSideLeft)
	}
	if lr.From.Line != 10 {
		t.Errorf("From.Line = %d, want 10", lr.From.Line)
	}
	if lr.To.Side != DiffSideLeft {
		t.Errorf("To.Side = %q, want %q", lr.To.Side, DiffSideLeft)
	}
	if lr.To.Line != 10 {
		t.Errorf("To.Line = %d, want 10", lr.To.Line)
	}
}

func TestLineRange_UnmarshalJSON_LegacyFormat(t *testing.T) {
	input := `{"from":5,"to":5}`
	var lr LineRange
	if err := json.Unmarshal([]byte(input), &lr); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if lr.From.Side != DiffSideRight {
		t.Errorf("From.Side = %q, want %q (legacy migrates to R)", lr.From.Side, DiffSideRight)
	}
	if lr.From.Line != 5 {
		t.Errorf("From.Line = %d, want 5", lr.From.Line)
	}
	if lr.To.Side != DiffSideRight {
		t.Errorf("To.Side = %q, want %q (legacy migrates to R)", lr.To.Side, DiffSideRight)
	}
	if lr.To.Line != 5 {
		t.Errorf("To.Line = %d, want 5", lr.To.Line)
	}
}

func TestLineRange_MarshalJSON(t *testing.T) {
	lr := LineRange{
		From: DiffPosition{Side: DiffSideRight, Line: 7},
		To:   DiffPosition{Side: DiffSideRight, Line: 9},
	}
	data, err := json.Marshal(lr)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	// Verify it round-trips correctly as the new format.
	var got LineRange
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("round-trip UnmarshalJSON error: %v", err)
	}
	if got.From.Side != DiffSideRight || got.From.Line != 7 {
		t.Errorf("From = {%q,%d}, want {%q,7}", got.From.Side, got.From.Line, DiffSideRight)
	}
	if got.To.Side != DiffSideRight || got.To.Line != 9 {
		t.Errorf("To = {%q,%d}, want {%q,9}", got.To.Side, got.To.Line, DiffSideRight)
	}
	// Verify the serialized form contains the side field (new format, not legacy integers).
	if !strings.Contains(string(data), `"side"`) {
		t.Errorf("marshaled form should contain side field; got %s", data)
	}
}

func TestReviewComment_NewFields(t *testing.T) {
	c := ReviewComment{
		ID:             "rc_1",
		File:           "src/new.ts",
		OldPath:        "src/old.ts",
		Line:           LineRange{From: DiffPosition{Side: DiffSideLeft, Line: 3}, To: DiffPosition{Side: DiffSideLeft, Line: 3}},
		Snippet:        "deleted line",
		SnippetContext: "surrounding context",
		Body:           "Why was this removed?",
		Submitted:      true,
		CreatedAt:      "2026-04-01T12:00:00Z",
		AnchorStatus:   AnchorStale,
	}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}
	var got ReviewComment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("UnmarshalJSON error: %v", err)
	}
	if got.OldPath != "src/old.ts" {
		t.Errorf("OldPath = %q, want %q", got.OldPath, "src/old.ts")
	}
	if got.SnippetContext != "surrounding context" {
		t.Errorf("SnippetContext = %q, want %q", got.SnippetContext, "surrounding context")
	}
	if got.AnchorStatus != AnchorStale {
		t.Errorf("AnchorStatus = %q, want %q", got.AnchorStatus, AnchorStale)
	}
	if got.Line.From.Side != DiffSideLeft || got.Line.From.Line != 3 {
		t.Errorf("Line.From = {%q,%d}, want {%q,3}", got.Line.From.Side, got.Line.From.Line, DiffSideLeft)
	}
}

func TestLegacyReviewFile_Migration(t *testing.T) {
	// Simulate a review file written in the old format.
	legacyJSON := `{
		"head": "feat/old",
		"base": "main",
		"comments": [
			{
				"id": "rc_legacy",
				"file": "src/foo.ts",
				"line": {"from": 8, "to": 10},
				"snippet": "old code",
				"body": "review comment",
				"submitted": false,
				"createdAt": "2025-01-01T00:00:00Z"
			}
		]
	}`
	dir := t.TempDir()
	path := filepath.Join(dir, "review.json")
	if err := os.WriteFile(path, []byte(legacyJSON), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	r, err := readReviewFile(path)
	if err != nil {
		t.Fatalf("readReviewFile: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil review")
	}
	if len(r.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(r.Comments))
	}
	c := r.Comments[0]
	if c.Line.From.Side != DiffSideRight {
		t.Errorf("legacy From.Side = %q, want %q", c.Line.From.Side, DiffSideRight)
	}
	if c.Line.From.Line != 8 {
		t.Errorf("legacy From.Line = %d, want 8", c.Line.From.Line)
	}
	if c.Line.To.Side != DiffSideRight {
		t.Errorf("legacy To.Side = %q, want %q", c.Line.To.Side, DiffSideRight)
	}
	if c.Line.To.Line != 10 {
		t.Errorf("legacy To.Line = %d, want 10", c.Line.To.Line)
	}
}

// TestDetectStaleness_RSideUsesHeadRef verifies that R-side comments are re-anchored
// against headRef (the HEAD commit), not the working tree.
func TestDetectStaleness_RSideUsesHeadRef(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)
	// Commit a file at headRef with the snippet at line 3.
	os.WriteFile(filepath.Join(dir, "src", "app.ts"), []byte("line1\nline2\nconst X = 42;\nline4\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "head commit")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	// Modify the working tree — the implementation must NOT read from disk.
	os.WriteFile(filepath.Join(dir, "src", "app.ts"), []byte("completely different content\n"), 0o644)

	comments := []ReviewComment{{
		ID: "rc_r", File: "src/app.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 10},
			To:   DiffPosition{Side: DiffSideRight, Line: 10},
		},
		Snippet:   "const X = 42;",
		Body:      "R-side comment",
		Submitted: true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (re-anchored via headRef), got %d", len(result))
	}
	if result[0].Line.From.Side != DiffSideRight {
		t.Errorf("side = %q, want R", result[0].Line.From.Side)
	}
	if result[0].Line.From.Line != 3 {
		t.Errorf("line = %d, want 3", result[0].Line.From.Line)
	}
}

// TestDetectStaleness_LSideUsesBaseRef verifies that L-side comments are re-anchored
// against baseRef (the merge-base commit), not headRef or the working tree.
func TestDetectStaleness_LSideUsesBaseRef(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// Create the base commit with a file containing the snippet at line 2.
	os.WriteFile(filepath.Join(dir, "src", "old.ts"), []byte("line1\ndeleted line here\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "base commit")
	baseRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	// Create a subsequent head commit where the file is changed.
	os.WriteFile(filepath.Join(dir, "src", "old.ts"), []byte("line1\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "head commit")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	comments := []ReviewComment{{
		ID: "rc_l", File: "src/old.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideLeft, Line: 5},
			To:   DiffPosition{Side: DiffSideLeft, Line: 5},
		},
		Snippet:   "deleted line here",
		Body:      "L-side comment on deleted line",
		Submitted: true,
	}}
	result := detectStaleness(dir, headRef, baseRef, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (re-anchored via baseRef), got %d", len(result))
	}
	if result[0].Line.From.Side != DiffSideLeft {
		t.Errorf("side = %q, want L", result[0].Line.From.Side)
	}
	if result[0].Line.From.Line != 2 {
		t.Errorf("line = %d, want 2", result[0].Line.From.Line)
	}
}

// TestDetectStaleness_LSideRenamedFile verifies that L-side comments with OldPath
// use OldPath (not File) to look up content against baseRef.
func TestDetectStaleness_LSideRenamedFile(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// Base commit: file exists at the old path.
	os.WriteFile(filepath.Join(dir, "src", "old_name.ts"), []byte("line1\nfunc oldLogic() {}\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "base commit with old path")
	baseRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	// Head commit: file is renamed to new_name.ts.
	runCmd(t, dir, "git", "mv", "src/old_name.ts", "src/new_name.ts")
	runCmd(t, dir, "git", "commit", "-m", "rename file")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	comments := []ReviewComment{{
		ID:      "rc_renamed",
		File:    "src/new_name.ts",
		OldPath: "src/old_name.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideLeft, Line: 10},
			To:   DiffPosition{Side: DiffSideLeft, Line: 10},
		},
		Snippet:   "func oldLogic() {}",
		Body:      "L-side comment on renamed file",
		Submitted: true,
	}}
	result := detectStaleness(dir, headRef, baseRef, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (re-anchored via baseRef+OldPath), got %d", len(result))
	}
	if result[0].Line.From.Side != DiffSideLeft {
		t.Errorf("side = %q, want L", result[0].Line.From.Side)
	}
	if result[0].Line.From.Line != 2 {
		t.Errorf("line = %d, want 2", result[0].Line.From.Line)
	}
}

// TestDetectStaleness_LSideRestoredLine verifies that an L-side comment becomes
// stale when the deleted line is restored in the head ref.
func TestDetectStaleness_LSideRestoredLine(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// Base commit: file has a comment line.
	os.WriteFile(filepath.Join(dir, "src", "utils.ts"), []byte("line1\n// important comment\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "base commit")
	baseRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	// Head commit 1: remove the comment line.
	os.WriteFile(filepath.Join(dir, "src", "utils.ts"), []byte("line1\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "remove comment line")

	// Head commit 2: restore the comment line.
	os.WriteFile(filepath.Join(dir, "src", "utils.ts"), []byte("line1\n// important comment\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "restore comment line")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	comments := []ReviewComment{{
		ID:   "rc_restored",
		File: "src/utils.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideLeft, Line: 2},
			To:   DiffPosition{Side: DiffSideLeft, Line: 2},
		},
		Snippet:   "// important comment",
		Body:      "Don't remove this comment",
		Submitted: true,
	}}
	result := detectStaleness(dir, headRef, baseRef, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].AnchorStatus != AnchorStale {
		t.Errorf("anchorStatus = %q, want %q — L-side comment should be stale when deleted line is restored in head", result[0].AnchorStatus, AnchorStale)
	}
}

// TestDetectStaleness_LSideRenamedFileRestoredLine verifies that an L-side
// comment on a renamed file becomes stale when the deleted line is restored.
func TestDetectStaleness_LSideRenamedFileRestoredLine(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// Base commit: file at old path with a comment line.
	os.WriteFile(filepath.Join(dir, "src", "old_name.ts"), []byte("line1\n// important comment\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "base commit")
	baseRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	// Head commit 1: rename file and remove the comment line.
	runCmd(t, dir, "git", "mv", "src/old_name.ts", "src/new_name.ts")
	os.WriteFile(filepath.Join(dir, "src", "new_name.ts"), []byte("line1\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "rename and remove line")

	// Head commit 2: restore the comment line.
	os.WriteFile(filepath.Join(dir, "src", "new_name.ts"), []byte("line1\n// important comment\nline3\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "restore comment line")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	comments := []ReviewComment{{
		ID:      "rc_renamed_restored",
		File:    "src/new_name.ts",
		OldPath: "src/old_name.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideLeft, Line: 2},
			To:   DiffPosition{Side: DiffSideLeft, Line: 2},
		},
		Snippet:   "// important comment",
		Body:      "Don't remove this comment",
		Submitted: true,
	}}
	result := detectStaleness(dir, headRef, baseRef, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(result))
	}
	if result[0].AnchorStatus != AnchorStale {
		t.Errorf("anchorStatus = %q, want %q — L-side comment on renamed file should be stale when deleted line is restored in head", result[0].AnchorStatus, AnchorStale)
	}
}

// TestDetectStaleness_StaleWhenAmbiguous verifies that multiple snippet matches combined
// with a non-matching snippetContext results in anchorStatus=stale.
func TestDetectStaleness_StaleWhenAmbiguous(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// Build a file where "return true;" appears at lines 5 and 20,
	// but the snippetContext matches neither location.
	var lines []string
	for i := 1; i <= 25; i++ {
		if i == 5 || i == 20 {
			lines = append(lines, "return true;")
		} else {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	os.WriteFile(filepath.Join(dir, "src", "svc.ts"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	comments := []ReviewComment{{
		ID: "rc_ambig", File: "src/svc.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 5},
			To:   DiffPosition{Side: DiffSideRight, Line: 5},
		},
		Snippet:        "return true;",
		SnippetContext: "this context does not appear anywhere in the file",
		Body:           "Ambiguous match",
		Submitted:      true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (stale, not pruned), got %d", len(result))
	}
	if result[0].AnchorStatus != AnchorStale {
		t.Errorf("anchorStatus = %q, want %q", result[0].AnchorStatus, AnchorStale)
	}
}

// TestDetectStaleness_ContextDisambiguatesMatch verifies that when a snippet appears
// at multiple locations, a snippetContext that uniquely identifies one of them causes
// the comment to be re-anchored to that specific location (not the nearest one).
func TestDetectStaleness_ContextDisambiguatesMatch(t *testing.T) {
	dir := initTestRepo(t)
	os.MkdirAll(filepath.Join(dir, "src"), 0o755)

	// Build a file where "const x = 1;" appears at lines 2 and 30.
	// A unique marker is placed at line 25 — within ±10 of line 30 but
	// more than 10 lines away from line 2, so only line 30's window sees it.
	var lines []string
	for i := 1; i <= 35; i++ {
		switch i {
		case 2, 30:
			lines = append(lines, "const x = 1;")
		case 25:
			lines = append(lines, "// uniqueMarkerForLine30")
		default:
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	os.WriteFile(filepath.Join(dir, "src", "dup.ts"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	headRef := runCmdOutput(t, dir, "git", "rev-parse", "HEAD")

	// Comment originally at line 1 (near line 2), but snippetContext uniquely
	// identifies the occurrence at line 30 — context should win over proximity.
	comments := []ReviewComment{{
		ID: "rc_ctx", File: "src/dup.ts",
		Line: LineRange{
			From: DiffPosition{Side: DiffSideRight, Line: 1},
			To:   DiffPosition{Side: DiffSideRight, Line: 1},
		},
		Snippet:        "const x = 1;",
		SnippetContext: "uniqueMarkerForLine30",
		Body:           "Context should anchor to line 30",
		Submitted:      true,
	}}
	result := detectStaleness(dir, headRef, "", comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (re-anchored via context), got %d", len(result))
	}
	if result[0].AnchorStatus != "" {
		t.Errorf("anchorStatus = %q, want empty (healthy)", result[0].AnchorStatus)
	}
	if result[0].Line.From.Line != 30 {
		t.Errorf("line = %d, want 30 (context-disambiguated)", result[0].Line.From.Line)
	}
	if result[0].Line.From.Side != DiffSideRight {
		t.Errorf("side = %q, want R", result[0].Line.From.Side)
	}
}
