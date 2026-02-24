package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func runAttach(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("session name or ID required\n\nUsage: argus session attach <name-or-id>")
	}
	query := args[0]

	path, err := discoveryFilePath()
	if err != nil {
		return err
	}
	c, err := newClient(path)
	if err != nil {
		return err
	}

	session, err := fetchAndResolve(c, query)
	if err != nil {
		return err
	}

	// Call EnsureSession via the GET /api/sessions/{id} endpoint
	// so the agent revives the tmux session if it died.
	_, err = c.get("/api/sessions/" + session.ID)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}

	return attachTmux(session.TmuxName)
}

// attachTmux replaces the current process with tmux attach-session.
func attachTmux(tmuxName string) error {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmux, []string{"tmux", "attach-session", "-t", tmuxName}, os.Environ())
}
