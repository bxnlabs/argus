package cli

import (
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
