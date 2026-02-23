package db

// RunMigrations runs any pending schema migrations.
// For v2 fresh start, there are no migrations yet.
func (d *DB) RunMigrations() error {
	// Future migrations go here. Each checks _migrations table
	// before applying to ensure idempotency.
	return nil
}
