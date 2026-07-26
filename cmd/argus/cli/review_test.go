package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/git/review"
)

func TestResolveReviewBase_FlagOverridesDefault(t *testing.T) {
	// When --base is provided, the helper must return it verbatim and must
	// NOT call /api/node/git/compare/branches at all (the network round-trip is
	// pure waste and may fail on stale discovery state).
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	got, err := resolveReviewBase(c, "path=/tmp/repo", "develop")
	if err != nil {
		t.Fatalf("resolveReviewBase: %v", err)
	}
	if got != "develop" {
		t.Errorf("base = %q, want %q", got, "develop")
	}
	if called {
		t.Error("server should not have been called when --base is set")
	}
}

func TestResolveReviewBase_FlagTrimsWhitespace(t *testing.T) {
	// A whitespace-padded --base value (e.g. accidental newline from a
	// shell paste) must be trimmed before being used as the lookup key.
	// Without this, `argus review get --base " main"` would key on the
	// padded string and return an empty result.
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	got, err := resolveReviewBase(c, "path=/tmp/repo", "  main\n")
	if err != nil {
		t.Fatalf("resolveReviewBase: %v", err)
	}
	if got != "main" {
		t.Errorf("base = %q, want %q", got, "main")
	}
	if called {
		t.Error("server should not have been called when --base is set")
	}
}

func TestResolveReviewBase_FallsBackToDefault(t *testing.T) {
	// With no flag, the helper must hit /api/node/git/compare/branches and use
	// whatever defaultBase the server returns.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/node/git/compare/branches" {
			t.Errorf("path = %q, want /api/node/git/compare/branches", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"defaultBase": "main"})
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	got, err := resolveReviewBase(c, "path=/tmp/repo", "")
	if err != nil {
		t.Fatalf("resolveReviewBase: %v", err)
	}
	if got != "main" {
		t.Errorf("base = %q, want %q", got, "main")
	}
}

func TestResolveReviewBase_EmptyFlagAndNoDefault(t *testing.T) {
	// If we have neither a --base flag nor a detectable default, the helper
	// must return a guidance-bearing error rather than silently passing an
	// empty string down to review.Load (which would key the file as
	// `head--.json` and look entirely broken).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"defaultBase": ""})
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	_, err := resolveReviewBase(c, "path=/tmp/repo", "")
	if err == nil {
		t.Fatal("expected error when no default base is detectable and --base is unset")
	}
	if !strings.Contains(err.Error(), "--base") {
		t.Errorf("error %q should mention --base to guide the user", err)
	}
}

func TestResolveReviewBase_HTTPError(t *testing.T) {
	// When the server returns an error, the helper must surface a wrapped
	// "get branches" error rather than silently using an empty base.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	_, err := resolveReviewBase(c, "path=/tmp/repo", "")
	if err == nil {
		t.Fatal("expected error when server returns 5xx")
	}
	if !strings.Contains(err.Error(), "get branches") {
		t.Errorf("error %q should be wrapped with %q", err, "get branches")
	}
}

func TestResolveReviewBase_MalformedJSON(t *testing.T) {
	// When the server returns invalid JSON, the helper must surface a
	// wrapped "parse branches" error so the failure mode is unambiguous.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{"))
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	_, err := resolveReviewBase(c, "path=/tmp/repo", "")
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
	if !strings.Contains(err.Error(), "parse branches") {
		t.Errorf("error %q should be wrapped with %q", err, "parse branches")
	}
}

func TestFormatReviewMarkdown(t *testing.T) {
	r := &review.Review{
		Head: "feat/auth-system",
		Base: "main",
		Body: &review.ReviewBody{
			Body:      "Auth looks mostly good, but token handling needs hardening",
			Submitted: true,
			CreatedAt: "2026-03-16T10:30:00Z",
		},
		Comments: []review.ReviewComment{
			{
				ID: "rc_1", File: "src/auth.ts",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 52}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 52}},
				Snippet:   "const TOKEN_EXPIRY = 1800;",
				Body:      "Token expiry should be 3600 not 1800",
				Submitted: true,
			},
			{
				ID: "rc_2", File: "src/auth.ts",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 12}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 15}},
				Snippet:   "function validateToken(token) {\n  if (!token) return false;\n}",
				Body:      "Missing signature check",
				Submitted: false, // Should be excluded
			},
			{
				ID: "rc_3", File: "src/utils.ts",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 1}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 3}},
				Snippet:   "import { hash } from 'crypto';",
				Body:      "Use a proper hashing library",
				Submitted: true,
			},
		},
	}

	output := formatReviewMarkdown(r)

	if !strings.Contains(output, "## Comments") {
		t.Error("missing ## Comments header")
	}
	if !strings.Contains(output, "Branch: feat/auth-system vs main") {
		t.Error("missing branch line")
	}
	if !strings.Contains(output, "Auth looks mostly good, but token handling needs hardening") {
		t.Error("missing body text")
	}
	if strings.Contains(output, "### General") {
		t.Error("should not have a ### General section")
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
	if strings.Contains(output, "Missing signature check") {
		t.Error("draft comment should not appear in output")
	}
	if !strings.Contains(output, "### src/utils.ts") {
		t.Error("missing file header for src/utils.ts")
	}
}

