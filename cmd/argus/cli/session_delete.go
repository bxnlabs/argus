package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "rm <name-or-id>",
		Short: "Delete a session",
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

			endpoint := "/api/sessions/" + session.ID
			if force {
				endpoint += "?force=true"
			}

			if _, err := c.delete(endpoint); err != nil {
				return err
			}

			fmt.Printf("Deleted session %q\n", session.Name)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force delete even if worktree has uncommitted changes")

	return cmd
}
