package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/bxnlabs/argus/internal/source"
)

func newCreateCmd() *cobra.Command {
	var (
		provider string
		src      string
		yolo     bool
		profile  string
		branch   string
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

			// Default to current directory when no source is specified.
			if src == "" {
				src = "."
			}

			// Resolve local paths to absolute before sending to the server
			// daemon, whose CWD may differ from the caller's shell.
			resolved, err := source.Resolve(src)
			if err == nil && !resolved.IsRemote() {
				src = resolved.LocalPath
			}

			reqBody := map[string]any{
				"name":          name,
				"provider_type": provider,
				"source":       src,
				"auto_approve": yolo,
			}
			if profile != "" {
				reqBody["profile"] = profile
			}
			if branch != "" {
				reqBody["branch"] = branch
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
			fmt.Fprintf(os.Stderr, "Created session %q (%s)\n", s.Name, s.ProviderType)

			return attachTmux(s.TmuxName)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "claude", "Provider type (claude, codex, gemini, shell)")
	cmd.Flags().StringVar(&src, "src", "", "Source: local path or git URL/shorthand (defaults to current directory)")
	cmd.Flags().BoolVar(&yolo, "yolo", true, "Auto-approve tool calls (use --yolo=false to disable)")
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name for lifecycle hooks")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name override (uses exact name, bypasses prefix/slug)")

	return cmd
}
