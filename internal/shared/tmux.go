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
func TmuxSocketPath() (string, error) {
	dir, err := tmuxStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "server"), nil
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
	if err := os.MkdirAll(dir, 0700); err != nil {
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
	f, err := os.OpenFile(confPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	switch {
	case err == nil:
		_, werr := f.WriteString(baseTmuxConfig)
		cerr := f.Close()
		if werr != nil {
			// Remove the empty/partial file so a later run re-seeds it rather
			// than treating the truncated config as the user's own.
			os.Remove(confPath)
			return "", fmt.Errorf("write tmux config: %w", werr)
		}
		if cerr != nil {
			return "", fmt.Errorf("close tmux config: %w", cerr)
		}
	case errors.Is(err, os.ErrExist):
		// Existing file (possibly user-edited) — leave it untouched.
	default:
		return "", fmt.Errorf("open tmux config: %w", err)
	}
	return confPath, nil
}
