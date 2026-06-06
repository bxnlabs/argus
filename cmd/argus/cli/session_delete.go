package cli

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var force bool
	var deleteBranch bool

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

			params := url.Values{}
			if force {
				params.Set("force", "true")
			}
			if deleteBranch {
				params.Set("delete_branch", "true")
			}

			endpoint := "/sessions/" + session.ID
			if len(params) > 0 {
				endpoint += "?" + params.Encode()
			}

			body, err := c.delete(endpoint)
			if err != nil {
				return err
			}

			fmt.Printf("Deleted session %q\n", session.Name)

			if deleteBranch && session.WorktreeBranch != nil {
				var resp struct {
					BranchDeleted bool `json:"branch_deleted"`
				}
				if err := json.Unmarshal(body, &resp); err == nil {
					if resp.BranchDeleted {
						fmt.Printf("Deleted branch %q\n", *session.WorktreeBranch)
					} else {
						fmt.Printf("Branch %q was not deleted (not eligible or could not be removed)\n", *session.WorktreeBranch)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force delete even if worktree has uncommitted changes")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Also delete the git branch")

	return cmd
}
