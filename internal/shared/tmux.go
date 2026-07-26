package shared

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// baseTmuxConfig is the default config seeded for Argus's dedicated tmux
// server. It is written once (see SeedTmuxConfig) and then owned by the user;
// Argus never overwrites it. It restores the rendering and styling Argus
// previously inherited from the user's ~/.tmux.conf on the shared server.
const baseTmuxConfig = `# Argus tmux defaults — seeded once. Edit to customize; Argus won't overwrite.
set -g default-terminal "tmux-256color"
set -ga terminal-overrides ",*:Tc"
set -g mouse on
set -g status-style "bg=#1e1e2e,fg=#cdd6f4"
set -g status-left "#[fg=#cba6f7,bold] Argus #[fg=#6c7086]| "
set -g status-left-length 20
set -g status-right-length 110
set -g status-position bottom
`

// tmuxStateDir returns Argus's dedicated tmux directory: <StateDir>/tmux.
// Honors ARGUS_HOME so the dev stack is isolated.
func tmuxStateDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmux"), nil
}

// TmuxSocketPath returns the path to Argus's dedicated tmux server socket:
// <StateDir>/tmux/server.
//
// It rejects a path that cannot fit in sockaddr_un.sun_path (see
// maxTmuxSocketPath). tmux reports that case as "File name too long" against a
// path it only prints in passing, which reads as a missing-file problem rather
// than a length one; failing here instead names the limit at the single point
// every tmux caller goes through.
func TmuxSocketPath() (string, error) {
	dir, err := tmuxStateDir()
	if err != nil {
		return "", err
	}
	sock := filepath.Join(dir, "server")
	if len(sock) > maxTmuxSocketPath {
		return "", fmt.Errorf(
			"tmux socket path %q is %d bytes, over this platform's %d-byte unix socket limit; set ARGUS_HOME to a shorter path",
			sock, len(sock), maxTmuxSocketPath)
	}
	return sock, nil
}

// TmuxConfigPath returns the path to Argus's tmux config: <StateDir>/tmux/tmux.conf.
func TmuxConfigPath() (string, error) {
	dir, err := tmuxStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmux.conf"), nil
}

// EnsureTmuxStateDir creates Argus's dedicated tmux directory (<StateDir>/tmux)
// if missing and returns it. tmux places the dedicated server's socket here on
// first new-session, so this must succeed before any session can be created.
func EnsureTmuxStateDir() (string, error) {
	dir, err := tmuxStateDir()
	if err != nil {
		return "", err
	}
	if err := EnsureSecureDir(dir); err != nil {
		return "", fmt.Errorf("create tmux dir: %w", err)
	}
	return dir, nil
}

// TmuxCommand builds an *exec.Cmd that targets Argus's dedicated tmux server
// socket. It is the single place the -S flag is threaded; all callers (node
// and CLI) build their tmux commands through it.
func TmuxCommand(args ...string) (*exec.Cmd, error) {
	sock, err := TmuxSocketPath()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-S", sock}, args...)
	return exec.Command("tmux", full...), nil
}

// TmuxCommandContext is TmuxCommand with a context for cancellation/timeout.
func TmuxCommandContext(ctx context.Context, args ...string) (*exec.Cmd, error) {
	sock, err := TmuxSocketPath()
	if err != nil {
		return nil, err
	}
	full := append([]string{"-S", sock}, args...)
	return exec.CommandContext(ctx, "tmux", full...), nil
}

// SeedTmuxConfig writes the default tmux.conf into Argus's tmux directory only
// when it is missing, so a user-edited config is never overwritten. The
// directory must already exist (see EnsureTmuxStateDir). Returns the config path.
func SeedTmuxConfig() (string, error) {
	confPath, err := TmuxConfigPath()
	if err != nil {
		return "", err
	}

	// Write the defaults to a temp file in the same directory and publish it
	// with a hard link. The link is atomic and fails with EEXIST when a
	// (possibly user-edited) config already exists, so we never overwrite it —
	// and a crash mid-write can only leave an orphan temp, never a truncated
	// tmux.conf that a later run would mistake for the user's own.
	tmp, err := os.CreateTemp(filepath.Dir(confPath), ".tmux.conf-*")
	if err != nil {
		return "", fmt.Errorf("create temp tmux config: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(baseTmuxConfig); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write tmux config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", fmt.Errorf("sync tmux config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close tmux config: %w", err)
	}

	if err := os.Link(tmp.Name(), confPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return confPath, nil
		}
		return "", fmt.Errorf("publish tmux config: %w", err)
	}
	return confPath, nil
}
