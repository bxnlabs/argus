package cli

import "github.com/spf13/cobra"

// NewGitCmd returns the top-level "git" command with its subcommands.
func NewGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Git-related tools",
		// Override root's PersistentPreRunE — git commands talk to the node via
		// the discovery file and don't need config loading.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(newCommentsCmd())
	cmd.AddCommand(newWtCmd())
	return cmd
}
