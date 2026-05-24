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
// databases where the columns/tables are already present in the base schema.
// On an existing database that lacks an artifact, that migration is left
// pending so that CheckMigrations reports it and RunMigrations applies it.
func (d *DB) seedMigrations() error {
	// Check whether any migrations have previously been recorded. An empty
	// _migrations table means this is either a brand-new database or one that
	// predates the migration system entirely. Combined with the column checks
	// below, this distinguishes fresh DBs from old ones.
	var priorMigrations int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM _migrations`).Scan(&priorMigrations); err != nil {
		return err
	}

	rows, err := d.sql.Query(`PRAGMA table_info(sessions)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var hasWorktreeBranch, hasBranchCreated, hasGitParentDir, hasGitRemoteURL, hasProfile, hasProviderType, hasUnreadSince, hasLastViewedAt, hasPinned, hasMarkedUnreadAt bool
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
		case "branch_created":
			hasBranchCreated = true
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
		case "pinned":
			hasPinned = true
		case "marked_unread_at":
			hasMarkedUnreadAt = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Seed each column migration whose artifact already exists.
	seeds := []struct {
		condition bool
		name      string
	}{
		{hasWorktreeBranch, "add_worktree_branch"},
		{hasGitParentDir, "add_git_parent_dir"},
		{hasGitRemoteURL, "add_git_remote_url"},
		{hasProfile, "add_profile"},
		{hasBranchCreated, "add_branch_created"},
		{hasProviderType, "rename_agent_type_to_provider_type"},
		{hasUnreadSince && hasLastViewedAt, "add_unread_since_and_last_viewed_at"},
		{hasPinned, "add_pinned"},
		{hasMarkedUnreadAt, "add_marked_unread_at"},
	}
	allColumnsPresent := true
	for _, s := range seeds {
		if !s.condition {
			allColumnsPresent = false
			continue
		}
		if _, err := d.sql.Exec(
			`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`, s.name,
		); err != nil {
			return err
		}
	}

	// The notifications table is in the base schema (CREATE TABLE IF NOT
	// EXISTS), so it exists on both fresh and existing databases. Only seed
	// it on a truly fresh database. A fresh database has:
	//   1. No prior migration records (_migrations was empty)
	//   2. All session columns present (base schema just created them)
	// An existing database with all columns but prior migration records is
	// an upgrade — let CheckMigrations flag create_notifications_table.
	if priorMigrations == 0 && allColumnsPresent {
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
