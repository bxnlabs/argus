package db

import (
	"context"
	"database/sql"
	"fmt"
)

// sessionColumns is the explicit column list matching scanSession's scan order.
const sessionColumns = `id, name, tmux_name, created_at, updated_at,
	working_directory, provider_session_id, model, system_prompt,
	provider_type, auto_approve, worktree_branch, git_parent_dir, git_remote_url, profile, branch_created,
	unread_since, last_viewed_at`

func scanSession(row interface{ Scan(...any) error }) (*Session, error) {
	var s Session
	var autoApprove int
	var branchCreated int
	err := row.Scan(
		&s.ID, &s.Name, &s.TmuxName, &s.CreatedAt, &s.UpdatedAt,
		&s.WorkingDirectory,
		&s.ProviderSessionID, &s.Model, &s.SystemPrompt,
		&s.ProviderType, &autoApprove, &s.WorktreeBranch,
		&s.GitParentDir, &s.GitRemoteURL, &s.Profile, &branchCreated,
		&s.UnreadSince, &s.LastViewedAt,
	)
	if err != nil {
		return nil, err
	}
	s.AutoApprove = autoApprove != 0
	s.BranchCreated = branchCreated != 0
	return &s, nil
}

func (d *DB) CreateSession(s *Session) error {
	autoApprove := 0
	if s.AutoApprove {
		autoApprove = 1
	}
	branchCreated := 0
	if s.BranchCreated {
		branchCreated = 1
	}
	_, err := d.sql.Exec(
		`INSERT INTO sessions (id, name, tmux_name, working_directory, provider_session_id, model, system_prompt, provider_type, auto_approve, worktree_branch, git_parent_dir, git_remote_url, profile, branch_created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.TmuxName, s.WorkingDirectory,
		s.ProviderSessionID, s.Model, s.SystemPrompt,
		s.ProviderType, autoApprove, s.WorktreeBranch,
		s.GitParentDir, s.GitRemoteURL, s.Profile, branchCreated,
	)
	if err != nil {
		return fmt.Errorf("create session %s: %w", s.ID, err)
	}
	return nil
}

func (d *DB) GetSession(id string) (*Session, error) {
	row := d.sql.QueryRow(`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (d *DB) ListSessions(ctx context.Context) ([]*Session, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+sessionColumns+` FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

type SessionUpdate struct {
	Name              *string `json:"name,omitempty"`
	TmuxName          *string `json:"tmux_name,omitempty"`
	ProviderSessionID *string `json:"provider_session_id,omitempty"`
	WorkingDirectory  *string `json:"working_directory,omitempty"`
}

func (d *DB) UpdateSession(id string, u SessionUpdate) (*Session, error) {
	// Build dynamic update
	sets := []string{}
	args := []any{}

	if u.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *u.Name)
	}
	if u.TmuxName != nil {
		sets = append(sets, "tmux_name = ?")
		args = append(args, *u.TmuxName)
	}
	if u.ProviderSessionID != nil {
		sets = append(sets, "provider_session_id = ?")
		args = append(args, *u.ProviderSessionID)
	}
	if u.WorkingDirectory != nil {
		sets = append(sets, "working_directory = ?")
		args = append(args, *u.WorkingDirectory)
	}

	if len(sets) == 0 {
		return d.GetSession(id)
	}

	sets = append(sets, "updated_at = datetime('now')")
	args = append(args, id)

	query := "UPDATE sessions SET "
	for i, s := range sets {
		if i > 0 {
			query += ", "
		}
		query += s
	}
	query += " WHERE id = ? RETURNING " + sessionColumns

	row := d.sql.QueryRow(query, args...)
	s, err := scanSession(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// CountSessionsByWorkingDir returns the number of sessions (excluding the
// given session ID) that share the same working directory. Used to determine
// whether it's safe to remove a worktree on session deletion.
func (d *DB) CountSessionsByWorkingDir(excludeID, workingDir string) (int, error) {
	var count int
	err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE id != ? AND working_directory = ?`,
		excludeID, workingDir,
	).Scan(&count)
	return count, err
}

// TouchSession sets updated_at to the given Unix timestamp (seconds).
// The WHERE guard skips the write when the stored value is already >= the
// supplied timestamp, so repeated calls with the same value are no-ops.
func (d *DB) TouchSession(ctx context.Context, id string, unixTS int64) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET updated_at = datetime(?, 'unixepoch')
		 WHERE id = ? AND updated_at < datetime(?, 'unixepoch')`,
		unixTS, id, unixTS,
	)
	return err
}

func (d *DB) DeleteSession(id string) error {
	result, err := d.sql.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}

// ListSessionsForBackfill returns sessions missing git_parent_dir.
// Used for backfill after migration — covers both worktree and
// non-worktree sessions in git repos.
func (d *DB) ListSessionsForBackfill() ([]*Session, error) {
	rows, err := d.sql.Query(
		`SELECT ` + sessionColumns + ` FROM sessions WHERE git_parent_dir IS NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// SetGitParentDir sets the git_parent_dir for a session.
func (d *DB) SetGitParentDir(id, dir string) error {
	_, err := d.sql.Exec(
		`UPDATE sessions SET git_parent_dir = ? WHERE id = ?`,
		dir, id,
	)
	return err
}

// ListSessionsForGitRemoteBackfill returns sessions known to be in git repos
// (have git_parent_dir or worktree_branch) but missing git_remote_url.
func (d *DB) ListSessionsForGitRemoteBackfill() ([]*Session, error) {
	rows, err := d.sql.Query(
		`SELECT ` + sessionColumns + ` FROM sessions WHERE git_remote_url IS NULL AND (git_parent_dir IS NOT NULL OR worktree_branch IS NOT NULL)`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// TransferBranchOwnership transfers the branch_created flag from the session
// being deleted to one sibling session sharing the same working directory and
// worktree branch. Only worktree-backed sessions (worktree_branch IS NOT NULL)
// are eligible recipients, since shell sessions skip branch cleanup on delete.
func (d *DB) TransferBranchOwnership(excludeID, workingDir, branch string) error {
	_, err := d.sql.Exec(
		`UPDATE sessions SET branch_created = 1
		 WHERE id = (
		   SELECT id FROM sessions
		   WHERE id != ? AND working_directory = ? AND worktree_branch = ?
		   ORDER BY created_at ASC, id ASC
		   LIMIT 1
		 )`,
		excludeID, workingDir, branch,
	)
	return err
}

// SetGitRemoteURL sets the git_remote_url for a session.
func (d *DB) SetGitRemoteURL(id, url string) error {
	_, err := d.sql.Exec(
		`UPDATE sessions SET git_remote_url = ? WHERE id = ?`,
		url, id,
	)
	return err
}

// SetUnreadSince sets or clears the unread_since timestamp.
// Pass nil to clear (mark as read).
func (d *DB) SetUnreadSince(ctx context.Context, id string, ts *string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET unread_since = ? WHERE id = ?`,
		ts, id,
	)
	return err
}

// SetLastViewedAt updates the last_viewed_at timestamp.
func (d *DB) SetLastViewedAt(ctx context.Context, id, ts string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET last_viewed_at = ? WHERE id = ?`,
		ts, id,
	)
	return err
}

// TouchLastViewedAt sets last_viewed_at to the current time.
func (d *DB) TouchLastViewedAt(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET last_viewed_at = datetime('now') WHERE id = ?`,
		id,
	)
	return err
}

// AcknowledgeSession clears unread_since and sets last_viewed_at to now.
func (d *DB) AcknowledgeSession(ctx context.Context, id string) error {
	_, err := d.sql.ExecContext(ctx,
		`UPDATE sessions SET unread_since = NULL, last_viewed_at = datetime('now') WHERE id = ?`,
		id,
	)
	return err
}
