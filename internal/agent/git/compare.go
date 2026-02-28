package git

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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

// GetCompare returns the full diff and per-file metadata comparing base to HEAD.
func GetCompare(dir, base string) (*CompareResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	// Find the merge base
	mergeBase, err := runGit(ctx, dir, defaultMaxBuffer, "merge-base", base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to find merge base: %w", err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	// Get HEAD ref
	headRef, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--short", "HEAD")
	if err != nil {
		return nil, err
	}
	headRef = strings.TrimSpace(headRef)

	// Full combined diff
	diff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U20", mergeBase+"..HEAD")
	if err != nil {
		return nil, err
	}

	// Per-file metadata: name-status
	statusMap := map[string]struct {
		status  string
		oldPath string
	}{}
	if nsOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--name-status", mergeBase+"..HEAD"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(nsOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			statusChar := fields[0]
			if len(fields) == 3 {
				statusMap[fields[2]] = struct {
					status  string
					oldPath string
				}{statusChar[:1], fields[1]}
			} else {
				statusMap[fields[1]] = struct {
					status  string
					oldPath string
				}{statusChar[:1], ""}
			}
		}
	}

	// Per-file metadata: numstat
	var files []CommitFile
	totalAdds, totalDels := 0, 0
	if numOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--numstat", mergeBase+"..HEAD"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(numOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) != 3 {
				continue
			}

			adds, dels := 0, 0
			isBinary := fields[0] == "-" && fields[1] == "-"
			if !isBinary {
				adds, _ = strconv.Atoi(fields[0])
				dels, _ = strconv.Atoi(fields[1])
			}

			path := fields[2]
			if idx := strings.Index(path, " => "); idx != -1 {
				path = path[idx+4:]
			}

			st := StatusModified
			var oldPath string
			if info, ok := statusMap[path]; ok {
				switch info.status {
				case "A":
					st = StatusAdded
				case "D":
					st = StatusDeleted
				case "R":
					st = StatusRenamed
					oldPath = info.oldPath
				case "C":
					st = StatusCopied
				}
			}

			files = append(files, CommitFile{
				Path:      path,
				Status:    st,
				Additions: adds,
				Deletions: dels,
				OldPath:   oldPath,
			})

			totalAdds += adds
			totalDels += dels
		}
	}

	if files == nil {
		files = []CommitFile{}
	}

	return &CompareResult{
		Diff:           diff,
		Files:          files,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
		BaseRef:        mergeBase[:7],
		HeadRef:        headRef,
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
