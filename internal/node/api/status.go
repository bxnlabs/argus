package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/node/status"
)

// handleStatus returns a handler for GET /api/sessions/status.
func handleStatus(mon *status.Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := mon.Snapshot()
		result := make(map[string]any, len(snap.Statuses))
		for id, entry := range snap.Statuses {
			result[id] = map[string]any{
				"sessionName":  entry.SessionName,
				"status":       string(entry.Status),
				"providerType": entry.ProviderType,
			}
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"statuses":        result,
			"lastRefreshedAt": snap.LastRefreshedAt,
		})
	}
}
