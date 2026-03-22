package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/node/review"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
)

type reviewHandler struct {
	projectDirOverride string // for testing — bypasses home dir derivation
}

func (h *reviewHandler) resolveProjectDir(expandedPath string) (string, error) {
	if h.projectDirOverride != "" {
		return h.projectDirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	parentKey := source.ParentKeyFromPath(expandedPath)
	return filepath.Join(home, ".argus", "projects", parentKey), nil
}

// GET /api/git/review?path=...&branch=...&base=...
func (h *reviewHandler) get(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")
	if repoPath == "" || branch == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path, branch, and base parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	rv, err := review.Load(projectDir, expandedPath, branch, base)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if rv == nil {
		rv = &review.Review{
			Head:     branch,
			Base:     base,
			Comments: []review.ReviewComment{},
		}
	}
	respondJSON(w, http.StatusOK, rv)
}

// POST /api/git/review?path=...
func (h *reviewHandler) post(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	if repoPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	var rv review.Review
	if err := parseBody(w, r, &rv); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, c := range rv.Comments {
		if _, err := sanitizeFilePath(expandedPath, c.File); err != nil {
			respondError(w, http.StatusBadRequest, "invalid file path in comment: "+c.File)
			return
		}
	}
	if err := review.Save(projectDir, &rv); err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/git/review?path=...&branch=...&base=...
func (h *reviewHandler) delete(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	base := r.URL.Query().Get("base")
	if repoPath == "" || branch == "" || base == "" {
		respondError(w, http.StatusBadRequest, "path, branch, and base parameters are required")
		return
	}
	expandedPath, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	projectDir, err := h.resolveProjectDir(expandedPath)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if err := review.Delete(projectDir, branch, base); err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
