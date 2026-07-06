package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/bxnlabs/argus/internal/shared"
	"github.com/spf13/cobra"
)

// capturePaneArgs builds the tmux capture-pane argument list for a session.
// When all is set, the full scrollback history is captured (-S -); otherwise
// only the currently visible pane.
func capturePaneArgs(tmuxName string, all bool) []string {
	args := []string{"capture-pane", "-p"}
	if all {
		args = append(args, "-S", "-")
	}
	return append(args, "-t", tmuxName)
}

// sliceLines returns the first head or last tail lines of text. head and tail
// are mutually exclusive; passing both (>0) is an error. Zero for both returns
// text unchanged. A single trailing newline is preserved on the result.
func sliceLines(text string, head, tail int) (string, error) {
	if head > 0 && tail > 0 {
		return "", fmt.Errorf("cannot use --head and --tail together")
	}
	if head <= 0 && tail <= 0 {
		return text, nil
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	switch {
	case head > 0 && head < len(lines):
		lines = lines[:head]
	case tail > 0 && tail < len(lines):
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func newPeekCmd() *cobra.Command {
	var (
		all    bool
		head   int
		tail   int
		output string
	)
	cmd := &cobra.Command{
		Use:   "peek <name-or-id>",
		Short: "Print a session's tmux contents to stdout or a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if head > 0 && tail > 0 {
				return fmt.Errorf("cannot use --head and --tail together")
			}

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

			// Revive the tmux session if it died, mirroring `session attach`:
			// GET /sessions/{id} runs EnsureSession on the node.
			if _, err := c.get("/sessions/" + s.ID); err != nil {
				return fmt.Errorf("ensure session: %w", err)
			}

			tmuxCmd, err := shared.TmuxCommand(capturePaneArgs(s.TmuxName, all)...)
			if err != nil {
				return fmt.Errorf("build tmux command: %w", err)
			}
			out, err := tmuxCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("capture pane: %s", strings.TrimSpace(string(out)))
			}

			sliced, err := sliceLines(string(out), head, tail)
			if err != nil {
				return err
			}

			if output != "" {
				if err := os.WriteFile(output, []byte(sliced), 0o644); err != nil {
					return fmt.Errorf("write output file: %w", err)
				}
				return nil
			}
			fmt.Print(sliced)
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Capture the full scrollback history instead of the visible pane")
	cmd.Flags().IntVar(&head, "head", 0, "Print only the first N lines")
	cmd.Flags().IntVar(&tail, "tail", 0, "Print only the last N lines")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Write to this file instead of stdout")
	return cmd
}
