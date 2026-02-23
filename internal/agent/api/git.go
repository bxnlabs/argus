package api

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bxnlabs/argus/internal/agent/git"
	"github.com/bxnlabs/argus/internal/shared"
)

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
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"diff": diff})
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
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"commit": commit})
}

// GET /api/git/history/{hash}/diff?path=...&file=...
func (h *gitHandler) commitFileDiff(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
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

	diff, err := git.GetCommitFileDiff(expandedPath, hash, file)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"diff": diff})
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"content": content,
		"isNew":   isNew,
	})
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"isGitRepo": isRepo})
}
