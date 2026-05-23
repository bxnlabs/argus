package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NewSessionCmd returns the "session" parent command with all subcommands.
func NewSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage sessions",
		Long:  "Create, list, attach, rename, and delete node sessions.",
		// Override root's PersistentPreRunE — session commands use
		// only the discovery file and don't need config loading.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	cmd.AddCommand(
		newListCmd(),
		newCreateCmd(),
		newAttachCmd(),
		newDeleteCmd(),
		newRenameCmd(),
		newProfileCmd(),
		newPwdCmd(),
	)

	return cmd
}

// discoveryFilePath returns the path to the node discovery file.
func discoveryFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".argus", "node.json"), nil
}
