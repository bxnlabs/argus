package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
)

// HeartbeatDB abstracts the DB operations needed by heartbeat/acknowledge handlers.
type HeartbeatDB interface {
	TouchLastViewedAt(ctx context.Context, id string) error
	AcknowledgeSession(ctx context.Context, id string) error
	MarkSessionUnread(ctx context.Context, id string) error
	MarkSessionRead(ctx context.Context, id string) error
}

type heartbeatHandler struct {
	db HeartbeatDB
}

// respond writes 204 on success, 404 when the session does not exist, and 500
// otherwise. Shared by the four idempotent unread-state endpoints below.
func (h *heartbeatHandler) respond(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, db.ErrNotFound):
		respondError(w, http.StatusNotFound, "session not found")
	default:
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

// heartbeat handles POST /sessions/{id}/heartbeat.
// Updates last_viewed_at = now(). Lightweight — single DB update.
func (h *heartbeatHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	h.respond(w, h.db.TouchLastViewedAt(r.Context(), r.PathValue("id")))
}

// acknowledge handles POST /sessions/{id}/acknowledge.
// Clears unread_since and sets last_viewed_at = now(). Idempotent.
func (h *heartbeatHandler) acknowledge(w http.ResponseWriter, r *http.Request) {
	h.respond(w, h.db.AcknowledgeSession(r.Context(), r.PathValue("id")))
}

// markUnread handles POST /sessions/{id}/unread.
// Sets the manual user_marked_unread_at marker; does not touch unread_since. Idempotent.
func (h *heartbeatHandler) markUnread(w http.ResponseWriter, r *http.Request) {
	h.respond(w, h.db.MarkSessionUnread(r.Context(), r.PathValue("id")))
}

// markRead handles POST /sessions/{id}/read.
// Clears both unread_since and user_marked_unread_at and sets last_viewed_at. Idempotent.
func (h *heartbeatHandler) markRead(w http.ResponseWriter, r *http.Request) {
	h.respond(w, h.db.MarkSessionRead(r.Context(), r.PathValue("id")))
}
