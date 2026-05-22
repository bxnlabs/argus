package api

import (
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/source"
)

// resolveProjectDir returns the on-disk directory where per-project state
// (reviews, etc.) is stored. If override is non-empty, it short-circuits
// the home-dir derivation — used by tests.
func resolveProjectDir(expandedPath, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	parentKey := source.ParentKeyFromPath(expandedPath)
	return filepath.Join(home, ".argus", "projects", parentKey), nil
}
