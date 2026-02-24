package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func runAttach(args []string) error {
	fs := flag.NewFlagSet("argus session attach", flag.ExitOnError)
	cc := fs.Bool("cc", false, "Use tmux control mode (-CC)")
	fs.Parse(args)

	if fs.NArg() == 0 {
		return fmt.Errorf("session name or ID required\n\nUsage: argus session attach [--cc] <name-or-id>")
	}
	query := fs.Arg(0)

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	// Fetch all sessions to resolve the query.
	body, err := c.get("/api/sessions")
	if err != nil {
		return err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	session, err := resolveSession(resp.Sessions, query)
	if err != nil {
		return err
	}

	// Call EnsureSession via the GET /api/sessions/{id} endpoint
	// so the agent revives the tmux session if it died.
	_, err = c.get("/api/sessions/" + session.ID)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}

	// Find tmux binary.
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// Build tmux args.
	var tmuxArgs []string
	if *cc {
		tmuxArgs = []string{"tmux", "-CC", "attach-session", "-t", session.TmuxName}
	} else {
		tmuxArgs = []string{"tmux", "attach-session", "-t", session.TmuxName}
	}

	// Replace the current process with tmux.
	return syscall.Exec(tmux, tmuxArgs, os.Environ())
}
