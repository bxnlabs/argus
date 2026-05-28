package cli

import (
	"fmt"
	"strings"
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
