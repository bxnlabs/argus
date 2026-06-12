package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newRenameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <name-or-id> <new-name>",
		Short: "Rename a session",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			query := args[0]
			newName := args[1]

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

			reqBody, err := json.Marshal(map[string]string{"name": newName})
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}
			if _, err := c.patch("/sessions/"+session.ID, bytes.NewReader(reqBody)); err != nil {
				return err
			}

			fmt.Printf("Renamed session %q → %q\n", session.Name, newName)
			return nil
		},
	}
}
