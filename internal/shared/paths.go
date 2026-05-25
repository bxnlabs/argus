package shared

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ to the user's home directory.
func ExpandPath(p string) (string, error) {
	if len(p) == 0 {
		return p, nil
	}
	if p[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home: %w", err)
		}
		return filepath.Join(home, p[1:]), nil
	}
	return p, nil
}

// EnsureSecureDir creates dir (and any missing parents) with 0700 permissions.
// If dir already exists with broader permissions, it is tightened to 0700.
// MkdirAll only applies the mode to directories it creates, so a directory left
// over from a looser umask (or pre-created by another user) would otherwise keep
// its permissions and expose the private state Argus stores inside. Use this for
// any directory holding sockets, tokens, indexes, or other per-user state.
func EnsureSecureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// StateDir returns the root directory for Argus's per-user state: config
// file, database, discovery file, project worktrees, and the search ignore
// file. It honors the ARGUS_HOME environment variable when set; otherwise it
// defaults to ~/.argus. ARGUS_HOME lets a local dev stack run fully isolated
// from a production instance on the same machine.
//
// ARGUS_HOME must be absolute (or start with ~). Callers join subpaths onto
// the result directly without re-expanding, so a relative override would
// resolve against each process's working directory — splitting state between
// the server and the CLI when they're launched from different directories.
// Expanding ~ and rejecting relative paths guarantees every consumer agrees
// on a single location.
func StateDir() (string, error) {
	if dir := os.Getenv("ARGUS_HOME"); dir != "" {
		expanded, err := ExpandPath(dir)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(expanded) {
			return "", fmt.Errorf("ARGUS_HOME must be an absolute path or start with ~, got %q", dir)
		}
		return filepath.Clean(expanded), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine state dir: %w", err)
	}
	return filepath.Join(home, ".argus"), nil
}

// CleanPath expands ~ and resolves the path to an absolute, cleaned form.
// Unlike SafeExpandPath, it does not restrict paths to the home directory.
// OS-level permissions are the sole access guard. This is acceptable under
// the assumption that the node runs behind Tailscale or a private network
// with no unauthenticated public exposure. See TODOS.md for hardening items
// (auth middleware, localhost-only binding) before any broader deployment.
func CleanPath(p string) (string, error) {
	if len(p) == 0 {
		return "", fmt.Errorf("path is required")
	}

	expanded, err := ExpandPath(p)
	if err != nil {
		return "", err
	}

	abs, err := filepath.Abs(filepath.Clean(expanded))
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	return abs, nil
}

// SafeExpandPath expands ~ and resolves the path to an absolute, cleaned form,
// then validates that the result is within the user's home directory.
// This prevents path traversal attacks (e.g., ../../../etc/passwd) and
// symlink-based bypasses (e.g., ~/evil-link -> /etc).
func SafeExpandPath(p string) (string, error) {
	abs, err := CleanPath(p)
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expand home: %w", err)
	}
	cleanHome := filepath.Clean(home)

	// Lexical check: fast-reject paths that are obviously outside home.
	if abs != cleanHome && !strings.HasPrefix(abs, cleanHome+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside home directory %q", abs, cleanHome)
	}

	// Resolve symlinks so the real target is validated, not just the
	// lexical path. filepath.Abs is purely lexical and won't catch
	// ~/evil-link -> /etc style bypasses.
	resolved, err := EvalSymlinks(abs)
	if err != nil {
		return "", err
	}

	if resolved != cleanHome && !strings.HasPrefix(resolved, cleanHome+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q resolves to %q which is outside home directory %q", abs, resolved, cleanHome)
	}

	// Return the original abs path to preserve symlink names in user-facing paths.
	return abs, nil
}

// EvalSymlinks resolves symlinks like filepath.EvalSymlinks but handles
// non-existent paths by resolving the nearest existing ancestor and
// reattaching the remaining (non-existent) path components.
func EvalSymlinks(p string) (string, error) {
	resolved, err := filepath.EvalSymlinks(p)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}

	// Path doesn't exist yet. Walk up to find the nearest existing ancestor,
	// resolve it, then rejoin the non-existent tail.
	parent := filepath.Dir(p)
	tail := filepath.Base(p)
	for parent != "/" && parent != "." {
		resolvedParent, err := filepath.EvalSymlinks(parent)
		if err == nil {
			return filepath.Join(resolvedParent, tail), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve symlinks: %w", err)
		}
		tail = filepath.Join(filepath.Base(parent), tail)
		parent = filepath.Dir(parent)
	}
	return "", fmt.Errorf("resolve symlinks: no existing ancestor for %s", p)
}
