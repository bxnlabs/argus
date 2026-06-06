package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// newProfileCmd returns the "profile" command group for managing a session's
// profile.
func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage a session's profile",
	}
	cmd.AddCommand(newProfileSetCmd(), newProfileRmCmd())
	return cmd
}

func newProfileSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <sess> <profile>",
		Short: "Set or change a session's profile (restarts the session)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			profile := args[1]
			return applyProfile(args[0], &profile)
		},
	}
}

func newProfileRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <sess>",
		Short: "Detach the profile from a session (restarts the session)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			return applyProfile(args[0], nil)
		},
	}
}

// applyProfile resolves the session matching query and sets its profile to the
// given value (nil detaches). When the profile already equals the requested
// value it is a no-op: the request and the session restart are skipped.
func applyProfile(query string, profile *string) error {
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

	// No-op when the profile is unchanged: nothing happens, so skip the
	// request and the session restart entirely.
	if samePtr(profile, session.Profile) {
		if profile == nil {
			fmt.Printf("Session %q has no profile; no change\n", session.Name)
		} else {
			fmt.Printf("Session %q already uses profile %q; no change\n", session.Name, *profile)
		}
		return nil
	}

	if profile == nil {
		if _, err := c.delete("/sessions/" + session.ID + "/profile"); err != nil {
			return err
		}
		fmt.Printf("Cleared profile on session %q (session restarted)\n", session.Name)
		return nil
	}

	reqBody, err := json.Marshal(map[string]any{"profile": *profile})
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	if _, err := c.put("/sessions/"+session.ID+"/profile", bytes.NewReader(reqBody)); err != nil {
		return err
	}
	fmt.Printf("Set profile %q on session %q (session restarted)\n", *profile, session.Name)
	return nil
}

// samePtr reports whether two optional strings hold the same value, treating
// nil and "" as distinct (nil = no profile).
func samePtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
