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
		unreadMap := make(map[string]*string, len(sessions))
		for _, s := range sessions {
			unreadMap[s.ID] = s.UnreadSince
		}

		result := make(map[string]any, len(snap.Statuses))
		for id, entry := range snap.Statuses {
			item := map[string]any{
				"sessionName":  entry.SessionName,
				"status":       string(entry.State),
				"providerType": entry.ProviderType,
			}
			if us, ok := unreadMap[id]; ok && us != nil {
				item["unreadSince"] = *us
			}
			result[id] = item
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"statuses":        result,
			"lastRefreshedAt": snap.LastRefreshedAt,
		})
	}
}
