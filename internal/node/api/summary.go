package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/status"
)

// snapshotReader supplies the in-memory activity snapshot. *status.WatcherManager
// satisfies it in production; tests inject a fake.
type snapshotReader interface {
	Snapshot() status.Snapshot
}

// handleSummary returns a handler for GET /summary (external /api/node/summary).
// It reports lightweight per-node counts for the multi-node node list:
//   - busy:      sessions currently active (agent running)
//   - attention: idle/dead sessions that are unread (waiting on the user)
//   - total:     all sessions
//
// "attention" mirrors the sidebar rule: a session needs attention when it is
// unread (auto unread_since OR manual user_marked_unread_at) and not active.
func handleSummary(snaps snapshotReader, database *db.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := snaps.Snapshot()
		sessions, err := database.ListSessions(r.Context())
		if err != nil {
			respondInternalError(w, err)
			return
		}
		var busy, attention int
		for _, s := range sessions {
			active := false
			if entry, ok := snap.Statuses[s.ID]; ok && entry.State == status.StateActive {
				active = true
			}
			unread := s.UnreadSince != nil || s.UserMarkedUnreadAt != nil
			if active {
				busy++
			} else if unread {
				attention++
			}
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"attention": attention,
			"busy":      busy,
			"total":     len(sessions),
		})
	}
}
