package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/agent/session"
	"github.com/bxnlabs/argus/internal/agent/status"
)

// handleStatus returns a handler for GET /api/sessions/status.
func handleStatus(mgr *session.Manager, detector *status.Detector) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessions, err := mgr.List()
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Collect tmux names
		names := make([]string, 0, len(sessions))
		for _, s := range sessions {
			names = append(names, s.TmuxName)
		}

		statuses := detector.GetAllStatuses(r.Context(), names)

		// Prune trackers for sessions that no longer exist in tmux.
		detector.Cleanup()

		// Sync tmux activity timestamps into updated_at so the sidebar
		// and CLI reflect real session activity, not just creation time.
		activities, _ := session.GetSessionActivities()
		activityByName := make(map[string]int64, len(activities))
		for _, a := range activities {
			activityByName[a.Name] = a.Timestamp
		}

		// Map back to session IDs
		result := make(map[string]any)
		for _, s := range sessions {
			st, ok := statuses[s.TmuxName]
			if !ok {
				st = status.StatusIdle
			}
			result[s.ID] = map[string]any{
				"sessionName": s.TmuxName,
				"status":      string(st),
				"agentType":   s.AgentType,
			}
			if ts, ok := activityByName[s.TmuxName]; ok {
				mgr.TouchSession(s.ID, ts)
			}
		}

		respondJSON(w, http.StatusOK, map[string]any{"statuses": result})
	}
}
