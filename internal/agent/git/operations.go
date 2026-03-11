package git

import (
	"bytes"
	"context"
	"fmt"
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
