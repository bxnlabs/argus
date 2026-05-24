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

// TmuxSocketPath returns the path to Argus's dedicated tmux server socket:
// <StateDir>/tmux/server. Honors ARGUS_HOME so the dev stack is isolated.
func TmuxSocketPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmux", "server"), nil
}

// TmuxConfigPath returns the path to Argus's tmux config: <StateDir>/tmux/tmux.conf.
func TmuxConfigPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tmux", "tmux.conf"), nil
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

// SeedTmuxConfig ensures <StateDir>/tmux exists and writes the default
// tmux.conf only when it is missing, so a user-edited config is never
// overwritten. Returns the config path; the directory creation it performs is
// also what lets tmux create the socket there on first new-session.
func SeedTmuxConfig() (string, error) {
	confPath, err := TmuxConfigPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(confPath), 0700); err != nil {
		return "", fmt.Errorf("create tmux dir: %w", err)
	}
	f, err := os.OpenFile(confPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	switch {
	case err == nil:
		defer f.Close()
		if _, werr := f.WriteString(baseTmuxConfig); werr != nil {
			return "", fmt.Errorf("write tmux config: %w", werr)
		}
	case errors.Is(err, os.ErrExist):
		// Existing file (possibly user-edited) — leave it untouched.
	default:
		return "", fmt.Errorf("open tmux config: %w", err)
	}
	return confPath, nil
}
