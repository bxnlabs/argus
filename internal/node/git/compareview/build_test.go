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

func TestBuild_CaseB_ContextHunkForChangedFile(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := initTestRepo(t)

	// 30-line file at base; head changes line 30 only.
	var baseLines []string
	for i := 1; i <= 30; i++ {
		baseLines = append(baseLines, "L"+itoa(i))
	}
	os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte(strings.Join(baseLines, "\n")+"\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "base")
	baseRef := runCmdOutput(t, repoDir, "git", "rev-parse", "HEAD")
	runCmd(t, repoDir, "git", "checkout", "-q", "-b", "feat")

	headLines := append([]string{}, baseLines...)
	headLines[29] = "L30-changed"
	os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte(strings.Join(headLines, "\n")+"\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "head")
	_ = baseRef

	// Save a submitted comment anchored at line 5 (R-side) — well outside the
	// hunk around line 30 that `git diff -U3` will produce.
	r := &review.Review{
		Head: "feat", Base: "main",
		Comments: []review.ReviewComment{{
			ID:        "rc_far",
			File:      "f.txt",
			Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 5}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 5}},
			Snippet:   "L5",
			Submitted: true,
			CreatedAt: "2026-01-01T00:00:00Z",
		}},
	}
	if err := review.Save(projectDir, r); err != nil {
		t.Fatalf("Save: %v", err)
	}

	v, err := Build(projectDir, repoDir, "feat", "main")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(v.Files) != 1 {
		t.Fatalf("files: %+v", v.Files)
	}
	f := v.Files[0]
	// Expect: the diff hunk around line 30, plus a context hunk around line 5.
	var diffHunks, ctxHunks int
	for _, h := range f.Hunks {
		switch h.Kind {
		case HunkKindDiff:
			diffHunks++
		case HunkKindContext:
			ctxHunks++
			// Must carry the comment id.
			if len(h.AnchorCommentIDs) != 1 || h.AnchorCommentIDs[0] != "rc_far" {
				t.Errorf("context hunk anchorCommentIds = %v, want [rc_far]", h.AnchorCommentIDs)
			}
			// Must cover line 5 (anchor) on R-side.
			var covers bool
			for _, ln := range h.Lines {
				if ln.NewLineNumber != nil && *ln.NewLineNumber == 5 {
					covers = true
				}
			}
			if !covers {
				t.Errorf("context hunk does not cover anchor line 5: %+v", h.Lines)
			}
		}
	}
	if diffHunks != 1 || ctxHunks != 1 {
		t.Errorf("expected 1 diff + 1 context hunk; got %d / %d (all hunks: %+v)", diffHunks, ctxHunks, f.Hunks)
	}
	// Hunks must be in file order by NewStart.
	for i := 1; i < len(f.Hunks); i++ {
		if f.Hunks[i].NewStart < f.Hunks[i-1].NewStart {
			t.Errorf("hunks out of order: %d < %d at index %d", f.Hunks[i].NewStart, f.Hunks[i-1].NewStart, i)
		}
	}
}

