package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/status"
)

// handleStatus returns a handler for GET /api/sessions/status.
// Composes activity state from in-memory snapshot with unread_since from DB.
func handleStatus(mgr *status.WatcherManager, database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := mgr.Snapshot()

		sessions, err := database.ListSessions(r.Context())
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// Build response from DB sessions (authoritative list) and overlay
		// snapshot state. Sessions without a snapshot entry default to idle
		// rather than being omitted (which the client would interpret as dead).
		result := make(map[string]any, len(sessions))
		for _, s := range sessions {
			item := map[string]any{
				"sessionName":  s.TmuxName,
				"status":       string(status.StateIdle),
				"providerType": s.ProviderType,
			}
			if entry, ok := snap.Statuses[s.ID]; ok {
				item["sessionName"] = entry.SessionName
				item["status"] = string(entry.State)
				item["providerType"] = entry.ProviderType
			}
			if s.UnreadSince != nil {
				item["unreadSince"] = *s.UnreadSince
			}
			result[s.ID] = item
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"statuses":        result,
			"lastRefreshedAt": snap.LastRefreshedAt,
		})
	}
}
