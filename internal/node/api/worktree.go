package api

import (
	"errors"
	"net/http"

	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/shared"
)

// worktreeHandler serves the worktree management routes backed by the shared
// worktree.Manager: reuse-or-create a worktree for a branch, list managed
// worktrees, and remove one.
type worktreeHandler struct {
	mgr *worktree.Manager
}

type worktreeCreateResponse struct {
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	Created       bool   `json:"created"`
	BranchCreated bool   `json:"branchCreated"`
}

// create handles POST /git/worktree?path=<repo>&branch=<b>. It reuses an
// existing worktree for the branch or creates a new one (session-less: no DB or
// tmux). branch is passed as the branch override, so it is used verbatim
// without the configured prefix.
func (h *worktreeHandler) create(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	if repoPath == "" || branch == "" {
		respondError(w, http.StatusBadRequest, "path and branch parameters are required")
		return
	}
	expanded, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	wtPath, br, created, branchCreated, err := h.mgr.CreateForLocalRepo(expanded, "", branch)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, worktreeCreateResponse{
		Path:          wtPath,
		Branch:        br,
		Created:       created,
		BranchCreated: branchCreated,
	})
}

// list handles GET /git/worktrees?path=<repo>. It returns the managed linked
// worktrees for the repo.
func (h *worktreeHandler) list(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	if repoPath == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expanded, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	worktrees, err := h.mgr.ListManaged(expanded)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"worktrees": worktrees})
}

// delete handles DELETE /git/worktree?path=<repo>&branch=<b>. It finds the
// worktree for the branch and removes it without force; a dirty worktree yields
// HTTP 400 so the caller can surface a clean error. A missing worktree yields
// HTTP 404. The branch itself is intentionally preserved.
func (h *worktreeHandler) delete(w http.ResponseWriter, r *http.Request) {
	repoPath := r.URL.Query().Get("path")
	branch := r.URL.Query().Get("branch")
	if repoPath == "" || branch == "" {
		respondError(w, http.StatusBadRequest, "path and branch parameters are required")
		return
	}
	expanded, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	wtPath, err := h.mgr.FindWorktree(expanded, branch)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if wtPath == "" {
		respondError(w, http.StatusNotFound, "no managed worktree for branch "+branch)
		return
	}
	if err := h.mgr.RemoveWorktree(wtPath, false); err != nil {
		if errors.Is(err, worktree.ErrWorktreeDirty) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
