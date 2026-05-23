package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newProfileCmd() *cobra.Command {
	var clear bool

	cmd := &cobra.Command{
		Use:   "profile <name-or-id> [profile]",
		Short: "Set, change, or clear a session's profile (restarts the session)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			query := args[0]

			var profile *string
			if clear {
				if len(args) == 2 {
					return fmt.Errorf("cannot pass a profile name together with --clear")
				}
			} else {
				if len(args) != 2 {
					return fmt.Errorf("provide a profile name, or use --clear to detach")
				}
				p := args[1]
				profile = &p
			}

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

			reqBody, err := json.Marshal(map[string]any{"profile": profile})
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}
			if _, err := c.put("/api/sessions/"+session.ID+"/profile", bytes.NewReader(reqBody)); err != nil {
				return err
			}

			if profile == nil {
				fmt.Printf("Cleared profile on session %q (session restarted)\n", session.Name)
			} else {
				fmt.Printf("Set profile %q on session %q (session restarted)\n", *profile, session.Name)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&clear, "clear", false, "Detach the profile from the session")
	return cmd
}
