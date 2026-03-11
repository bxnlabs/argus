package worktree

import (
	"fmt"
	"strings"

	"github.com/bxnlabs/argus/internal/git"
)

// worktreeEntry represents a parsed entry from git worktree list --porcelain.
type worktreeEntry struct {
	path   string // worktree path (may contain spaces)
	branch string // short branch name (e.g. "main"), empty for detached HEAD
}

// listWorktrees returns parsed worktree entries, excluding the first entry
// (main working tree). Uses --porcelain for reliable parsing of paths with
// spaces.
func listWorktrees(repoDir string) ([]worktreeEntry, error) {
	out, err := git.Output(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var entries []worktreeEntry
	var current worktreeEntry
	first := true

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			if current.path != "" {
				if first {
					first = false
				} else {
					entries = append(entries, current)
				}
			}
			current = worktreeEntry{}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.path = line[len("worktree "):]
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			current.branch = line[len("branch refs/heads/"):]
		}
	}
	// Flush last entry (porcelain output may not end with blank line).
	if current.path != "" && !first {
		entries = append(entries, current)
	}
	return entries, nil
}

// mainWorktreeBranch returns the branch checked out in the main working tree.
// Returns empty string if the main worktree has a detached HEAD.
func mainWorktreeBranch(repoDir string) (string, error) {
	out, err := git.Output(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}

	// The first entry in porcelain output is always the main working tree.
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			break // end of first entry
		}
		if strings.HasPrefix(line, "branch refs/heads/") {
			return line[len("branch refs/heads/"):], nil
		}
	}
	return "", nil
}
