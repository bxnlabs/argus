package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func strOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// formatSessionDescribe renders the curated, sectioned human-readable summary
// of a session. status is the runtime status ("active"/"idle"/"dead"), or ""
// when unavailable; home is the user's home dir for path compression.
func formatSessionDescribe(s sessionInfo, status, home string) string {
	if status == "" {
		status = "-"
	}

	dir := s.WorkingDirectory
	if s.GitParentDir != nil && *s.GitParentDir != "" {
		dir = *s.GitParentDir
	}
	if dir == "" {
		dir = "-"
	} else {
		dir = compressPath(dir, home, 60)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\n", s.Name)
	fmt.Fprintf(&b, "  ID:        %s\n", s.ID)
	fmt.Fprintf(&b, "  Status:    %s\n", status)
	fmt.Fprintf(&b, "  Pinned:    %s\n", yesNo(s.Pinned))
	fmt.Fprintf(&b, "  Profile:   %s\n", strOr(s.Profile, "none"))

	b.WriteString("\nProvider\n")
	fmt.Fprintf(&b, "  Type:         %s\n", s.ProviderType)
	if s.Model != nil && *s.Model != "" {
		fmt.Fprintf(&b, "  Model:        %s\n", *s.Model)
	}
	fmt.Fprintf(&b, "  Auto-approve: %s\n", onOff(s.AutoApprove))

	b.WriteString("\nLocation\n")
	fmt.Fprintf(&b, "  Directory: %s\n", dir)
	if s.GitRemoteURL != nil {
		if repo := parseRepo(*s.GitRemoteURL); repo != "" {
			fmt.Fprintf(&b, "  Repo:      %s\n", repo)
		}
	}
	if s.WorktreeBranch != nil && *s.WorktreeBranch != "" {
		fmt.Fprintf(&b, "  Branch:    %s\n", *s.WorktreeBranch)
	}

	b.WriteString("\nTimestamps\n")
	fmt.Fprintf(&b, "  Created:   %s (%s)\n", s.CreatedAt, relativeTime(s.CreatedAt))
	fmt.Fprintf(&b, "  Updated:   %s (%s)\n", s.UpdatedAt, relativeTime(s.UpdatedAt))

	return b.String()
}

// newDescribeCmd returns the "session describe" command.
func newDescribeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "describe <sess>",
		Short: "Show full details for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			query := args[0]

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			// Fetch the list once: resolve the query and learn the home dir
			// (used for path compression). Mirrors `session ls`.
			body, err := c.get("/api/sessions")
			if err != nil {
				return err
			}
			var listResp struct {
				Sessions []sessionInfo `json:"sessions"`
				HomeDir  string        `json:"home_dir"`
			}
			if err := json.Unmarshal(body, &listResp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			s, err := resolveSession(listResp.Sessions, query)
			if err != nil {
				return err
			}

			if asJSON {
				// Print the canonical full record from GET /api/sessions/{id}.
				detail, err := c.get("/api/sessions/" + s.ID)
				if err != nil {
					return err
				}
				var wrap struct {
					Session json.RawMessage `json:"session"`
				}
				if err := json.Unmarshal(detail, &wrap); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, wrap.Session, "", "  "); err != nil {
					return fmt.Errorf("format json: %w", err)
				}
				fmt.Println(pretty.String())
				return nil
			}

			// Best-effort runtime status (don't fail if unavailable).
			status := ""
			if statusBody, err := c.get("/api/sessions/status"); err == nil {
				var statusResp struct {
					Statuses map[string]struct {
						Status string `json:"status"`
					} `json:"statuses"`
				}
				if err := json.Unmarshal(statusBody, &statusResp); err == nil {
					status = statusResp.Statuses[s.ID].Status
				}
			}

			fmt.Print(formatSessionDescribe(*s, status, listResp.HomeDir))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the raw session record as JSON")
	return cmd
}
