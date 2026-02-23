package db

import "errors"

// ErrNotFound is returned when a session does not exist.
var ErrNotFound = errors.New("not found")

// Session represents a tmux-backed coding session.
type Session struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	TmuxName         string  `json:"tmux_name"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	WorkingDirectory string  `json:"working_directory"`
	ProviderSessionID  *string `json:"provider_session_id"`
	Model            *string `json:"model"`
	SystemPrompt     *string `json:"system_prompt"`
	AgentType        string  `json:"agent_type"`
	AutoApprove      bool    `json:"auto_approve"`
}
