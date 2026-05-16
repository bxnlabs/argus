package compareview

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/git"
	"github.com/bxnlabs/argus/internal/node/git/review"
)

// Helpers copied from review_test.go to keep this package self-contained.

func runCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
}

func runCmdOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return strings.TrimSpace(string(out))
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runCmd(t, dir, "git", "init", "-q", "-b", "main")
	runCmd(t, dir, "git", "config", "user.email", "test@example.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")
	return dir
}

// TestBuild_NoReview verifies Build returns a View identical in shape to
// what the frontend needs even when no review file exists.
func TestBuild_NoReview(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := initTestRepo(t)

	os.WriteFile(filepath.Join(repoDir, "a.txt"), []byte("line1\nline2\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "base")
	runCmd(t, repoDir, "git", "checkout", "-q", "-b", "feat")

	os.WriteFile(filepath.Join(repoDir, "a.txt"), []byte("line1\nLINE2\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "head")

	v, err := Build(projectDir, repoDir, "feat", "main")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if v == nil {
		t.Fatal("nil view")
	}
	if len(v.Files) != 1 || v.Files[0].Path != "a.txt" {
		t.Fatalf("files: %+v", v.Files)
	}
	if len(v.Files[0].Hunks) != 1 || v.Files[0].Hunks[0].Kind != HunkKindDiff {
		t.Errorf("expected one diff hunk, got %+v", v.Files[0].Hunks)
	}
	if len(v.Review.Comments) != 0 {
		t.Errorf("expected no comments, got %d", len(v.Review.Comments))
	}
	if v.HeadRef == "" || v.BaseRef == "" {
		t.Errorf("missing refs: head=%q base=%q", v.HeadRef, v.BaseRef)
	}
	// Smoke-check totals propagated from CompareResult.
	if v.TotalAdditions == 0 && v.TotalDeletions == 0 {
		t.Errorf("expected non-zero totals; got %d/%d", v.TotalAdditions, v.TotalDeletions)
	}
	// Status must round-trip from git package.
	if v.Files[0].Status != git.StatusModified {
		t.Errorf("status: %q", v.Files[0].Status)
	}
	// ReviewPayload defaults populate Head/Base from the branch arguments.
	if v.Review.Head != "feat" || v.Review.Base != "main" {
		t.Errorf("review head/base: %q/%q", v.Review.Head, v.Review.Base)
	}
}

func TestIsCommentHostedByHunk_RSideMatch(t *testing.T) {
	h := Hunk{
		OldStart: 10, OldCount: 3, NewStart: 10, NewCount: 4,
		Lines: []HunkLine{
			{Type: "context", OldLineNumber: intp(10), NewLineNumber: intp(10)},
			{Type: "addition", NewLineNumber: intp(11)},
			{Type: "context", OldLineNumber: intp(11), NewLineNumber: intp(12)},
		},
	}
	c := review.ReviewComment{
		Line: review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 11}},
	}
	if !isCommentHostedByHunk(c, h) {
		t.Error("expected hosted")
	}
}

func TestIsCommentHostedByHunk_LSideMatch(t *testing.T) {
	h := Hunk{
		Lines: []HunkLine{
			{Type: "deletion", OldLineNumber: intp(5)},
		},
	}
	c := review.ReviewComment{
		Line: review.LineRange{From: review.DiffPosition{Side: review.DiffSideLeft, Line: 5}},
	}
	if !isCommentHostedByHunk(c, h) {
		t.Error("expected hosted")
	}
}

func TestIsCommentHostedByHunk_NotHosted(t *testing.T) {
	h := Hunk{
		Lines: []HunkLine{
			{Type: "context", OldLineNumber: intp(10), NewLineNumber: intp(10)},
		},
	}
	c := review.ReviewComment{
		Line: review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 99}},
	}
	if isCommentHostedByHunk(c, h) {
		t.Error("expected not hosted")
	}
}
