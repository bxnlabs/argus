package git

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// GetStatus returns the git status for a directory.
// Matches lib/git-status.ts:getGitStatus().
func GetStatus(dir string) (*GitStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	// Get current branch
	branch, err := runGit(ctx, dir, defaultMaxBuffer, "branch", "--show-current")
	if err != nil {
		return nil, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "HEAD" // detached HEAD
	}

	// Get ahead/behind counts (may fail if no upstream)
	ahead, behind := 0, 0
	if out, err := runGit(ctx, dir, defaultMaxBuffer, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		parts := strings.Fields(strings.TrimSpace(out))
		if len(parts) == 2 {
			behind, _ = strconv.Atoi(parts[0])
			ahead, _ = strconv.Atoi(parts[1])
		}
	}

	// Get porcelain status. -uall enumerates individual files inside untracked
	// directories rather than collapsing them to a single directory entry.
	out, err := runGit(ctx, dir, defaultMaxBuffer, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}

	staged := []GitFile{}
	unstaged := []GitFile{}
	untracked := []GitFile{}

	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		filePath := line[3:]

		// Handle renames: "old -> new"
		var oldPath string
		if idx := strings.Index(filePath, " -> "); idx != -1 {
			oldPath = filePath[:idx]
			filePath = filePath[idx+4:]
		}

		// Untracked
		if indexStatus == '?' && workTreeStatus == '?' {
			untracked = append(untracked, GitFile{
				Path:   filePath,
				Status: StatusUntracked,
				Staged: false,
			})
			continue
		}

		// Staged (index has changes)
		if indexStatus != ' ' && indexStatus != '?' {
			staged = append(staged, GitFile{
				Path:    filePath,
				OldPath: oldPath,
				Status:  parseStatus(indexStatus),
				Staged:  true,
			})
		}

		// Unstaged (worktree has changes)
		if workTreeStatus != ' ' && workTreeStatus != '?' {
			unstaged = append(unstaged, GitFile{
				Path:    filePath,
				OldPath: oldPath,
				Status:  parseStatus(workTreeStatus),
				Staged:  false,
			})
		}
	}

	return &GitStatus{
		Branch:    branch,
		Ahead:     ahead,
		Behind:    behind,
		Staged:    staged,
		Unstaged:  unstaged,
		Untracked: untracked,
	}, nil
}

// GetFileDiff returns the diff for a file.
// Matches lib/git-status.ts:getFileDiff() and getUntrackedFileDiff().
func GetFileDiff(dir, file string, staged, untracked bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	if untracked {
		// git diff --no-index exits 1 when files differ (normal behavior).
		// runGit discards stdout on non-zero exit, so we run this directly
		// to capture the diff output despite the expected exit code 1.
		return runGitDiffNoIndex(ctx, dir, file)
	}

	var args []string
	if staged {
		args = []string{"diff", "-U20", "--cached", "--", file}
	} else {
		args = []string{"diff", "-U20", "--", file}
	}

	return runGit(ctx, dir, diffMaxBuffer, args...)
}

// runGitDiffNoIndex runs git diff --no-index /dev/null <file> and returns
// stdout even when git exits 1 (which means files differ — normal behavior).
func runGitDiffNoIndex(ctx context.Context, dir, file string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "-U20", "--no-index", "/dev/null", file)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	stdout := &limitedWriter{limit: diffMaxBuffer}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Exit code 1 = files differ, which is expected for untracked files
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return stdout.buf.String(), nil
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git diff: %s", errMsg)
	}
	return stdout.buf.String(), nil
}

// GetFileContent returns the HEAD version of a file.
// Returns (content, isNew, error). isNew=true if file doesn't exist in HEAD.
func GetFileContent(dir, file string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	content, err := runGit(ctx, dir, diffMaxBuffer, "show", fmt.Sprintf("HEAD:%s", file))
	if err != nil {
		// File not in HEAD = new file
		return "", true, nil
	}
	return content, false, nil
}

// Check reports whether dir is inside a git work tree.
func Check(dir string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	_, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--git-dir")
	return err == nil, nil
}

