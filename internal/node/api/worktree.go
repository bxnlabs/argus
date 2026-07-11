package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/bxnlabs/argus/internal/git"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/shared"
)

// worktreeHandler serves the worktree management routes backed by the shared
// worktree.Manager: reuse-or-create a worktree for a branch, list managed
// worktrees, and remove one. db is used to refuse removing a worktree that an
// active session still occupies.
type worktreeHandler struct {
	mgr *worktree.Manager
	db  *db.DB
}

type worktreeCreateResponse struct {
	Path          string `json:"path"`
	Branch        string `json:"branch"`
	Created       bool   `json:"created"`
	BranchCreated bool   `json:"branchCreated"`
}

// resolveRepoParam expands the ?path= repo param and normalizes it to the main
// repository root, so managed worktrees are keyed consistently regardless of
// whether a subdirectory or linked-worktree path was passed (the CLI already
// normalizes, but a direct API caller may not). It writes a 400 and returns
// ok=false on an unsafe path or a non-repository path.
func (h *worktreeHandler) resolveRepoParam(w http.ResponseWriter, repoPath string) (string, bool) {
	expanded, err := shared.SafeExpandPath(repoPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	root, err := git.FindMainRepo(expanded)
	if err != nil {
		respondError(w, http.StatusBadRequest, "not a git repository: "+repoPath)
		return "", false
	}
	return root, true
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
	// Validate the branch name up front so a bad name is a clean 400 rather than
	// a generic 500 from a downstream git failure. A leading dash is rejected
	// explicitly (matching session validation) because check-ref-format accepts
	// it as a refname but `git worktree add -b` treats it as a flag.
	if strings.HasPrefix(branch, "-") || git.Run("", "check-ref-format", "refs/heads/"+branch) != nil {
		respondError(w, http.StatusBadRequest, "invalid branch name: "+branch)
		return
	}
	repoRoot, ok := h.resolveRepoParam(w, repoPath)
	if !ok {
		return
	}
	wtPath, br, created, branchCreated, err := h.mgr.CreateForLocalRepo(repoRoot, "", branch)
	if err != nil {
		// A precondition conflict (e.g. branch checked out in the main tree) is
		// user-actionable — surface it as 409 with its message, not a 500.
		if errors.Is(err, worktree.ErrWorktreeConflict) {
			respondError(w, http.StatusConflict, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	// created==false means an existing worktree for the branch was reused.
	// FindWorktree matches any linked worktree, so guard against reusing an
	// unmanaged one — list hides it and delete refuses it, so returning its
	// path here would be inconsistent. Reject with 409 to match that boundary.
	if !created && !h.mgr.IsManagedPath(wtPath) {
		respondError(w, http.StatusConflict, "branch "+branch+" has a worktree that is not Argus-managed")
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
	repoRoot, ok := h.resolveRepoParam(w, repoPath)
	if !ok {
		return
	}
	worktrees, err := h.mgr.ListManaged(repoRoot)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"worktrees": worktrees})
}

// worktreeInUse reports whether any session's working directory refers to
// wtPath. It compares by symlink-resolved path rather than exact string, so a
// session persisted under a different spelling that resolves to the same
// directory — e.g. a reuse-by-path session created under a symlinked
// ARGUS_HOME — is still detected. An exact-string match would silently miss
// those and let rm delete a worktree out from under a live session.
//
// This per-guard resolution is a workaround for paths not being canonicalized
// at ingestion (BXN-122). The check is also a TOCTOU read, not mutual exclusion
// with session creation (BXN-123).
func (h *worktreeHandler) worktreeInUse(ctx context.Context, wtPath string) (bool, error) {
	target := wtPath
	if resolved, err := shared.EvalSymlinks(wtPath); err == nil {
		target = resolved
	}
	sessions, err := h.db.ListSessions(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.WorkingDirectory == "" {
			continue
		}
		dir := s.WorkingDirectory
		if resolved, err := shared.EvalSymlinks(dir); err == nil {
			dir = resolved
		}
		if dir == target {
			return true, nil
		}
	}
	return false, nil
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
	repoRoot, ok := h.resolveRepoParam(w, repoPath)
	if !ok {
		return
	}
	wtPath, err := h.mgr.FindWorktree(repoRoot, branch)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if wtPath == "" || !h.mgr.IsManagedPath(wtPath) {
		respondError(w, http.StatusNotFound, "no managed worktree for branch "+branch)
		return
	}
	// Refuse to remove a worktree an active session still occupies, so wt rm
	// cannot pull a live tmux session's directory out from under it. This
	// mirrors the session-deletion guard.
	if h.db != nil {
		inUse, err := h.worktreeInUse(r.Context(), wtPath)
		if err != nil {
			respondInternalError(w, err)
			return
		}
		if inUse {
			respondError(w, http.StatusConflict, "worktree for branch "+branch+" is in use by an active session")
			return
		}
	} else {
		log.Printf("worktree delete: no database configured; skipping in-use session check for %s", wtPath)
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
