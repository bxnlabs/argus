package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// pinnedFirst returns sessions with pinned ones first, preserving the input
// order within each group (the API already returns sessions updated_at DESC).
func pinnedFirst(sessions []sessionInfo) []sessionInfo {
	out := make([]sessionInfo, 0, len(sessions))
	for _, s := range sessions {
		if s.Pinned {
			out = append(out, s)
		}
	}
	for _, s := range sessions {
		if !s.Pinned {
			out = append(out, s)
		}
	}
	return out
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			body, err := c.get("/api/sessions")
			if err != nil {
				return err
			}

			var resp struct {
				Sessions []sessionInfo `json:"sessions"`
				HomeDir  string        `json:"home_dir"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			if len(resp.Sessions) == 0 {
				fmt.Println("No sessions.")
				return nil
			}

			// Fetch session statuses (best-effort — don't fail if unavailable)
			type statusEntry struct {
				Status             string  `json:"status"`
				UnreadSince        *string `json:"unreadSince"`
				UserMarkedUnreadAt *string `json:"userMarkedUnreadAt"`
			}
			statuses := make(map[string]statusEntry)
			if statusBody, err := c.get("/api/sessions/status"); err == nil {
				var statusResp struct {
					Statuses map[string]statusEntry `json:"statuses"`
				}
				if err := json.Unmarshal(statusBody, &statusResp); err == nil {
					statuses = statusResp.Statuses
				}
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "  ID\tPINNED\tNAME\tSTATUS\tPROVIDER\tPROFILE\tDIRECTORY\tBRANCH\tUPDATED")
			for _, s := range pinnedFirst(resp.Sessions) {
				entry := statuses[s.ID]
				st := entry.Status
				if st == "" {
					st = "-"
				}
				branch := ""
				if s.WorktreeBranch != nil {
					branch = *s.WorktreeBranch
				}
				dir := s.WorkingDirectory
				if s.GitParentDir != nil {
					dir = *s.GitParentDir
				}
				if dir == "" {
					dir = "-"
				} else {
					dir = compressPath(dir, resp.HomeDir, 35)
				}

				// Unread marker — set by either the automatic unread_since or the
				// manual user_marked_unread_at follow-up marker.
				eff := entry.UnreadSince
				if eff == nil {
					eff = entry.UserMarkedUnreadAt
				}
				marker := " "
				updated := relativeTime(s.UpdatedAt)
				if eff != nil {
					marker = "*"
					updated += " (unread " + relativeTime(*eff) + ")"
				}

				pinnedMark := ""
				if s.Pinned {
					pinnedMark = "✓"
				}

				profile := "-"
				if s.Profile != nil && *s.Profile != "" {
					profile = *s.Profile
				}

				fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					marker, s.ID, pinnedMark, s.Name, st, s.ProviderType, profile, dir, branch, updated)
			}
			w.Flush()
			return nil
		},
	}
}

// relativeTime converts a datetime string to a human-readable relative time.
func relativeTime(ts string) string {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, ts)
		if err == nil {
			break
		}
	}
	if err != nil {
		return ts
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
