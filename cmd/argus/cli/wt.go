package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/bxnlabs/argus/internal/git"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/spf13/cobra"
)

// wtItem is one managed worktree as returned by the node worktree routes.
type wtItem struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// resolveRepoRoot returns the git worktree root of the current directory.
// Worktree operations are keyed by repository root — matching the comments
// command and how node sessions key repos — so running from a subdirectory
// still addresses the right repo.
func resolveRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	root, err := git.Output(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}
	return strings.TrimSpace(root), nil
}

// worktreesTable renders managed worktrees as a BRANCH/PATH table, compressing
// each path against home like `session ls`.
func worktreesTable(items []wtItem, home string) string {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BRANCH\tPATH")
	for _, it := range items {
		fmt.Fprintf(w, "%s\t%s\n", it.Branch, shared.CompressPath(it.Path, home, 60))
	}
	w.Flush()
	return b.String()
}

// runWtCo posts a create-or-reuse request and prints the worktree path to
// stdout (for `cd "$(...)"`) with a human note to stderr.
func runWtCo(c *apiClient, repoDir, branch string, stdout, stderr io.Writer) error {
	params := url.Values{"path": {repoDir}, "branch": {branch}}
	body, err := c.post("/git/worktree?"+params.Encode(), nil, "create worktree")
	if err != nil {
		return err
	}
	var resp wtItem
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	fmt.Fprintf(stderr, "Worktree for %q ready\n", resp.Branch)
	fmt.Fprintln(stdout, resp.Path)
	return nil
}

// runWtLs fetches and prints the managed worktrees table.
func runWtLs(c *apiClient, repoDir, home string, stdout io.Writer) error {
	params := url.Values{"path": {repoDir}}
	body, err := c.get("/git/worktrees?" + params.Encode())
	if err != nil {
		return err
	}
	var resp struct {
		Worktrees []wtItem `json:"worktrees"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	fmt.Fprint(stdout, worktreesTable(resp.Worktrees, home))
	return nil
}

// runWtRm deletes the worktree for branch and prints a confirmation.
func runWtRm(c *apiClient, repoDir, branch string, stdout io.Writer) error {
	params := url.Values{"path": {repoDir}, "branch": {branch}}
	if _, err := c.delete("/git/worktree?" + params.Encode()); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed worktree for %q\n", branch)
	return nil
}

// clientForRepo resolves the repo root, discovery file, and node client shared
// by all wt subcommands.
func clientForRepo() (*apiClient, string, error) {
	repoDir, err := resolveRepoRoot()
	if err != nil {
		return nil, "", err
	}
	dp, err := discoveryFilePath()
	if err != nil {
		return nil, "", err
	}
	c, err := newClient(dp)
	if err != nil {
		return nil, "", err
	}
	return c, repoDir, nil
}

// newWtCmd returns the "wt" group under `argus git`.
func newWtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wt",
		Short: "Manage git worktrees",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(
		newWtCoCmd(),
		newWtLsCmd(),
		newWtRmCmd(),
	)
	return cmd
}

func newWtCoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "co <branch>",
		Short: "Create or reuse a worktree for a branch and print its path",
		Long: "Create or reuse a git worktree for <branch> and print its path to stdout.\n\n" +
			"Because a binary cannot change the caller's shell, use it with cd:\n\n" +
			"    cd \"$(argus git wt co my-branch)\"",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, repoDir, err := clientForRepo()
			if err != nil {
				return err
			}
			return runWtCo(c, repoDir, args[0], os.Stdout, os.Stderr)
		},
	}
	return cmd
}

func newWtLsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List managed worktrees for the current repo",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, repoDir, err := clientForRepo()
			if err != nil {
				return err
			}
			home, _ := os.UserHomeDir()
			return runWtLs(c, repoDir, home, os.Stdout)
		},
	}
	return cmd
}

func newWtRmCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rm <branch>",
		Short: "Remove the worktree for a branch (the branch is preserved)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, repoDir, err := clientForRepo()
			if err != nil {
				return err
			}
			return runWtRm(c, repoDir, args[0], os.Stdout)
		},
	}
	return cmd
}
