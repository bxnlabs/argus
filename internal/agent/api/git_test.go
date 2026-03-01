package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/git"
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
