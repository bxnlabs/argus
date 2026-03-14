package api

import (
	"errors"
	"net/http"
	"os"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/provider"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/git/worktree"
)

type sessionHandler struct {
	manager *session.Manager
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
	home, _ := os.UserHomeDir()
	respondJSON(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"home_dir": home,
	})
}

// POST /api/sessions
func (h *sessionHandler) create(w http.ResponseWriter, r *http.Request) {
	var opts session.CreateOptions
	if err := parseBody(w, r, &opts); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if opts.ProviderType == "" {
		opts.ProviderType = string(provider.ProviderClaude)
	}
	if opts.Name == "" {
		opts.Name = "New Session"
	}

	sess, err := h.manager.Create(opts)
	if err != nil {
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": sess})
}

// GET /api/sessions/{id}
func (h *sessionHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := h.manager.EnsureSession(id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
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
	respondJSON(w, http.StatusOK, map[string]any{"session": session})
}

// PATCH /api/sessions/{id}
func (h *sessionHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Name              *string `json:"name"`
		ProviderSessionID *string `json:"provider_session_id"`
	}
	if err := parseBody(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	update := db.SessionUpdate{
		Name:              body.Name,
		ProviderSessionID: body.ProviderSessionID,
	}
	session, err := h.manager.Update(id, update)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session": session})
}

// GET /api/profiles
func (h *sessionHandler) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.manager.ListProfiles()
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
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
