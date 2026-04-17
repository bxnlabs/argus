package db

import (
	"database/sql"
	"fmt"

	"github.com/bxnlabs/argus/internal/shared/sqlite"
)

// DB wraps a SQLite database with Argus-specific operations.
type DB struct {
	sql *sql.DB
}

// Open opens the database and runs schema creation.
func Open(path string) (*DB, error) {
	sqlDB, err := sqlite.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	d := &DB{sql: sqlDB}

	if _, err := d.sql.Exec(schema); err != nil {
		d.sql.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Seed migrations that are already reflected in the schema so that
	// RunMigrations skips them on fresh databases.
	if err := d.seedMigrations(); err != nil {
		d.sql.Close()
		return nil, fmt.Errorf("seed migrations: %w", err)
	}

	return d, nil
}

// seedMigrations pre-marks schema-embedded migrations as applied for fresh
// databases where the columns are already present in the CREATE TABLE statement.
// On an existing database that lacks the column, this is a no-op so that
// RunMigrations can apply the ALTER TABLE normally.
func (d *DB) seedMigrations() error {
	rows, err := d.sql.Query(`PRAGMA table_info(sessions)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var hasWorktreeBranch, hasGitParentDir, hasGitRemoteURL, hasProfile, hasProviderType, hasUnreadSince, hasLastViewedAt bool
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return err
		}
		switch name {
		case "worktree_branch":
			hasWorktreeBranch = true
		case "git_parent_dir":
			hasGitParentDir = true
		case "git_remote_url":
			hasGitRemoteURL = true
		case "profile":
			hasProfile = true
		case "provider_type":
			hasProviderType = true
		case "unread_since":
			hasUnreadSince = true
		case "last_viewed_at":
			hasLastViewedAt = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Check if notifications table exists (fresh schema includes it).
	var notifTableCount int
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='notifications'`)
	if err := row.Scan(&notifTableCount); err != nil {
		return err
	}

	if hasWorktreeBranch {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"add_worktree_branch",
		); err != nil {
			return err
		}
	}
	if hasGitParentDir {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"add_git_parent_dir",
		); err != nil {
			return err
		}
	}
	if hasGitRemoteURL {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"add_git_remote_url",
		); err != nil {
			return err
		}
	}
	if hasProfile {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"add_profile",
		); err != nil {
			return err
		}
	}
	if hasProviderType {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"rename_agent_type_to_provider_type",
		); err != nil {
			return err
		}
	}
	if hasUnreadSince && hasLastViewedAt {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"add_unread_since_and_last_viewed_at",
		); err != nil {
			return err
		}
	}
	if notifTableCount > 0 {
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
			"create_notifications_table",
		); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.sql.Close()
}
