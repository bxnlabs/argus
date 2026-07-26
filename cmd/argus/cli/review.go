package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bxnlabs/argus/internal/node/git/review"
)

// resolveReviewBase returns the base branch to use when loading a review.
// If flagBase is non-empty after trimming whitespace (the user passed --base),
// the trimmed value is returned and no network round-trip is made. Otherwise,
// the helper queries /api/node/git/compare/branches and returns the repo's detected
// default base (typically main or master). If neither source yields a base,
// an error is returned that explicitly guides the user toward --base.
func resolveReviewBase(c *apiClient, pathParam, flagBase string) (string, error) {
	if base := strings.TrimSpace(flagBase); base != "" {
		return base, nil
	}
	body, err := c.get("/git/compare/branches?" + pathParam)
	if err != nil {
		return "", fmt.Errorf("get branches: %w", err)
	}
	var resp struct {
		DefaultBase string `json:"defaultBase"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse branches: %w", err)
	}
	if resp.DefaultBase == "" {
		return "", fmt.Errorf("no default base branch detected; use --base to specify one")
	}
	return resp.DefaultBase, nil
}

// commentGroupKey uniquely identifies a file section in the review output.
// Using File+OldPath avoids collisions when a renamed file's old path matches
// another real file in the same diff.
type commentGroupKey struct {
	file    string
	oldPath string
}

// commentGroup holds the display metadata and comments for a file section.
type commentGroup struct {
	displayFile string
	comments    []review.ReviewComment
}

// commentGroupKeyFor returns the grouping key for a comment.
func commentGroupKeyFor(c review.ReviewComment) commentGroupKey {
	return commentGroupKey{file: c.File, oldPath: c.OldPath}
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
		fmt.Fprintf(&b, "No submitted comments for %s vs %s.\n", r.Head, r.Base)
		return b.String()
	}

	b.WriteString("## Comments\n")
	fmt.Fprintf(&b, "Branch: %s vs %s\n", r.Head, r.Base)

	if hasBody {
		b.WriteString("\n" + r.Body.Body + "\n")
	}

	groups := make(map[commentGroupKey]*commentGroup)
	var keyOrder []commentGroupKey
	for _, c := range submitted {
		key := commentGroupKeyFor(c)
		g, seen := groups[key]
		if !seen {
			g = &commentGroup{displayFile: c.File}
			if c.OldPath != "" {
				g.displayFile = c.OldPath + " \u2192 " + c.File
			}
			groups[key] = g
			keyOrder = append(keyOrder, key)
		}
		g.comments = append(g.comments, c)
	}
	sort.Slice(keyOrder, func(i, j int) bool {
		di, dj := groups[keyOrder[i]].displayFile, groups[keyOrder[j]].displayFile
		if di != dj {
			return di < dj
		}
		return keyOrder[i].oldPath < keyOrder[j].oldPath
	})

	for _, key := range keyOrder {
		g := groups[key]
		fmt.Fprintf(&b, "\n### %s\n\n", g.displayFile)
		for _, c := range g.comments {
			fmt.Fprintf(&b, "**Lines %d-%d:**\n", c.Line.From.Line, c.Line.To.Line)
			for _, line := range strings.Split(c.Snippet, "\n") {
				b.WriteString("> " + line + "\n")
			}
			b.WriteString("\n" + c.Body + "\n")
		}
	}

	return b.String()
}
