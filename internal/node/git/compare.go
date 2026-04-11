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

// GetAllBranches returns local + remote branches for a repo directory.
// Remote tracking branches are stripped of the "origin/" prefix and
// deduplicated against local branches. Symbolic refs like origin/HEAD
// are filtered out.
func GetAllBranches(dir string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	// Local branches
	localOut, err := runGit(ctx, dir, defaultMaxBuffer,
		"branch", "--format=%(refname:short)")
	if err != nil {
		return nil, err
	}

	localSet := make(map[string]bool)
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(localOut), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			localSet[line] = true
			branches = append(branches, line)
		}
	}

	// Remote tracking branches (origin only)
	remoteOut, err := runGit(ctx, dir, defaultMaxBuffer,
		"branch", "-r", "--format=%(refname:short)")
	if err != nil {
		// No remotes is fine — just return local branches
		sort.Strings(branches)
		return branches, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(remoteOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "origin/") {
			continue
		}
		if line == "origin/HEAD" {
			continue
		}
		short := strings.TrimPrefix(line, "origin/")
		if !localSet[short] {
			branches = append(branches, short)
		}
	}

	sort.Strings(branches)
	return branches, nil
}

// ValidateBranchName checks that the given string is a valid git branch name.
// It trims whitespace and rejects leading dashes.
// Exported wrapper around validateBranchRef for use at session creation time.
func ValidateBranchName(dir, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()
	return validateBranchRef(ctx, dir, name)
}

// validateBranchRef checks that the given ref is a valid git branch name
// and does not start with a dash (to prevent git option injection).
func validateBranchRef(ctx context.Context, dir, ref string) error {
	if strings.HasPrefix(ref, "-") {
		return fmt.Errorf("%w: invalid branch name: %q", ErrInvalidInput, ref)
	}
	if _, err := runGit(ctx, dir, defaultMaxBuffer, "check-ref-format", "--branch", ref); err != nil {
		return fmt.Errorf("%w: invalid branch name: %q", ErrInvalidInput, ref)
	}
	return nil
}

// GetCompare returns the full diff and per-file metadata comparing base to HEAD.
func GetCompare(dir, base string) (*CompareResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	if err := validateBranchRef(ctx, dir, base); err != nil {
		return nil, err
	}

	// Find the merge base
	mergeBase, err := runGit(ctx, dir, defaultMaxBuffer, "merge-base", base, "HEAD")
	if err != nil {
		// merge-base exits 1 when no common ancestor exists (disconnected
		// histories). Together with isNotFoundError (bad ref), these are
		// user-facing not-found conditions. Other failures (timeout,
		// corruption) should surface as internal errors.
		if isNotFoundError(err) || strings.HasSuffix(err.Error(), "exit status 1") {
			return nil, fmt.Errorf("%w: no merge base found for %q", ErrNotFound, base)
		}
		return nil, fmt.Errorf("failed to find merge base for %q: %w", base, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	// Get HEAD ref
	headRef, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	headRef = strings.TrimSpace(headRef)

	// Full combined diff
	diff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U3", mergeBase+"..HEAD")
	if err != nil {
		return nil, err
	}

	// Per-file metadata: name-status
	statusMap := map[string]struct {
		status  string
		oldPath string
	}{}
	nsOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--name-status", mergeBase+"..HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get file statuses: %w", err)
	}
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

	// Per-file metadata: numstat
	var files []CommitFile
	totalAdds, totalDels := 0, 0
	numOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--numstat", mergeBase+"..HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
	}
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

		path := normalizeNumstatPath(fields[2])

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

	if files == nil {
		files = []CommitFile{}
	}

	// Compute per-file total line counts from postimage at HEAD
	fileTotalLines := computeTotalLines(ctx, dir, headRef, files)

	return &CompareResult{
		Diff:           diff,
		Files:          files,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
		BaseRef:        mergeBase,
		HeadRef:        headRef,
		TotalLines:     fileTotalLines,
	}, nil
}

// normalizeNumstatPath extracts the new file path from a numstat rename entry.
// Handles both simple renames ("old => new") and brace-form ("dir/{old => new}/file").
func normalizeNumstatPath(p string) string {
	if !strings.Contains(p, " => ") {
		return p
	}
	// Brace-form: dir/{old.go => new.go} or {old => new}/file
	if pre, rest, ok := strings.Cut(p, "{"); ok {
		if mid, post, ok2 := strings.Cut(rest, "}"); ok2 {
			if _, newPart, ok3 := strings.Cut(mid, " => "); ok3 {
				return pre + strings.TrimSpace(newPart) + post
			}
		}
	}
	// Simple form: "old => new"
	if _, newPart, ok := strings.Cut(p, " => "); ok {
		return strings.TrimSpace(newPart)
	}
	return p
}

func truncateRef(ref string) string {
	if len(ref) > 7 {
		return ref[:7]
	}
	return ref
}

// detectDefaultBase returns main or master as the default comparison base.
func detectDefaultBase(_ context.Context, _ string, branches []string) string {
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
