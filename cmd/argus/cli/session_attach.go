package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <name-or-id>",
		Short: "Attach to a session's tmux",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
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
			// so the node revives the tmux session if it died.
			_, err = c.get("/api/sessions/" + session.ID)
			if err != nil {
				return fmt.Errorf("ensure session: %w", err)
			}

			// Acknowledge unread state before attaching
			_, _ = c.post("/api/sessions/"+session.ID+"/acknowledge", nil)

			return attachTmux(session.ID, session.TmuxName, c.baseURL)
		},
	}
}

// attachTmux runs tmux attach-session as a subprocess with a heartbeat goroutine.
func attachTmux(sessionID, tmuxName, baseURL string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	tmuxCmd := exec.Command(tmuxPath, "attach-session", "-t", tmuxName)
	tmuxCmd.Stdin = os.Stdin
	tmuxCmd.Stdout = os.Stdout
	tmuxCmd.Stderr = os.Stderr

	if err := tmuxCmd.Start(); err != nil {
		return fmt.Errorf("start tmux: %w", err)
	}

	// Start heartbeat goroutine
	ctx, cancel := context.WithCancel(context.Background())
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		runHeartbeat(ctx, sessionID, baseURL)
	}()

	// Wait for tmux to exit (user detaches or session ends)
	err = tmuxCmd.Wait()

	// Stop heartbeat
	cancel()
	<-heartbeatDone

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("tmux: %w", err)
	}
	return nil
}

// runHeartbeat sends periodic heartbeat requests while the context is active.
// Sends one immediate heartbeat before entering the ticker loop so that
// last_viewed_at is fresh from the start (covers activity in the first 2s).
func runHeartbeat(ctx context.Context, sessionID, baseURL string) {
	client := &http.Client{Timeout: 2 * time.Second}
	url := baseURL + "/api/sessions/" + sessionID + "/heartbeat"

	// Immediate heartbeat at attach start
	if req, err := http.NewRequestWithContext(ctx, "POST", url, nil); err == nil {
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
		}
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
			if err != nil {
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				continue // heartbeat failures are silent
			}
			resp.Body.Close()
		}
	}
}
