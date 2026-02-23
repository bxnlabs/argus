package db

import (
	"database/sql"
	"fmt"
)

// sessionColumns is the explicit column list matching scanSession's scan order.
const sessionColumns = `id, name, tmux_name, created_at, updated_at,
	working_directory, provider_session_id, model, system_prompt,
	agent_type, auto_approve`

func scanSession(row interface{ Scan(...any) error }) (*Session, error) {
	var s Session
	var autoApprove int
	err := row.Scan(
		&s.ID, &s.Name, &s.TmuxName, &s.CreatedAt, &s.UpdatedAt,
		&s.WorkingDirectory,
		&s.ProviderSessionID, &s.Model, &s.SystemPrompt,
		&s.AgentType, &autoApprove,
	)
	if err != nil {
		return nil, err
	}
	s.AutoApprove = autoApprove != 0
	return &s, nil
}

func (d *DB) CreateSession(s *Session) error {
	autoApprove := 0
	if s.AutoApprove {
		autoApprove = 1
	}
	_, err := d.sql.Exec(
		`INSERT INTO sessions (id, name, tmux_name, working_directory, provider_session_id, model, system_prompt, agent_type, auto_approve)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.TmuxName, s.WorkingDirectory,
		s.ProviderSessionID, s.Model, s.SystemPrompt,
		s.AgentType, autoApprove,
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

func (d *DB) ListSessions() ([]*Session, error) {
	rows, err := d.sql.Query(`SELECT ` + sessionColumns + ` FROM sessions ORDER BY updated_at DESC`)
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
	Name             *string `json:"name,omitempty"`
	TmuxName         *string `json:"tmux_name,omitempty"`
	ProviderSessionID  *string `json:"provider_session_id,omitempty"`
	WorkingDirectory *string `json:"working_directory,omitempty"`
}

func (d *DB) UpdateSession(id string, u SessionUpdate) error {
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
		return nil
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
	query += " WHERE id = ?"

	result, err := d.sql.Exec(query, args...)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
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
