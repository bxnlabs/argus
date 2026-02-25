package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
	var (
		provider string
		src      string
		yolo     bool
	)

	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new session and attach",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			name := args[0]

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			reqBody := map[string]any{
				"name":         name,
				"agent_type":   provider,
				"auto_approve": yolo,
			}
			if src != "" {
				reqBody["source"] = src
			}

			data, err := json.Marshal(reqBody)
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}

			body, err := c.post("/api/sessions", bytes.NewReader(data))
			if err != nil {
				return err
			}

			var resp struct {
				Session sessionInfo `json:"session"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			s := resp.Session
			fmt.Fprintf(os.Stderr, "Created session %q (%s)\n", s.Name, s.AgentType)

			return attachTmux(s.TmuxName)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent type (claude, codex, gemini, shell)")
	cmd.Flags().StringVar(&src, "src", "", "Source: local path or git URL/shorthand (defaults to current directory)")
	cmd.Flags().BoolVar(&yolo, "yolo", false, "Enable auto-approve")

	return cmd
}
