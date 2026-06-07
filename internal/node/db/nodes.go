package db

import (
	"context"
	"fmt"
	"strings"
)

// ManualNode is a user-added peer node persisted in the registry.
type ManualNode struct {
	ID        string
	Name      string
	URL       string
	CreatedAt string
}

// ListManualNodes returns all manually-added nodes, oldest first.
func (d *DB) ListManualNodes(ctx context.Context) ([]ManualNode, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, name, url, created_at FROM nodes ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []ManualNode
	for rows.Next() {
		var n ManualNode
		if err := rows.Scan(&n.ID, &n.Name, &n.URL, &n.CreatedAt); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// AddManualNode inserts a node. The UNIQUE(url) constraint surfaces duplicates
// as ErrDuplicateURL.
func (d *DB) AddManualNode(ctx context.Context, id, name, url string) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO nodes (id, name, url) VALUES (?, ?, ?)`, id, name, url)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("%w: %s", ErrDuplicateURL, url)
		}
		return err
	}
	return nil
}

// RenameManualNode updates a node's display name.
func (d *DB) RenameManualNode(ctx context.Context, id, name string) error {
	res, err := d.sql.ExecContext(ctx,
		`UPDATE nodes SET name = ? WHERE id = ?`, name, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}

// DeleteManualNode removes a node by id.
func (d *DB) DeleteManualNode(ctx context.Context, id string) error {
	res, err := d.sql.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}
