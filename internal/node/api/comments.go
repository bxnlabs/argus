package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/node/comments"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
)

type commentsHandler struct {
	projectDirOverride string // for testing — bypasses home dir derivation
}

func (h *commentsHandler) resolveProjectDir(expandedPath string) (string, error) {
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

// GET /api/git/comments?path=...&branch=...&base=...
func (h *commentsHandler) get(w http.ResponseWriter, r *http.Request) {
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
	cf, err := comments.Load(projectDir, expandedPath, branch, base)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if cf == nil {
		cf = &comments.CommentsFile{
			Branch:     branch,
			BaseBranch: base,
			Comments:   []comments.Comment{},
		}
	}
	respondJSON(w, http.StatusOK, cf)
}

// POST /api/git/comments?path=...
func (h *commentsHandler) post(w http.ResponseWriter, r *http.Request) {
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
	var cf comments.CommentsFile
	if err := parseBody(w, r, &cf); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for _, c := range cf.Comments {
		if _, err := sanitizeFilePath(expandedPath, c.File); err != nil {
			respondError(w, http.StatusBadRequest, "invalid file path in comment: "+c.File)
			return
		}
	}
	if err := comments.Save(projectDir, &cf); err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/git/comments?path=...&branch=...&base=...
func (h *commentsHandler) delete(w http.ResponseWriter, r *http.Request) {
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
	if err := comments.Delete(projectDir, branch, base); err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
