package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bxnlabs/argus/internal/node/comments"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/spf13/cobra"
)

// NewCommentsCmd returns the "comments" parent command.
func NewCommentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Manage inline review comments",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.AddCommand(newCommentsGetCmd())
	return cmd
}

func newCommentsGetCmd() *cobra.Command {
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

			body, err := c.get(fmt.Sprintf("/api/git/status?path=%s", repoDir))
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

			body, err = c.get(fmt.Sprintf("/api/git/compare/branches?path=%s", repoDir))
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

			cf, err := comments.Load(projectDir, repoDir, branch, baseBranch)
			if err != nil {
				return fmt.Errorf("load comments: %w", err)
			}
			if cf == nil {
				cf = &comments.CommentsFile{
					Branch:     branch,
					BaseBranch: baseBranch,
				}
			}

			fmt.Print(formatCommentsMarkdown(cf))
			return nil
		},
	}
}

// formatCommentsMarkdown formats submitted comments as structured markdown.
func formatCommentsMarkdown(cf *comments.CommentsFile) string {
	var b strings.Builder

	var submitted []comments.Comment
	for _, c := range cf.Comments {
		if c.Submitted {
			submitted = append(submitted, c)
		}
	}

	hasGeneral := cf.GeneralComment != nil && cf.GeneralComment.Submitted && cf.GeneralComment.Body != ""

	if len(submitted) == 0 && !hasGeneral {
		b.WriteString("No submitted comments.\n")
		return b.String()
	}

	b.WriteString("## Comments\n")
	fmt.Fprintf(&b, "Branch: %s vs %s\n", cf.Branch, cf.BaseBranch)

	byFile := make(map[string][]comments.Comment)
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
			fmt.Fprintf(&b, "**Lines %d-%d:**\n", c.Line.From, c.Line.To)
			for _, line := range strings.Split(c.Snippet, "\n") {
				b.WriteString("> " + line + "\n")
			}
			b.WriteString("\n" + c.Body + "\n")
		}
	}

	if hasGeneral {
		b.WriteString("\n### General\n\n")
		b.WriteString(cf.GeneralComment.Body + "\n")
	}

	return b.String()
}