func TestFormatReviewMarkdown_RenamedFile(t *testing.T) {
	r := &review.Review{
		Head: "feat/refactor",
		Base: "main",
		Comments: []review.ReviewComment{
			{
				// L-side comment on a renamed file: line numbers reference the OLD file.
				ID: "rc_1", File: "benchmarking/utils.py", OldPath: "tools/ci_bench_utils.py",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideLeft, Line: 21}, To: review.DiffPosition{Side: review.DiffSideLeft, Line: 21}},
				Snippet:   "# Create the worksheet if it doesn't exist",
				Body:      "This comment should not be removed.",
				Submitted: true,
			},
			{
				// R-side comment on the same renamed file: line numbers reference the NEW file.
				ID: "rc_2", File: "benchmarking/utils.py", OldPath: "tools/ci_bench_utils.py",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 36}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 36}},
				Snippet:   "ws = sh.add_worksheet(title=worksheet, rows=100, cols=20)",
				Body:      "Add error handling here.",
				Submitted: true,
			},
		},
	}

	output := formatReviewMarkdown(r)

	// Both L-side and R-side comments on the same renamed file should be grouped
	// under a single header showing the rename.
	wantHeader := "### tools/ci_bench_utils.py \u2192 benchmarking/utils.py"
	if !strings.Contains(output, wantHeader) {
		t.Errorf("expected rename header %q; got:\n%s", wantHeader, output)
	}
	// Both comments should appear in the output.
	if !strings.Contains(output, "This comment should not be removed.") {
		t.Errorf("missing L-side comment body; got:\n%s", output)
	}
	if !strings.Contains(output, "Add error handling here.") {
		t.Errorf("missing R-side comment body; got:\n%s", output)
	}
}

func TestFormatReviewMarkdown_RenamedFilePathCollision(t *testing.T) {
	// Scenario: branch renames a.txt → b.txt, then creates a new a.txt.
	// L-side rename comment and R-side new-file comment both display as "a.txt"
	// but must NOT merge into one section.
	r := &review.Review{
		Head: "feat/collision",
		Base: "main",
		Comments: []review.ReviewComment{
			{
				// L-side comment on renamed file: displays as a.txt (old path).
				ID: "rc_rename", File: "b.txt", OldPath: "a.txt",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideLeft, Line: 5}, To: review.DiffPosition{Side: review.DiffSideLeft, Line: 5}},
				Snippet:   "old content",
				Body:      "Rename comment body",
				Submitted: true,
			},
			{
				// R-side comment on the newly created a.txt (no OldPath).
				ID: "rc_new", File: "a.txt",
				Line:      review.LineRange{From: review.DiffPosition{Side: review.DiffSideRight, Line: 10}, To: review.DiffPosition{Side: review.DiffSideRight, Line: 10}},
				Snippet:   "new content",
				Body:      "New file comment body",
				Submitted: true,
			},
		},
	}

	output := formatReviewMarkdown(r)

	// Both should appear as separate sections.
	renameHeader := "### a.txt \u2192 b.txt"
	if !strings.Contains(output, renameHeader) {
		t.Errorf("missing rename section header; got:\n%s", output)
	}
	// The new a.txt section should have NO rename annotation.
	if !strings.Contains(output, "### a.txt\n") {
		t.Errorf("missing plain a.txt section header (without rename annotation); got:\n%s", output)
	}
	// Verify comments land in the correct sections.
	renameIdx := strings.Index(output, renameHeader)
	newIdx := strings.Index(output, "### a.txt\n")
	renameBody := strings.Index(output, "Rename comment body")
	newBody := strings.Index(output, "New file comment body")
	if renameBody < renameIdx {
		t.Errorf("rename comment appeared before rename section; got:\n%s", output)
	}
	if newBody < newIdx {
		t.Errorf("new file comment appeared before new file section; got:\n%s", output)
	}
	// Comments must not be in each other's sections.
	if renameIdx < newIdx && renameBody > newIdx {
		t.Errorf("rename comment leaked into new file section; got:\n%s", output)
	}
	if newIdx < renameIdx && newBody > renameIdx {
		t.Errorf("new file comment leaked into rename section; got:\n%s", output)
	}
}

func TestFormatReviewMarkdown_Empty(t *testing.T) {
	// The empty-output message must echo the queried head/base so a typo'd
	// --base value is visible to the user instead of producing an
	// indistinguishable "no comments" result against the wrong key.
	r := &review.Review{
		Head:     "feat/test",
		Base:     "main",
		Comments: []review.ReviewComment{},
	}
	output := formatReviewMarkdown(r)
	want := "No submitted comments for feat/test vs main."
	if !strings.Contains(output, want) {
		t.Errorf("output = %q, want it to contain %q", output, want)
	}
}
