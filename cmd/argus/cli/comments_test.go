package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/git/review"
)

func rc(id, file string, line int, submitted bool, body string) review.ReviewComment {
	return review.ReviewComment{
		ID:   id,
		File: file,
		Line: review.LineRange{
			From: review.DiffPosition{Side: review.DiffSideRight, Line: line},
			To:   review.DiffPosition{Side: review.DiffSideRight, Line: line},
		},
		Submitted: submitted,
		Body:      body,
	}
}

func TestFirstLineTruncated_FirstLineOnly(t *testing.T) {
	if got := firstLineTruncated("hello\nworld", 60); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFirstLineTruncated_Short(t *testing.T) {
	if got := firstLineTruncated("abc", 60); got != "abc" {
		t.Errorf("got %q, want %q", got, "abc")
	}
}

func TestFirstLineTruncated_Truncates(t *testing.T) {
	got := firstLineTruncated(strings.Repeat("x", 70), 60)
	if n := len([]rune(got)); n != 60 {
		t.Errorf("rune length = %d, want 60", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("got %q, want an ellipsis suffix", got)
	}
}

func TestCommentsTable_Rows(t *testing.T) {
	out := commentsTable([]review.ReviewComment{
		rc("c1", "a.go", 52, true, "first line\nsecond line"),
		rc("c2", "b.go", 10, false, "short"),
	})
	for _, want := range []string{"c1", "a.go:52", "yes", "first line", "c2", "b.go:10", "no", "short"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "second line") {
		t.Errorf("table should only show the first body line; got:\n%s", out)
	}
}

func TestCommentsTable_Empty(t *testing.T) {
	if out := commentsTable(nil); !strings.Contains(out, "No review comments") {
		t.Errorf("got %q, want a no-comments message", out)
	}
}

// TestCommentsLsCmd_EmptyReview drives `git comments ls` end-to-end from inside
// a real git repo against a fake node, with --base supplied to skip the
// default-base lookup. With no review file on disk it exercises
// resolveReviewContext -> loadLocalReview -> commentsTable and prints the
// no-comments message.
func TestCommentsLsCmd_EmptyReview(t *testing.T) {
	repo := initWtGitRepo(t)
	t.Chdir(repo)

	var gotStatusPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/node/git/status" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotStatusPath = r.URL.Query().Get("path")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": map[string]any{"branch": "feature-x"},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	writeTestDiscovery(t, dir, os.Getpid(), strings.TrimPrefix(srv.URL, "http://"))

	out := captureStdout(t, func() {
		cmd := newCommentsLsCmd()
		cmd.SetArgs([]string{"--base", "main"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(out, "No review comments") {
		t.Errorf("output = %q, want a no-comments message", out)
	}
	if gotStatusPath == "" {
		t.Error("git status request missing path param")
	}
}

func TestCommentsTable_LeftSideRenamedUsesOldPath(t *testing.T) {
	// A left-side comment on a renamed file anchors to a line in the OLD file,
	// so the location must show OldPath, not the new File path.
	c := review.ReviewComment{
		ID:      "c1",
		File:    "new/path.go",
		OldPath: "old/path.go",
		Line: review.LineRange{
			From: review.DiffPosition{Side: review.DiffSideLeft, Line: 20},
			To:   review.DiffPosition{Side: review.DiffSideLeft, Line: 20},
		},
		Submitted: true,
		Body:      "deleted line note",
	}
	out := commentsTable([]review.ReviewComment{c})
	if !strings.Contains(out, "old/path.go:20") {
		t.Errorf("L-side renamed comment should locate at the old path; got:\n%s", out)
	}
	if strings.Contains(out, "new/path.go:20") {
		t.Errorf("L-side renamed comment should not use the new path; got:\n%s", out)
	}
}
