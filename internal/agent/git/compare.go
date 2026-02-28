package git

import (
	"context"
	"sort"
	"strings"
)

// GetBranches returns all local branches and the auto-detected default base branch.
func GetBranches(dir string) (*BranchList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	out, err := runGit(ctx, dir, defaultMaxBuffer,
		"branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	sort.Strings(branches)

	defaultBase := detectDefaultBase(ctx, dir, branches)

	return &BranchList{
		Branches:    branches,
		DefaultBase: defaultBase,
	}, nil
}

// detectDefaultBase tries upstream tracking branch, then falls back to main/master.
func detectDefaultBase(ctx context.Context, dir string, branches []string) string {
	// Try upstream tracking branch
	if upstream, err := runGit(ctx, dir, defaultMaxBuffer,
		"rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		upstream = strings.TrimSpace(upstream)
		if upstream != "" {
			return upstream
		}
	}

	// Fall back to main, then master
	branchSet := make(map[string]bool, len(branches))
	for _, b := range branches {
		branchSet[b] = true
	}
	if branchSet["main"] {
		return "main"
	}
	if branchSet["master"] {
		return "master"
	}
	return ""
}
