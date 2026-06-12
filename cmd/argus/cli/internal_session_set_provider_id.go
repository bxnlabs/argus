package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newSetProviderIDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-provider-id <session-id> <provider-session-id>",
		Short: "Persist a provider session ID for resume support",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sessionID := args[0]
			providerSessionID := args[1]

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			reqBody, err := json.Marshal(map[string]string{
				"provider_session_id": providerSessionID,
			})
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}
			if _, err := c.patch("/sessions/"+sessionID, bytes.NewReader(reqBody)); err != nil {
				return err
			}

			return nil
		},
	}
}
