package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sessionInfo is a lightweight mirror of the API session response.
type sessionInfo struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	TmuxName         string  `json:"tmux_name"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	WorkingDirectory string  `json:"working_directory"`
	AgentType        string  `json:"agent_type"`
	AutoApprove      bool    `json:"auto_approve"`
	Model            *string `json:"model"`
	WorktreeBranch   *string `json:"worktree_branch"`
	GitParentDir     *string `json:"git_parent_dir"`
}

// fetchAndResolve fetches all sessions from the agent and resolves the query
// to a single session by name or ID prefix.
func fetchAndResolve(c *apiClient, query string) (*sessionInfo, error) {
	body, err := c.get("/api/sessions")
	if err != nil {
		return nil, err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return resolveSession(resp.Sessions, query)
}

// resolveSession finds a session by exact name match or ID prefix.
func resolveSession(sessions []sessionInfo, query string) (*sessionInfo, error) {
	// 1. Exact name match.
	for i := range sessions {
		if sessions[i].Name == query {
			return &sessions[i], nil
		}
	}

	// 2. ID prefix match.
	var matches []*sessionInfo
	for i := range sessions {
		if strings.HasPrefix(sessions[i].ID, query) {
			matches = append(matches, &sessions[i])
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session found matching %q", query)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, fmt.Sprintf("  %s (%s)", m.Name, m.ID))
		}
		return nil, fmt.Errorf("ambiguous match %q — multiple sessions match:\n%s", query, strings.Join(names, "\n"))
	}
}
