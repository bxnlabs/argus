package db

import (
	"fmt"
	"strings"
)

// migration defines a named schema migration with its apply function.
type migration struct {
	name string
	run  func(d *DB) error
}

// allMigrations is the single source of truth for all schema migrations,
// used by both RunMigrations (to apply) and CheckMigrations (to verify).
var allMigrations = []migration{
	{"add_worktree_branch", func(d *DB) error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN worktree_branch TEXT`)
		return err
	}},
	{"add_git_parent_dir", func(d *DB) error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN git_parent_dir TEXT`)
		return err
	}},
	{"add_profile", func(d *DB) error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN profile TEXT`)
		return err
	}},
	{"add_git_remote_url", func(d *DB) error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN git_remote_url TEXT`)
		return err
	}},
	{"add_branch_created", func(d *DB) error {
		return d.addColumnIfMissing("branch_created",
			`ALTER TABLE sessions ADD COLUMN branch_created INTEGER NOT NULL DEFAULT 0`)
	}},
	{"rename_agent_type_to_provider_type", func(d *DB) error {
		// Only rename if the old column still exists (no-op for fresh databases
		// created with provider_type directly).
		var hasOldColumn int
		row := d.sql.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = 'agent_type'`,
		)
		if err := row.Scan(&hasOldColumn); err != nil {
			return err
		}
		if hasOldColumn == 0 {
			return nil // already using provider_type
		}
		_, err := d.sql.Exec(`ALTER TABLE sessions RENAME COLUMN agent_type TO provider_type`)
		return err
	}},
	{"add_unread_since_and_last_viewed_at", func(d *DB) error {
		if err := d.addColumnIfMissing("unread_since",
			`ALTER TABLE sessions ADD COLUMN unread_since TEXT`); err != nil {
			return err
		}
		return d.addColumnIfMissing("last_viewed_at",
			`ALTER TABLE sessions ADD COLUMN last_viewed_at TEXT`)
	}},
	{"create_notifications_table", func(d *DB) error {
		if _, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			sent_at TEXT NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`); err != nil {
			return err
		}
		_, err := d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_notifications_session_sent_at
			ON notifications(session_id, sent_at)`)
		return err
	}},
	{"add_pinned", func(d *DB) error {
		return d.addColumnIfMissing("pinned",
			`ALTER TABLE sessions ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0`)
	}},
	{"add_user_marked_unread_at", func(d *DB) error {
		return d.addColumnIfMissing("user_marked_unread_at",
			`ALTER TABLE sessions ADD COLUMN user_marked_unread_at TEXT`)
	}},
	{"add_nodes_table", func(d *DB) error {
		_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS nodes (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  url TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
)`)
		return err
	}},
}

// hasColumn reports whether the sessions table already has the named column.
func (d *DB) hasColumn(column string) (bool, error) {
	var count int
	row := d.sql.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = ?`,
		column,
	)
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// addColumnIfMissing runs the given ALTER TABLE only when the column is absent.
// This keeps multi-column migrations restartable after a partial failure and
// makes them no-ops when the column already exists in the base schema.
func (d *DB) addColumnIfMissing(column, alterSQL string) error {
	has, err := d.hasColumn(column)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = d.sql.Exec(alterSQL)
	return err
}

// CheckMigrations verifies that all expected migrations have been applied.
// Returns an error listing missing migrations if any are pending.
func (d *DB) CheckMigrations() error {
	rows, err := d.sql.Query(`SELECT name FROM _migrations`)
	if err != nil {
		return fmt.Errorf("check migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		applied[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	var missing []string
	for _, m := range allMigrations {
		if !applied[m.name] {
			missing = append(missing, m.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("database has pending migrations: %s\nRun 'argus migrate' to apply them", strings.Join(missing, ", "))
	}
	return nil
}

// RunMigrations runs any pending schema migrations.
func (d *DB) RunMigrations() error {
	for _, m := range allMigrations {
		if err := d.migrate(m.name, func() error {
			return m.run(d)
		}); err != nil {
			return err
		}
	}
	return nil
}

// migrate runs fn only if the named migration has not been applied.
func (d *DB) migrate(name string, fn func() error) error {
	var count int
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE name = ?`, name)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // already applied
	}
	if err := fn(); err != nil {
		return err
	}
	_, err := d.sql.Exec(`INSERT INTO _migrations (name) VALUES (?)`, name)
	return err
}
