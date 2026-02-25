package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

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
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			if len(resp.Sessions) == 0 {
				fmt.Println("No sessions.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tPROVIDER\tUPDATED")
			for _, s := range resp.Sessions {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.ID, s.Name, s.AgentType, relativeTime(s.UpdatedAt))
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
