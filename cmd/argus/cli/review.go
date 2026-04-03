package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bxnlabs/argus/internal/node/git/review"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Manage code reviews",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(newReviewGetCmd())
	return cmd
}

func newReviewGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get submitted inline comments for the current branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("cannot determine working directory: %w", err)
			}
			resolved, err := source.Resolve(cwd)
			if err != nil {
				return fmt.Errorf("cannot resolve project: %w", err)
			}
			repoDir := resolved.LocalPath
			parentKey := resolved.ParentKey()
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}
			projectDir := filepath.Join(home, ".argus", "projects", parentKey)

			dp, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(dp)
			if err != nil {
				return err
			}

			pathParam := url.Values{"path": []string{repoDir}}.Encode()
			body, err := c.get("/api/git/status?" + pathParam)
			if err != nil {
				return fmt.Errorf("get git status: %w", err)
			}
			var statusResp struct {
				Status struct {
					Branch string `json:"branch"`
				} `json:"status"`
			}
			if err := json.Unmarshal(body, &statusResp); err != nil {
				return fmt.Errorf("parse status: %w", err)
			}
			branch := statusResp.Status.Branch

			body, err = c.get("/api/git/compare/branches?" + pathParam)
			if err != nil {
				return fmt.Errorf("get branches: %w", err)
			}
			var branchResp struct {
				DefaultBase string `json:"defaultBase"`
			}
			if err := json.Unmarshal(body, &branchResp); err != nil {
				return fmt.Errorf("parse branches: %w", err)
			}
			baseBranch := branchResp.DefaultBase

			rv, err := review.Load(projectDir, repoDir, branch, baseBranch, "", "")
			if err != nil {
				return fmt.Errorf("load review: %w", err)
			}
			if rv == nil {
				rv = &review.Review{Head: branch, Base: baseBranch}
			}

			fmt.Print(formatReviewMarkdown(rv))
			return nil
		},
	}
}

// formatReviewMarkdown formats submitted review comments as structured markdown.
func formatReviewMarkdown(r *review.Review) string {
	var b strings.Builder

	var submitted []review.ReviewComment
	for _, c := range r.Comments {
		if c.Submitted {
			submitted = append(submitted, c)
		}
	}

	hasBody := r.Body != nil && r.Body.Submitted && r.Body.Body != ""

	if len(submitted) == 0 && !hasBody {
		b.WriteString("No submitted review comments.\n")
		return b.String()
	}

	b.WriteString("## Review\n")
	fmt.Fprintf(&b, "Branch: %s vs %s\n", r.Head, r.Base)

	if hasBody {
		b.WriteString("\n" + r.Body.Body + "\n")
	}

	byFile := make(map[string][]review.ReviewComment)
	var fileOrder []string
	for _, c := range submitted {
		if _, seen := byFile[c.File]; !seen {
			fileOrder = append(fileOrder, c.File)
		}
		byFile[c.File] = append(byFile[c.File], c)
	}
	sort.Strings(fileOrder)

	for _, file := range fileOrder {
		fmt.Fprintf(&b, "\n### %s\n\n", file)
		for _, c := range byFile[file] {
			fmt.Fprintf(&b, "**Lines %d-%d:**\n", c.Line.From.Line, c.Line.To.Line)
			for _, line := range strings.Split(c.Snippet, "\n") {
				b.WriteString("> " + line + "\n")
			}
			b.WriteString("\n" + c.Body + "\n")
		}
	}

	return b.String()
}
