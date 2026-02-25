package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
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

			if _, err := c.delete("/api/sessions/" + session.ID); err != nil {
				return err
			}

			fmt.Printf("Deleted session %q\n", session.Name)
			return nil
		},
	}
}
