package cli

import (
	"fmt"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/shared"
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
		newDescribeCmd(),
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
	stateDir, err := shared.StateDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine state dir: %w", err)
	}
	return filepath.Join(stateDir, "node.json"), nil
}
