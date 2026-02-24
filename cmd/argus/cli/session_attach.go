package cli

import (
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
