package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/provider"
	agentsession "github.com/bxnlabs/argus/internal/agent/session"
	"github.com/bxnlabs/argus/internal/git"
	"github.com/bxnlabs/argus/internal/git/worktree"
)

// sessionResponse extends db.Session with computed display fields.
type sessionResponse struct {
	*db.Session
	GitParentDir *string `json:"git_parent_dir"`
}

// enrichSession computes display fields for a session response.
func enrichSession(s *db.Session) sessionResponse {
	resp := sessionResponse{Session: s}
	if s.WorktreeBranch != nil {
		if dir, err := git.FindMainRepo(s.WorkingDirectory); err == nil {
			resp.GitParentDir = &dir
		}
	}
	return resp
}

type sessionHandler struct {
	manager *agentsession.Manager
}

// GET /api/sessions
func (h *sessionHandler) list(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.manager.List(r.Context())
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if sessions == nil {
		sessions = []*db.Session{}
	}
	resp := make([]sessionResponse, len(sessions))
	for i, s := range sessions {
		resp[i] = enrichSession(s)
	}
	home, _ := os.UserHomeDir()
	respondJSON(w, http.StatusOK, map[string]any{
		"sessions": resp,
		"home_dir": home,
	})
}

// POST /api/sessions
func (h *sessionHandler) create(w http.ResponseWriter, r *http.Request) {
	var opts agentsession.CreateOptions
	if err := parseBody(w, r, &opts); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if opts.AgentType == "" {
		opts.AgentType = string(provider.AgentClaude)
	}
	if opts.Name == "" {
		opts.Name = "New Session"
	}
	// opts.Source may be empty — lifecycle defaults to home directory

	session, err := h.manager.Create(opts)
	if err != nil {
		if errors.Is(err, agentsession.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": enrichSession(session)})
}

// GET /api/sessions/{id}
func (h *sessionHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// EnsureSession revives the tmux session if it died, then we fetch
	// the full DB record to return to the caller.
	if _, err := h.manager.EnsureSession(id); err != nil {
		if errors.Is(err, agentsession.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		respondInternalError(w, err)
		return
	}

	session, err := h.manager.Get(id)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if session == nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session": enrichSession(session)})
}

// PATCH /api/sessions/{id}
func (h *sessionHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Name *string `json:"name"`
	}
	if err := parseBody(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// If renaming, use the lifecycle rename (display name only)
	if body.Name != nil {
		if err := h.manager.Rename(id, *body.Name); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				respondError(w, http.StatusNotFound, "session not found")
				return
			}
			respondInternalError(w, err)
			return
		}
	}

	session, err := h.manager.Get(id)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if session == nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session": enrichSession(session)})
}

// DELETE /api/sessions/{id}
func (h *sessionHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := r.URL.Query().Get("force") == "true"

	if err := h.manager.Delete(id, force); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, worktree.ErrWorktreeDirty) {
			respondError(w, http.StatusConflict, "worktree has uncommitted changes; use force to delete anyway")
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"success": true})
}


