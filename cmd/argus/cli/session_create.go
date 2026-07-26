package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/bxnlabs/argus/internal/source"
)

// renderNewOutput writes the machine-facing result of a non-attach create to
// stdout: the pretty-printed record when asJSON, otherwise the bare session ID.
func renderNewOutput(stdout io.Writer, raw json.RawMessage, info sessionInfo, asJSON bool) error {
	if asJSON {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, raw, "", "  "); err != nil {
			return fmt.Errorf("format json: %w", err)
		}
		fmt.Fprintln(stdout, pretty.String())
		return nil
	}
	fmt.Fprintln(stdout, info.ID)
	return nil
}

func newCreateCmd() *cobra.Command {
	var (
		provider string
		src      string
		yolo     bool
		profile  string
		branch   string
		attach   bool
		asJSON   bool
	)

	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new session (headless by default; use --attach for interactive)",
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

			body, err := c.post("/sessions", bytes.NewReader(data), "create")
			if err != nil {
				return err
			}

			var resp struct {
				Session json.RawMessage `json:"session"`
			}
			if err := json.Unmarshal(body, &resp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			var s sessionInfo
			if err := json.Unmarshal(resp.Session, &s); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}

			fmt.Fprintf(os.Stderr, "Created session %q (%s)\n", s.Name, s.ProviderType)

			if attach {
				return attachTmux(s.ID, s.TmuxName, c.baseURL)
			}
			return renderNewOutput(os.Stdout, resp.Session, s, asJSON)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "claude", "Provider type (claude, codex, gemini, shell)")
	cmd.Flags().StringVar(&src, "src", "", "Source: local path or git URL/shorthand (defaults to current directory)")
	cmd.Flags().BoolVar(&yolo, "yolo", true, "Auto-approve tool calls (use --yolo=false to disable)")
	cmd.Flags().StringVar(&profile, "profile", "", "Profile name for lifecycle hooks")
	cmd.Flags().StringVar(&branch, "branch", "", "Branch name override (uses exact name, bypasses prefix/slug)")
	cmd.Flags().BoolVar(&attach, "attach", false, "Attach to the session's tmux after creating (interactive; default is headless)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the full session record as JSON instead of the bare ID")
	// --attach short-circuits to an interactive tmux attach and never prints the
	// record, so --json would be silently ignored; make the conflict explicit.
	cmd.MarkFlagsMutuallyExclusive("attach", "json")

	return cmd
}
