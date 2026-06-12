package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/git"
)

func TestRespondGitError(t *testing.T) {
	t.Run("ErrInvalidInput returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		respondGitError(w, fmt.Errorf("%w: bad input", git.ErrInvalidInput))
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})

	t.Run("ErrNotFound returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		respondGitError(w, fmt.Errorf("%w: missing ref", git.ErrNotFound))
		if w.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", w.Code)
		}
	})

	t.Run("unknown error returns 500", func(t *testing.T) {
		w := httptest.NewRecorder()
		respondGitError(w, fmt.Errorf("something went wrong"))
		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	})

	t.Run("unknown error does not leak message", func(t *testing.T) {
		w := httptest.NewRecorder()
		respondGitError(w, fmt.Errorf("secret internal path /etc/shadow"))
		body := w.Body.String()
		if strings.Contains(body, "/etc/shadow") {
			t.Errorf("internal error message leaked to client: %s", body)
		}
		if !strings.Contains(body, "internal error") {
			t.Errorf("expected generic 'internal error' message, got: %s", body)
		}
	})

	t.Run("ErrFetchFailed returns 502 with surfaced message", func(t *testing.T) {
		w := httptest.NewRecorder()
		// Use the real wrapping path so we exercise both Is() matching and
		// the user-facing message format Fetch produces in production.
		dir := t.TempDir()
		if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		if out, err := exec.Command("git", "-C", dir, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing")).CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
		fetchErr := git.Fetch(t.Context(), dir, "")
		if fetchErr == nil {
			t.Fatal("setup error: Fetch was expected to fail against missing remote")
		}

		respondGitError(w, fetchErr)
		if w.Code != http.StatusBadGateway {
			t.Errorf("expected 502, got %d", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "origin") {
			t.Errorf("expected error body to mention failing remote 'origin', got: %s", body)
		}
		// Generic fallback ("internal error") would mask the real failure —
		// guard against a regression that drops the ErrFetchFailed case.
		if strings.Contains(body, "internal error") {
			t.Errorf("ErrFetchFailed should not be collapsed to 'internal error', got: %s", body)
		}
	})
}

func TestSanitizeFilePath(t *testing.T) {
	dir := "/repo"

	tests := []struct {
		name    string
		file    string
		want    string
		wantErr bool
	}{
		{name: "simple file", file: "main.go", want: "main.go"},
		{name: "nested file", file: "src/lib/utils.ts", want: "src/lib/utils.ts"},
		{name: "dot-slash prefix", file: "./main.go", want: "main.go"},
		{name: "redundant slashes", file: "src//lib//utils.ts", want: "src/lib/utils.ts"},
		{name: "trailing slash", file: "src/", want: "src"},

		// Traversal attempts
		{name: "parent traversal", file: "../etc/passwd", wantErr: true},
		{name: "deep traversal", file: "foo/../../../../etc/passwd", wantErr: true},
		{name: "bare dotdot", file: "..", wantErr: true},
		{name: "dotdot slash", file: "../", wantErr: true},

		// Absolute paths
		{name: "absolute path", file: "/etc/passwd", wantErr: true},

		// Edge: resolve to dir itself
		{name: "dot", file: ".", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeFilePath(dir, tt.file)
			if tt.wantErr {
				if err == nil {
					t.Errorf("sanitizeFilePath(%q, %q) = %q, want error", dir, tt.file, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("sanitizeFilePath(%q, %q) returned error: %v", dir, tt.file, err)
			}
			if got != tt.want {
				t.Errorf("sanitizeFilePath(%q, %q) = %q, want %q", dir, tt.file, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilePath_Symlinks(t *testing.T) {
	// Use a real temp directory so EvalSymlinks can resolve paths.
	tmpDir := t.TempDir()

	// Create a subdirectory and file inside the "repo".
	subDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Symlink inside repo → location inside repo (should succeed).
	goodLink := filepath.Join(tmpDir, "link-to-src")
	if err := os.Symlink(subDir, goodLink); err != nil {
		t.Fatalf("failed to create good symlink: %v", err)
	}

	// Symlink inside repo → /tmp (outside repo).
	evilLink := filepath.Join(tmpDir, "evil-link")
	if err := os.Symlink("/tmp", evilLink); err != nil {
		t.Fatalf("failed to create evil symlink: %v", err)
	}

	t.Run("symlink within repo is allowed", func(t *testing.T) {
		got, err := sanitizeFilePath(tmpDir, "link-to-src/main.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "link-to-src/main.go" {
			t.Errorf("got %q, want %q", got, "link-to-src/main.go")
		}
	})

	t.Run("symlink escaping repo is blocked", func(t *testing.T) {
		_, err := sanitizeFilePath(tmpDir, "evil-link/somefile")
		if err == nil {
			t.Fatal("expected error for symlink escaping repo, got nil")
		}
	})

	t.Run("symlink escaping repo bare is blocked", func(t *testing.T) {
		_, err := sanitizeFilePath(tmpDir, "evil-link")
		if err == nil {
			t.Fatal("expected error for bare symlink escaping repo, got nil")
		}
	})
}

func TestGitFetch_ReturnsOKForRealRepo(t *testing.T) {
	// Fetch behavior (remote resolution, prune, upstream selection) is covered
	// in internal/node/git/fetch_test.go. This test only verifies the HTTP
	// handler is wired correctly: a no-remote repo makes Fetch a no-op, so a
	// 200 response proves routing → handler → git.Fetch.
	dir := homeTempDir(t)
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	router := NewRouter(Deps{})
	req := httptest.NewRequest(http.MethodPost, "/git/fetch?path="+dir, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGitFetch_MissingPathIs400(t *testing.T) {
	router := NewRouter(Deps{})
	req := httptest.NewRequest(http.MethodPost, "/git/fetch", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestGitFetch_AcceptsBaseQueryParam verifies that the handler reads the
// optional `base` parameter and forwards it through to git.Fetch — without
// this wiring, the fork-workflow fix in git.Fetch is unreachable. The
// downstream "did the right remote actually advance" assertion lives in
// internal/node/git/fetch_test.go (TestFetch_FetchesBaseUpstreamRemote).
func TestGitFetch_AcceptsBaseQueryParam(t *testing.T) {
	dir := homeTempDir(t)
	if out, err := exec.Command("git", "init", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	router := NewRouter(Deps{})
	q := url.Values{"path": {dir}, "base": {"main"}}
	req := httptest.NewRequest(http.MethodPost, "/git/fetch?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp)
	}
}
