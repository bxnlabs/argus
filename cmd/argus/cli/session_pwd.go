package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPwdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pwd <name-or-id>",
		Short: "Print the working directory of a session",
		Long: `Print the working directory of a session to stdout.

Useful for shell integration:

  acd() { cd "$(argus session pwd "$1")"; }`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}
			s, err := fetchAndResolve(c, args[0])
			if err != nil {
				return err
			}
			fmt.Println(s.WorkingDirectory)
			return nil
		},
	}
}
