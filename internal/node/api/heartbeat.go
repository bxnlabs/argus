package api

import (
	"context"
	"net/http"
)

// HeartbeatDB abstracts the DB operations needed by heartbeat/acknowledge handlers.
type HeartbeatDB interface {
	TouchLastViewedAt(ctx context.Context, id string) error
	AcknowledgeSession(ctx context.Context, id string) error
	MarkSessionUnread(ctx context.Context, id string) error
}

type heartbeatHandler struct {
	db HeartbeatDB
}

// heartbeat handles POST /api/sessions/{id}/heartbeat.
// Updates last_viewed_at = now(). Lightweight — single DB update.
func (h *heartbeatHandler) heartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.TouchLastViewedAt(r.Context(), id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// acknowledge handles POST /api/sessions/{id}/acknowledge.
// Clears unread_since and sets last_viewed_at = now(). Idempotent.
func (h *heartbeatHandler) acknowledge(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.AcknowledgeSession(r.Context(), id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// markUnread handles POST /api/sessions/{id}/unread.
// Sets unread_since = now(). Idempotent.
func (h *heartbeatHandler) markUnread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.db.MarkSessionUnread(r.Context(), id); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
