package db

import "context"

// UnreadSession holds the fields needed by the notification service.
type UnreadSession struct {
	ID               string
	Name             string
	ProviderType     string
	WorkingDirectory string
	UnreadSince      string
}

// UnreadSessions returns sessions where unread_since IS NOT NULL.
func (d *DB) UnreadSessions(ctx context.Context) ([]UnreadSession, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, name, provider_type, working_directory, unread_since
		 FROM sessions WHERE unread_since IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UnreadSession
	for rows.Next() {
		var s UnreadSession
		if err := rows.Scan(&s.ID, &s.Name, &s.ProviderType, &s.WorkingDirectory, &s.UnreadSince); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// HasNotification checks if a notification exists for the given session
// with sent_at > the provided timestamp. Used for deduplication.
func (d *DB) HasNotification(ctx context.Context, sessionID, after string) (bool, error) {
	var count int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE session_id = ? AND sent_at > ?`,
		sessionID, after,
	).Scan(&count)
	return count > 0, err
}

// InsertNotification records that a notification was sent for a session.
func (d *DB) InsertNotification(ctx context.Context, sessionID, sentAt string) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO notifications (session_id, sent_at) VALUES (?, ?)`,
		sessionID, sentAt,
	)
	return err
}
