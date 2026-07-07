package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/node/git/review"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/spf13/cobra"
)

// newCommentsCmd returns the "comments" group under `argus git`.
func newCommentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Read and write review comments for the current branch",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(
		newCommentsViewCmd(),
	)
	return cmd
}

// addBaseFlag registers the shared --base flag with its standard help text.
func addBaseFlag(cmd *cobra.Command, base *string) {
	cmd.Flags().StringVar(base, "base", "", "Base branch to compare against (default: detected default branch, typically main or master)")
}

// reviewContext bundles everything the comments subcommands need to locate and
// address a review document: the node API client, the repo's local path, the
// per-project state dir (for local review.Load), and the resolved head/base
// branch pair.
type reviewContext struct {
	client     *apiClient
	repoDir    string
	projectDir string
	branch     string
	base       string
}

// resolveReviewContext resolves the current repo, its head branch (via
// GET /git/status), and the base branch (via --base or GET /git/compare/branches)
// and returns a reviewContext. It mirrors the resolution the old
// `tools git review get` performed inline.
func resolveReviewContext(baseFlag string) (*reviewContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}
	resolved, err := source.Resolve(cwd)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve project: %w", err)
	}
	repoDir := resolved.LocalPath
	stateDir, err := shared.StateDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine state dir: %w", err)
	}
	projectDir := filepath.Join(stateDir, "projects", resolved.ParentKey())

	dp, err := discoveryFilePath()
	if err != nil {
		return nil, err
	}
	c, err := newClient(dp)
	if err != nil {
		return nil, err
	}

	pathParam := url.Values{"path": []string{repoDir}}.Encode()
	body, err := c.get("/git/status?" + pathParam)
	if err != nil {
		return nil, fmt.Errorf("get git status: %w", err)
	}
	var statusResp struct {
		Status struct {
			Branch string `json:"branch"`
		} `json:"status"`
	}
	if err := json.Unmarshal(body, &statusResp); err != nil {
		return nil, fmt.Errorf("parse status: %w", err)
	}

	base, err := resolveReviewBase(c, pathParam, baseFlag)
	if err != nil {
		return nil, err
	}

	return &reviewContext{
		client:     c,
		repoDir:    repoDir,
		projectDir: projectDir,
		branch:     statusResp.Status.Branch,
		base:       base,
	}, nil
}

// loadLocalReview reads the review document from local disk for the resolved
// branch pair, returning an empty (non-nil) Review when no file exists.
func loadLocalReview(rc *reviewContext) (*review.Review, error) {
	rv, err := review.Load(rc.projectDir, rc.repoDir, rc.branch, rc.base, "", "")
	if err != nil {
		return nil, fmt.Errorf("load review: %w", err)
	}
	if rv == nil {
		rv = &review.Review{Head: rc.branch, Base: rc.base}
	}
	return rv, nil
}

func newCommentsViewCmd() *cobra.Command {
	var baseFlag string
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Print submitted review comments for the current branch as markdown",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			rc, err := resolveReviewContext(baseFlag)
			if err != nil {
				return err
			}
			rv, err := loadLocalReview(rc)
			if err != nil {
				return err
			}
			fmt.Print(formatReviewMarkdown(rv))
			return nil
		},
	}
	addBaseFlag(cmd, &baseFlag)
	return cmd
}
