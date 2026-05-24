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
	ProviderType     string  `json:"provider_type"`
	AutoApprove      bool    `json:"auto_approve"`
	WorktreeBranch   *string `json:"worktree_branch"`
	GitParentDir     *string `json:"git_parent_dir"`
	GitRemoteURL     *string `json:"git_remote_url"`
	Profile          *string `json:"profile"`
	BranchCreated    bool    `json:"branch_created"`
	UnreadSince      *string `json:"unread_since"`
	LastViewedAt     *string `json:"last_viewed_at"`
	MarkedUnreadAt   *string `json:"markedUnreadAt"`
	Pinned           bool    `json:"pinned"`
}
