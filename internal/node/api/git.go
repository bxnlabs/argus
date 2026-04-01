package api

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bxnlabs/argus/internal/node/git"
	"github.com/bxnlabs/argus/internal/shared"
)

// respondGitError maps git package sentinel errors to appropriate HTTP
// status codes, falling back to respondInternalError for unknown errors.
func respondGitError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, git.ErrInvalidInput):
		respondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, git.ErrNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, git.ErrFileTooLarge):
		respondError(w, http.StatusRequestEntityTooLarge, err.Error())
	case errors.Is(err, git.ErrBinaryFile):
		respondError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		respondInternalError(w, err)
	}
}

// sanitizeFilePath resolves file relative to dir, verifies the result stays
// inside dir (including through symlinks), and returns the cleaned relative path.
func sanitizeFilePath(dir, file string) (string, error) {
	if filepath.IsAbs(file) {
		return "", fmt.Errorf("file path escapes repository")
	}
	abs := filepath.Clean(filepath.Join(dir, file))
	cleanDir := filepath.Clean(dir)

	// Lexical check first.
	if !strings.HasPrefix(abs, cleanDir+string(filepath.Separator)) {
		return "", fmt.Errorf("file path escapes repository")
	}

	// Resolve symlinks to catch escape-via-symlink (e.g., a symlink inside
	// the repo pointing outside it). If the directory doesn't exist on disk,
	// skip the symlink check — there are no symlinks to resolve.
	resolvedDir, dirErr := filepath.EvalSymlinks(cleanDir)
	if dirErr == nil {
		resolved, err := shared.EvalSymlinks(abs)
		if err != nil {
			return "", fmt.Errorf("file path escapes repository")
		}
		if !strings.HasPrefix(resolved, resolvedDir+string(filepath.Separator)) {
			return "", fmt.Errorf("file path escapes repository")
		}
	} else if !os.IsNotExist(dirErr) {
		return "", fmt.Errorf("file path escapes repository")
	}

	// Return relative path using original names (preserves symlink names for git).
	rel, err := filepath.Rel(cleanDir, abs)
	if err != nil {
		return "", fmt.Errorf("file path escapes repository")
	}
	return rel, nil
}

type gitHandler struct{}

// GET /api/git/status?path=...
func (h *gitHandler) status(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	status, err := git.GetStatus(expandedPath)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": status})
}

// GET /api/git/diff?path=...&file=...&staged=...&untracked=...
func (h *gitHandler) diff(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	file := r.URL.Query().Get("file")
	if path == "" || file == "" {
		respondError(w, http.StatusBadRequest, "path and file parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, err = sanitizeFilePath(expandedPath, file)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid file path")
		return
	}

	staged := r.URL.Query().Get("staged") == "true"
	untracked := r.URL.Query().Get("untracked") == "true"
	diff, err := git.GetFileDiff(expandedPath, file, staged, untracked)
	if err != nil {
		log.Printf("git diff failed: file=%s staged=%v untracked=%v err=%v", file, staged, untracked, err)
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"diff": diff})
}

// GET /api/git/working-diff?path=...
func (h *gitHandler) workingDiff(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := git.GetWorkingDiff(expandedPath)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/history?path=...&limit=...
func (h *gitHandler) history(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit := 30
	if s := r.URL.Query().Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
		}
	}

	commits, err := git.GetHistory(expandedPath, limit)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"commits": commits})
}

// GET /api/git/history/{hash}?path=...
func (h *gitHandler) commitDetail(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	commit, err := git.GetCommitDetail(expandedPath, hash)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"commit": commit})
}

// GET /api/git/history/{hash}/full-diff?path=...
func (h *gitHandler) commitFullDiff(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := git.GetCommitFullDiff(expandedPath, hash)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/compare?path=...&base=...
func (h *gitHandler) compare(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	base := r.URL.Query().Get("base")
	if path == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path and base parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := git.GetCompare(expandedPath, base)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/compare/branches?path=...
func (h *gitHandler) compareBranches(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := git.GetBranches(expandedPath)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/file-content?path=...&file=...
func (h *gitHandler) fileContent(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	file := r.URL.Query().Get("file")
	if path == "" || file == "" {
		respondError(w, http.StatusBadRequest, "path and file parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, err = sanitizeFilePath(expandedPath, file)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid file path")
		return
	}

	content, isNew, err := git.GetFileContent(expandedPath, file)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"content": content,
		"isNew":   isNew,
	})
}

// lexicalValidateFilePath rejects absolute paths and paths containing ".."
// components. Unlike sanitizeFilePath, it does not resolve symlinks — this is
// appropriate for ref-based reads where the working tree may have been
// restructured since the commit.
func lexicalValidateFilePath(file string) error {
	if filepath.IsAbs(file) {
		return fmt.Errorf("file path must be relative")
	}
	for _, part := range strings.Split(filepath.ToSlash(file), "/") {
		if part == ".." {
			return fmt.Errorf("file path must not contain '..'")
		}
	}
	return nil
}

// GET /api/git/file-lines?path=...&file=...&start=...&end=...&ref=...
func (h *gitHandler) fileLines(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	file := r.URL.Query().Get("file")
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")
	ref := r.URL.Query().Get("ref")

	if path == "" || file == "" || startStr == "" || endStr == "" {
		respondError(w, http.StatusBadRequest, "path, file, start, and end parameters are required")
		return
	}

	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate file path: filesystem-aware for working tree, lexical-only for refs
	if ref == "" {
		file, err = sanitizeFilePath(expandedPath, file)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid file path")
			return
		}
	} else {
		if err := lexicalValidateFilePath(file); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	start, err := strconv.Atoi(startStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "start must be an integer")
		return
	}
	end, err := strconv.Atoi(endStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "end must be an integer")
		return
	}

	result, err := git.GetFileLines(expandedPath, file, start, end, ref)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

// GET /api/git/check?path=...
func (h *gitHandler) check(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	isRepo, err := git.Check(expandedPath)
	if err != nil {
		respondGitError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"isGitRepo": isRepo})
}
