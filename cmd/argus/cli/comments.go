package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/bxnlabs/argus/internal/git"
	"github.com/bxnlabs/argus/internal/node/git/review"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/spf13/cobra"
)

// newCommentsCmd returns the "comments" group under `argus git`.
func newCommentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Read comments for the current branch",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(
		newCommentsViewCmd(),
		newCommentsLsCmd(),
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
// and returns a reviewContext.
//
// The repo is the worktree root (git rev-parse --show-toplevel), not the
// current directory: reviews are keyed by the repository root — matching how
// node sessions and the web UI key them — so running a comments command from a
// subdirectory addresses the same review document, and file paths are anchored
// relative to the root.
func resolveReviewContext(baseFlag string) (*reviewContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot determine working directory: %w", err)
	}
	root, err := git.Output(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository: %w", err)
	}
	repoDir := strings.TrimSpace(root)
	stateDir, err := shared.StateDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine state dir: %w", err)
	}
	projectDir := filepath.Join(stateDir, "projects", source.ParentKeyFromPath(repoDir))

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

// commentBodyMax bounds the BODY column of the comments table.
const commentBodyMax = 60

// firstLineTruncated returns the first line of s, truncated to max runes with a
// trailing ellipsis when longer. Used for the compact BODY column.
func firstLineTruncated(s string, max int) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// commentsTable renders comments as an aligned table with columns
// ID, FILE:LINE, SUBMITTED, BODY. An empty slice yields a single message line.
func commentsTable(comments []review.ReviewComment) string {
	if len(comments) == 0 {
		return "No comments.\n"
	}
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tFILE:LINE\tSUBMITTED\tBODY")
	for _, c := range comments {
		submitted := "no"
		if c.Submitted {
			submitted = "yes"
		}
		// Left-side comments anchor to a line in the base version of the file;
		// for a renamed file that line lives at OldPath, so show it there.
		file := c.File
		if c.Line.From.Side == review.DiffSideLeft && c.OldPath != "" {
			file = c.OldPath
		}
		loc := fmt.Sprintf("%s:%d", file, c.Line.From.Line)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.ID, loc, submitted, firstLineTruncated(c.Body, commentBodyMax))
	}
	w.Flush()
	return b.String()
}

func newCommentsLsCmd() *cobra.Command {
	var baseFlag string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List comments for the current branch in a compact table",
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
			fmt.Print(commentsTable(rv.Comments))
			return nil
		},
	}
	addBaseFlag(cmd, &baseFlag)
	return cmd
}

func newCommentsViewCmd() *cobra.Command {
	var baseFlag string
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Print submitted comments for the current branch as markdown",
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
