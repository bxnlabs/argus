package api

import (
	"path/filepath"

	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
)

// resolveProjectDir returns the on-disk directory where per-project state
// (reviews, etc.) is stored. If override is non-empty, it short-circuits
// the state-dir derivation — used by tests.
func resolveProjectDir(expandedPath, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	stateDir, err := shared.StateDir()
	if err != nil {
		return "", err
	}
	parentKey := source.ParentKeyFromPath(expandedPath)
	return filepath.Join(stateDir, "projects", parentKey), nil
}