// GetWorkingDiff returns the full combined diff for all working-tree changes
// (staged, unstaged, and untracked), along with per-file metadata.
func GetWorkingDiff(dir string) (*WorkingDiffResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	hasHEAD := true
	if _, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--verify", "HEAD"); err != nil {
		hasHEAD = false
	}

	var trackedDiff string
	if hasHEAD {
		var err error
		trackedDiff, err = runGit(ctx, dir, diffMaxBuffer, "diff", "-U20", "HEAD")
		if err != nil {
			return nil, err
		}
	} else {
		cachedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U20", "--cached")
		if err != nil {
			return nil, err
		}
		unstagedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U20")
		if err != nil {
			return nil, err
		}
		switch {
		case cachedDiff != "" && unstagedDiff != "":
			trackedDiff = cachedDiff + "\n" + unstagedDiff
		case cachedDiff != "":
			trackedDiff = cachedDiff
		default:
			trackedDiff = unstagedDiff
		}
	}

	// -uall enumerates individual files inside untracked directories.
	statusOut, err := runGit(ctx, dir, defaultMaxBuffer, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}

	var untrackedPaths []string
	for _, line := range strings.Split(statusOut, "\n") {
		if len(line) < 4 {
			continue
		}
		if line[0] == '?' && line[1] == '?' {
			untrackedPaths = append(untrackedPaths, line[3:])
		}
	}

	var diffParts []string
	if trackedDiff != "" {
		diffParts = append(diffParts, trackedDiff)
	}
	for _, path := range untrackedPaths {
		fileDiff, err := GetFileDiff(dir, path, false, true)
		if err != nil {
			log.Printf("warning: diff for untracked %q: %v", path, err)
			continue
		}
		if fileDiff != "" {
			diffParts = append(diffParts, fileDiff)
		}
	}

	combinedDiff := strings.Join(diffParts, "\n")

	files, totalAdds, totalDels, err := getWorkingDiffFileStats(ctx, dir, hasHEAD, untrackedPaths)
	if err != nil {
		return nil, err
	}

	return &WorkingDiffResult{
		Diff:           combinedDiff,
		Files:          files,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
	}, nil
}

func getWorkingDiffFileStats(ctx context.Context, dir string, hasHEAD bool, untrackedPaths []string) ([]CommitFile, int, int, error) {
	var files []CommitFile
	totalAdds, totalDels := 0, 0

	if hasHEAD {
		statusMap := map[string]struct {
			status  string
			oldPath string
		}{}

		nsOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--name-status", "HEAD")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("name-status: %w", err)
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

		numOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--numstat", "HEAD")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("numstat: %w", err)
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
	} else {
		nsOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--name-status", "--cached")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("name-status --cached: %w", err)
		}
		numOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--numstat", "--cached")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("numstat --cached: %w", err)
		}

		cachedStatusMap := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(nsOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) >= 2 {
				cachedStatusMap[fields[len(fields)-1]] = fields[0][:1]
			}
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
			if fields[0] != "-" {
				adds, _ = strconv.Atoi(fields[0])
				dels, _ = strconv.Atoi(fields[1])
			}
			path := normalizeNumstatPath(fields[2])
			st := StatusAdded
			if s, ok := cachedStatusMap[path]; ok && s == "M" {
				st = StatusModified
			}
			files = append(files, CommitFile{
				Path:      path,
				Status:    st,
				Additions: adds,
				Deletions: dels,
			})
			totalAdds += adds
			totalDels += dels
		}
	}

	// Untracked file numstat: uses exec.Command directly (not runGit) because
	// git diff --no-index exits 1 when files differ, and runGit discards stdout
	// on non-zero exit. Same pattern as runGitDiffNoIndex.
	for _, path := range untrackedPaths {
		adds := 0
		cmd := exec.CommandContext(ctx, "git", "diff", "--numstat", "--no-index", "/dev/null", "--", path)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
		numBytes, err := cmd.Output()
		if err != nil {
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
				err = nil
			} else {
				log.Printf("warning: numstat for untracked %q: %v", path, err)
			}
		}
		if err == nil && len(numBytes) > 0 {
			fields := strings.SplitN(strings.TrimSpace(string(numBytes)), "\t", 3)
			if len(fields) >= 1 && fields[0] != "-" {
				adds, _ = strconv.Atoi(fields[0])
			}
		}
		files = append(files, CommitFile{
			Path:      path,
			Status:    StatusAdded,
			Additions: adds,
		})
		totalAdds += adds
	}

	if files == nil {
		files = []CommitFile{}
	}
	return files, totalAdds, totalDels, nil
}