func TestBuild_CaseA_FileViewForUnchangedFile(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := initTestRepo(t)

	// Both files exist at base; only "changed.txt" is modified on the branch.
	os.WriteFile(filepath.Join(repoDir, "changed.txt"), []byte("a\nb\nc\n"), 0o644)
	os.WriteFile(filepath.Join(repoDir, "stable.txt"), []byte("x\ny\nz\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "base")
	runCmd(t, repoDir, "git", "checkout", "-q", "-b", "feat")

	os.WriteFile(filepath.Join(repoDir, "changed.txt"), []byte("a\nB\nc\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "head")

	r := &review.Review{
		Head: "feat", Base: "main",
		Comments: []review.ReviewComment{{
			ID:        "rc_stable",
			File:      "stable.txt",
			Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 2}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 2}},
			Snippet:   "y",
			Submitted: true,
			CreatedAt: "2026-01-01T00:00:00Z",
		}},
	}
	if err := review.Save(projectDir, r); err != nil {
		t.Fatalf("Save: %v", err)
	}

	v, err := Build(projectDir, repoDir, "feat", "main")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(v.Files) != 2 {
		t.Fatalf("expected 2 files (changed + synthetic stable), got %d: %+v", len(v.Files), v.Files)
	}
	// changed.txt first (real diff), then stable.txt (synthetic).
	if v.Files[0].Path != "changed.txt" || v.Files[0].Status != git.StatusModified {
		t.Errorf("file 0: %+v", v.Files[0])
	}
	if v.Files[1].Path != "stable.txt" || v.Files[1].Status != git.StatusContext {
		t.Errorf("file 1: %+v", v.Files[1])
	}
	if len(v.Files[1].Hunks) != 1 || v.Files[1].Hunks[0].Kind != HunkKindContext {
		t.Errorf("stable.txt hunks: %+v", v.Files[1].Hunks)
	}
	if len(v.Files[1].Hunks[0].AnchorCommentIDs) != 1 || v.Files[1].Hunks[0].AnchorCommentIDs[0] != "rc_stable" {
		t.Errorf("stable.txt anchorCommentIds: %v", v.Files[1].Hunks[0].AnchorCommentIDs)
	}
}

func TestBuild_CaseA_MultipleCommentsOnSameUnchangedFile(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := initTestRepo(t)

	// stable.txt is 10 lines; changed.txt provides the real diff so we're in case-(a).
	os.WriteFile(filepath.Join(repoDir, "stable.txt"),
		[]byte("L1\nL2\nL3\nL4\nL5\nL6\nL7\nL8\nL9\nL10\n"), 0o644)
	os.WriteFile(filepath.Join(repoDir, "changed.txt"), []byte("a\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "base")
	runCmd(t, repoDir, "git", "checkout", "-q", "-b", "feat")
	os.WriteFile(filepath.Join(repoDir, "changed.txt"), []byte("aa\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "head")

	// Two comments on the unchanged file at lines 2 and 3 — within each other's
	// ±3 context window. Both IDs must be preserved.
	r := &review.Review{
		Head: "feat", Base: "main",
		Comments: []review.ReviewComment{
			{
				ID: "rc_near_a", File: "stable.txt",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 2}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 2}},
				Snippet:   "L2",
				Submitted: true, CreatedAt: "2026-01-01T00:00:00Z",
			},
			{
				ID: "rc_near_b", File: "stable.txt",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 3}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 3}},
				Snippet:   "L3",
				Submitted: true, CreatedAt: "2026-01-01T00:00:00Z",
			},
		},
	}
	if err := review.Save(projectDir, r); err != nil {
		t.Fatalf("Save: %v", err)
	}

	v, err := Build(projectDir, repoDir, "feat", "main")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Find the synthetic FileView.
	var synth *FileView
	for i := range v.Files {
		if v.Files[i].Path == "stable.txt" && v.Files[i].Status == git.StatusContext {
			synth = &v.Files[i]
			break
		}
	}
	if synth == nil {
		t.Fatalf("no synthetic FileView for stable.txt in: %+v", v.Files)
	}
	// Both comments must be represented across the FileView's hunks.
	got := map[string]bool{}
	for _, h := range synth.Hunks {
		for _, id := range h.AnchorCommentIDs {
			got[id] = true
		}
	}
	for _, want := range []string{"rc_near_a", "rc_near_b"} {
		if !got[want] {
			t.Errorf("missing %q in AnchorCommentIDs across hunks (got %v)", want, got)
		}
	}
}

func TestBuild_CaseD_SnippetFallbackWhenFileMissingAtRef(t *testing.T) {
	projectDir := t.TempDir()
	repoDir := initTestRepo(t)

	// File exists only on base; deleted on head.
	os.WriteFile(filepath.Join(repoDir, "gone.txt"), []byte("hello\nworld\n"), 0o644)
	os.WriteFile(filepath.Join(repoDir, "other.txt"), []byte("a\n"), 0o644)
	runCmd(t, repoDir, "git", "add", ".")
	runCmd(t, repoDir, "git", "commit", "-m", "base")
	runCmd(t, repoDir, "git", "checkout", "-q", "-b", "feat")

	os.Remove(filepath.Join(repoDir, "gone.txt"))
	os.WriteFile(filepath.Join(repoDir, "other.txt"), []byte("aa\n"), 0o644)
	runCmd(t, repoDir, "git", "add", "-A")
	runCmd(t, repoDir, "git", "commit", "-m", "head")

	// Comment on the deleted file, R-side (so backend looks for the file at headRef, which is gone).
	r := &review.Review{
		Head: "feat", Base: "main",
		Comments: []review.ReviewComment{{
			ID:        "rc_gone",
			File:      "gone.txt",
			Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 1}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 1}},
			Snippet:   "captured snippet text",
			Submitted: true,
			CreatedAt: "2026-01-01T00:00:00Z",
		}},
	}
	if err := review.Save(projectDir, r); err != nil {
		t.Fatalf("Save: %v", err)
	}

	v, err := Build(projectDir, repoDir, "feat", "main")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Find the synthetic file for gone.txt.
	var target *FileView
	for i := range v.Files {
		if v.Files[i].Path == "gone.txt" && v.Files[i].Status == git.StatusContext {
			target = &v.Files[i]
			break
		}
	}
	if target == nil {
		t.Fatalf("no synthetic FileView for gone.txt in: %+v", v.Files)
	}
	if len(target.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(target.Hunks))
	}
	h := target.Hunks[0]
	if h.Kind != HunkKindSnippet {
		t.Errorf("kind = %q, want %q", h.Kind, HunkKindSnippet)
	}
	if !h.AnchorMissing {
		t.Error("expected AnchorMissing=true")
	}
	if len(h.AnchorCommentIDs) != 1 || h.AnchorCommentIDs[0] != "rc_gone" {
		t.Errorf("anchorCommentIds: %v", h.AnchorCommentIDs)
	}
	if len(h.Lines) != 1 || h.Lines[0].Content != "captured snippet text" {
		t.Errorf("hunk lines: %+v", h.Lines)
	}
}

// itoa: tiny dependency-free int-to-string helper for tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
