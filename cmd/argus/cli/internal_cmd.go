package cli

import "github.com/spf13/cobra"

// NewInternalCmd returns the "internal" parent command for non-user-facing operations.
func NewInternalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal commands (not user-facing)",
		Hidden: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Internal session operations",
	}
	sessionCmd.AddCommand(
		newSetProviderIDCmd(),
	)

	cmd.AddCommand(sessionCmd)

	return cmd
}
