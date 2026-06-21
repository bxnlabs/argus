package api

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/provider"
	"github.com/bxnlabs/argus/internal/node/session"
)

// watcherEnsurer is satisfied by *status.WatcherManager.
type watcherEnsurer interface {
	EnsureWatching(sessionID, tmuxName, providerType string)
	StopWatcher(sessionID string)
}

type sessionHandler struct {
	manager        *session.Manager
	watcherManager watcherEnsurer
}

// GET /sessions
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

// POST /sessions
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
		if errors.Is(err, session.ErrStackStart) {
			respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": sess})
}

// POST /sessions/{id}/clone
func (h *sessionHandler) clone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	sess, err := h.manager.Clone(id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, session.ErrStackStart) {
			respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": sess})
}

// GET /sessions/{id}
func (h *sessionHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, err := h.manager.EnsureSession(id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, session.ErrStackStart) {
			respondError(w, http.StatusBadGateway, err.Error())
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

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(session.ID, session.TmuxName, session.ProviderType)
	}

	respondJSON(w, http.StatusOK, map[string]any{"session": session})
}

// PATCH /sessions/{id}
func (h *sessionHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Name              *string `json:"name"`
		ProviderSessionID *string `json:"provider_session_id"`
		Pinned            *bool   `json:"pinned"`
	}
	if err := parseBody(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	update := db.SessionUpdate{
		Name:              body.Name,
		ProviderSessionID: body.ProviderSessionID,
		Pinned:            body.Pinned,
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

// PUT /sessions/{id}/profile sets or changes a session to a named profile.
// To detach a profile, use DELETE instead — a profile name is required here.
func (h *sessionHandler) setProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Profile string `json:"profile"`
	}
	if err := parseBody(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(body.Profile) == "" {
		respondError(w, http.StatusBadRequest, "profile is required; use DELETE to detach")
		return
	}

	sess, err := h.manager.ChangeProfile(id, &body.Profile)
	h.respondProfileChange(w, sess, err)
}

// DELETE /sessions/{id}/profile detaches the profile (restarts the
// session). Detaching an already-detached session is a no-op.
func (h *sessionHandler) detachProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := h.manager.ChangeProfile(id, nil)
	h.respondProfileChange(w, sess, err)
}

// respondProfileChange writes the result of a ChangeProfile call, mapping
// known errors to status codes and re-arming the session watcher on success.
func (h *sessionHandler) respondProfileChange(w http.ResponseWriter, sess *db.Session, err error) {
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, session.ErrStackStart) {
			respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
	}

	respondJSON(w, http.StatusOK, map[string]any{"session": sess})
}

// GET /profiles
func (h *sessionHandler) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.manager.ListProfilesDetailed()
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

// POST /profiles/{name}/up brings a dockerized profile's stack up.
func (h *sessionHandler) profileUp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.manager.ProfileUp(name); err != nil {
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, session.ErrStackStart) {
			respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"name": name, "stack": "up"})
}

// POST /profiles/{name}/down tears a dockerized profile's stack down.
func (h *sessionHandler) profileDown(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.manager.ProfileDown(name); err != nil {
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, session.ErrProfileInUse) {
			respondError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, session.ErrStackStop) {
			respondError(w, http.StatusBadGateway, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"name": name, "stack": "down"})
}

// DELETE /sessions/{id}
func (h *sessionHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := r.URL.Query().Get("force") == "true"
	deleteBranch := r.URL.Query().Get("delete_branch") == "true"

	result, err := h.manager.Delete(id, force, deleteBranch)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, worktree.ErrWorktreeDirty) {
			respondError(w, http.StatusConflict, "worktree has uncommitted changes; use force to delete anyway")
			return
		}
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	if h.watcherManager != nil {
		h.watcherManager.StopWatcher(id)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"branch_deleted": result.BranchDeleted,
	})
}
