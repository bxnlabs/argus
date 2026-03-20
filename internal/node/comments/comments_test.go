package comments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestWriteAndReadCommentsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := &CommentsFile{
		Branch: "feat/auth", BaseBranch: "main",
		Comments: []Comment{{
			ID: "rc_123_abc", File: "src/auth.ts",
			Line: LineRange{From: 10, To: 12}, Snippet: "const x = 1;",
			Body: "Change this", Submitted: false, CreatedAt: "2026-03-16T10:30:00Z",
		}},
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

func TestDetectStaleness_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "src", "auth.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	os.WriteFile(filePath, []byte("line1\nline2\nconst TOKEN = 1800;\nline4\n"), 0o644)
	comments := []Comment{{
		ID: "rc_1", File: "src/auth.ts",
		Line: LineRange{From: 10, To: 10}, Snippet: "const TOKEN = 1800;",
		Body: "Change to 3600", Submitted: true,
	}}
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
	comments := []Comment{{
		ID: "rc_1", File: "src/auth.ts",
		Line: LineRange{From: 5, To: 5}, Snippet: "const TOKEN = 1800;",
		Body: "Change to 3600", Submitted: true,
	}}
	result := detectStaleness(dir, comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (pruned), got %d", len(result))
	}
}

func TestDetectStaleness_FileDeleted(t *testing.T) {
	dir := t.TempDir()
	comments := []Comment{{
		ID: "rc_1", File: "src/deleted.ts",
		Line: LineRange{From: 1, To: 1}, Snippet: "anything",
		Body: "Comment on deleted file", Submitted: true,
	}}
	result := detectStaleness(dir, comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (file deleted), got %d", len(result))
	}
}

func TestDetectStaleness_MultipleMatches_NearestWins(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "src", "util.ts")
	os.MkdirAll(filepath.Dir(filePath), 0o755)
	var lines []string
	for i := 1; i <= 25; i++ {
		if i == 5 || i == 20 {
			lines = append(lines, "return true;")
		} else {
			lines = append(lines, fmt.Sprintf("line %d", i))
		}
	}
	os.WriteFile(filePath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	comments := []Comment{{
		ID: "rc_1", File: "src/util.ts",
		Line: LineRange{From: 6, To: 6}, Snippet: "return true;",
		Body: "Should return false", Submitted: true,
	}}
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
	comments := []Comment{{
		ID: "rc_1", File: "src/nonexistent.ts",
		Line: LineRange{From: 1, To: 1}, Snippet: "anything",
		Body: "Draft comment", Submitted: false,
	}}
	result := detectStaleness(dir, comments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comment (draft preserved), got %d", len(result))
	}
}

func TestDetectStaleness_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	comments := []Comment{{
		ID: "rc_1", File: "../etc/passwd",
		Line: LineRange{From: 1, To: 1}, Snippet: "root:x:0:0",
		Body: "Traversal attempt", Submitted: true,
	}}
	result := detectStaleness(dir, comments)
	if len(result) != 0 {
		t.Fatalf("expected 0 comments (path traversal rejected), got %d", len(result))
	}
}

func TestFindSnippet_SingleMatch_TooFar(t *testing.T) {
	// Single match on line 100, prior line was 1 — distance is 99, exceeds 50
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
	if result != -1 {
		t.Errorf("expected -1 for single match too far away, got %d", result)
	}
}

func TestLoadSaveDelete(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, "src"), 0o755)
	os.WriteFile(
		filepath.Join(repoDir, "src", "auth.ts"),
		[]byte("line1\nconst TOKEN = 1800;\nline3\n"),
		0o644,
	)
	cf := &CommentsFile{
		Branch: "feat/auth", BaseBranch: "main",
		Comments: []Comment{{
			ID: "rc_1", File: "src/auth.ts",
			Line: LineRange{From: 2, To: 2}, Snippet: "const TOKEN = 1800;",
			Body: "Change to 3600", Submitted: true,
		}},
	}
	if err := Save(projectDir, cf); err != nil {
		t.Fatalf("Save: %v", err)
	}
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
	if err := Delete(projectDir, "feat/auth", "main"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	loaded, err = Load(projectDir, repoDir, "feat/auth", "main")
	if err != nil {
		t.Fatalf("Load after delete: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil after delete, got %+v", loaded)
	}
}
