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

	defaultBase := detectDefaultBase(branches)

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
		// No remotes is fine — just return local branches. A blown deadline is
		// not: everything after it fails too, so a short list would look like
		// a repo without remotes.
		if isOperationalError(err) {
			return nil, err
		}
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
		// A deadline says nothing about the name's validity, and 400 would
		// blame the user for a failure that was ours.
		if isOperationalError(err) {
			return err
		}
		return fmt.Errorf("%w: invalid branch name: %q", ErrInvalidInput, ref)
	}
	return nil
}

// resolveComparisonBase returns the ref that should be used as the effective
// comparison base, along with staleness metadata. If `base` has an upstream
// tracking branch and the local ref is strictly behind that upstream (behind
// but not also ahead), the upstream ref is returned so the compare behaves
// like GitHub (diff against the freshest known tip of the base). Diverged
// bases keep the local ref so local-only commits remain on the base side of
// the diff. Otherwise the original `base` is returned unchanged.
//
// The returned upstreamName is the abbreviated upstream ref (e.g. "origin/main")
// when substitution occurred, and empty otherwise. behindBy is the number of
// commits local base is behind upstream (0 when no substitution happened).
func resolveComparisonBase(ctx context.Context, dir, base string) (string, string, int) {
	upstream, err := runGit(ctx, dir, defaultMaxBuffer,
		"rev-parse", "--abbrev-ref", base+"@{upstream}")
	if err != nil {
		// No upstream configured, or upstream ref missing locally. Fall back
		// to local base — behavior matches the pre-upstream-resolution code.
		return base, "", 0
	}
	upstream = strings.TrimSpace(upstream)
	if upstream == "" || upstream == base {
		return base, "", 0
	}

	// Use a symmetric count so we can distinguish "behind only" from "diverged".
	// A diverged base has local-only commits that belong on the base side of the
	// diff; substituting upstream would misattribute them to the feature branch.
	countOut, err := runGit(ctx, dir, defaultMaxBuffer,
		"rev-list", "--left-right", "--count", base+"..."+upstream)
	if err != nil {
		return base, "", 0
	}
	parts := strings.Fields(strings.TrimSpace(countOut))
	if len(parts) != 2 {
		return base, "", 0
	}
	ahead, aheadErr := strconv.Atoi(parts[0])
	behind, behindErr := strconv.Atoi(parts[1])
	if aheadErr != nil || behindErr != nil || ahead > 0 || behind <= 0 {
		return base, "", 0
	}
	return upstream, upstream, behind
}

// GetCompare returns the full diff and per-file metadata comparing base to HEAD.
func GetCompare(dir, base string) (*CompareResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	if err := validateBranchRef(ctx, dir, base); err != nil {
		return nil, err
	}

	// Resolve base to its upstream tip when local is stale. Matches GitHub PR
	// compare semantics so users do not have to keep worktree refs in sync.
	effectiveBase, upstreamName, behindBy := resolveComparisonBase(ctx, dir, base)

	// Pin HEAD to a commit id up front, and address it by that id everywhere
	// below. Every step here is a separate git invocation, so a commit landing
	// mid-flight — routine on a node running coding agents — would otherwise let
	// the steps disagree: a result carrying one HeadRef but a diff, a file list
	// and line counts taken from another. Callers use (headRef, baseRef) as the
	// identity of a comparison, so that combination is not merely inconsistent,
	// it is a stale identity attached to fresh content.
	headRef, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "HEAD")
	if err != nil {
		// An unborn HEAD — a fresh `checkout --orphan`, or a clone of an empty
		// remote — is a user-facing not-found, not a server fault. It has to be
		// classified here now that this runs first: merge-base used to be the
		// command that met it, and mapped it below.
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: HEAD has no commit", ErrNotFound)
		}
		return nil, fmt.Errorf("failed to resolve HEAD: %w", err)
	}
	headRef = strings.TrimSpace(headRef)

	// Find the merge base
	mergeBase, err := runGit(ctx, dir, defaultMaxBuffer, "merge-base", effectiveBase, headRef)
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

	// The one range every step below is computed from.
	compareRange := mergeBase + ".." + headRef

	// Full combined diff
	diff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U3", compareRange)
	if err != nil {
		return nil, err
	}

	// Per-file metadata: name-status
	statusMap := map[string]struct {
		status  string
		oldPath string
	}{}
	nsOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--name-status", compareRange)
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
	numOut, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--numstat", compareRange)
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
		BaseUpstream:   upstreamName,
		BaseBehindBy:   behindBy,
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
func detectDefaultBase(branches []string) string {
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
