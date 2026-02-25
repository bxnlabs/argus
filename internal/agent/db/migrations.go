package db

// RunMigrations runs any pending schema migrations.
func (d *DB) RunMigrations() error {
	return d.migrate("add_worktree_branch", func() error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN worktree_branch TEXT`)
		return err
	})
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
