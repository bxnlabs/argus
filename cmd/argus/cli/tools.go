package cli

import "github.com/spf13/cobra"

// NewToolsCmd returns the "tools" parent command with git subcommands.
func NewToolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Developer tools",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	gitCmd := &cobra.Command{
		Use:   "git",
		Short: "Git-related tools",
	}
	gitCmd.AddCommand(newReviewCmd())

	cmd.AddCommand(gitCmd)

	return cmd
}
