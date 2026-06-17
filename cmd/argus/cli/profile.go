package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// profileInfo mirrors the node API's ProfileInfo payload.
type profileInfo struct {
	Name       string `json:"name"`
	Dockerized bool   `json:"dockerized"`
}

// NewProfileCmd returns the top-level "profile" command group for managing
// dockerized-profile compose stacks.
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage dockerized-profile stacks",
		// Override root's PersistentPreRunE — profile commands use only the
		// discovery file and don't need config loading, mirroring the session
		// command group.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.AddCommand(newProfileLsCmd(), newProfileUpCmd(), newProfileDownCmd())
	return cmd
}

// profileClient builds an API client from the discovery file.
func profileClient() (*apiClient, error) {
	path, err := discoveryFilePath()
	if err != nil {
		return nil, err
	}
	return newClient(path)
}

func fetchProfiles(c *apiClient) ([]profileInfo, error) {
	body, err := c.get("/profiles")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Profiles []profileInfo `json:"profiles"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return resp.Profiles, nil
}

func newProfileLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List profiles and their type",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			profiles, err := fetchProfiles(c)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				fmt.Println("No profiles.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE")
			for _, p := range profiles {
				typ := "host"
				if p.Dockerized {
					typ = "docker"
				}
				fmt.Fprintf(w, "%s\t%s\n", p.Name, typ)
			}
			w.Flush()
			return nil
		},
	}
}

func newProfileUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up <profile>",
		Short: "Bring a dockerized profile's stack up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			if _, err := c.post("/profiles/"+args[0]+"/up", nil); err != nil {
				return err
			}
			fmt.Printf("Profile %q stack is up\n", args[0])
			return nil
		},
	}
}

func newProfileDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down <profile>",
		Short: "Tear a dockerized profile's stack down",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			if _, err := c.post("/profiles/"+args[0]+"/down", nil); err != nil {
				return err
			}
			fmt.Printf("Profile %q stack is down\n", args[0])
			return nil
		},
	}
}
