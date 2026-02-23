package api

import (
	"errors"
	"net/http"

	"github.com/bxnlabs/argus/internal/agent/db"
	agentsession "github.com/bxnlabs/argus/internal/agent/session"
)

type sessionHandler struct {
	manager *agentsession.Manager
}

// GET /api/sessions
func (h *sessionHandler) list(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.manager.List()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if sessions == nil {
		sessions = []*db.Session{}
	}
	respondJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// POST /api/sessions
func (h *sessionHandler) create(w http.ResponseWriter, r *http.Request) {
	var opts agentsession.CreateOptions
	if err := parseBody(w, r, &opts); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if opts.AgentType == "" {
		opts.AgentType = "claude"
	}
	if opts.WorkingDirectory == "" {
		opts.WorkingDirectory = "~"
	}
	if opts.Name == "" {
		opts.Name = "New Session"
	}

	session, err := h.manager.Create(opts)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": session})
}

// GET /api/sessions/{id}
func (h *sessionHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, err := h.manager.Get(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
		Name             *string `json:"name"`
		WorkingDirectory *string `json:"working_directory"`
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
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Apply other updates
	u := db.SessionUpdate{
		WorkingDirectory: body.WorkingDirectory,
	}
	if body.WorkingDirectory != nil {
		if err := h.manager.Update(id, u); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				respondError(w, http.StatusNotFound, "session not found")
				return
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	session, err := h.manager.Get(id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if session == nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session": session})
}

// DELETE /api/sessions/{id}
func (h *sessionHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.manager.Delete(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]bool{"success": true})
}


