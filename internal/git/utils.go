package git

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// Run executes a git command in the given directory. If dir is empty, the
// command runs in the process's current directory.
func Run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return fmt.Errorf("git: %w", err)
	}
	return nil
}

// Output executes a git command and returns its stdout.
func Output(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// FindMainRepo returns the root of the main repository for the given
// directory, which may be the main working tree or a linked worktree.
// It uses "git rev-parse --git-common-dir" which returns the shared .git
// directory (e.g. /path/to/main/.git), then takes its parent.
//
// Note: this assumes a standard repo layout. It does not handle submodules,
// bare repos, or repos with a separate git directory (--separate-git-dir).
func FindMainRepo(dir string) (string, error) {
	out, err := Output(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("git-common-dir: %w", err)
	}
	gitCommon := strings.TrimSpace(out)
	if !filepath.IsAbs(gitCommon) {
		gitCommon = filepath.Join(dir, gitCommon)
	}
	gitCommon, err = filepath.EvalSymlinks(gitCommon)
	if err != nil {
		return "", fmt.Errorf("resolve git-common-dir: %w", err)
	}
	return filepath.Dir(gitCommon), nil
}

// BranchExists reports whether a local branch with the given name exists.
func BranchExists(repoDir, branch string) (bool, error) {
	out, err := Output(repoDir, "branch", "--list", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// RemoteURL returns the URL of the "origin" remote for the repo at dir.
func RemoteURL(dir string) (string, error) {
	out, err := Output(dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("get remote url: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// SanitizeRemoteURL strips userinfo (credentials/tokens) from HTTPS/HTTP
// remote URLs. SSH URLs are returned as-is.
func SanitizeRemoteURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" {
		return rawURL // SSH or unparseable — return as-is
	}
	u.User = nil
	return u.String()
}

// DefaultBranch returns the repo's default branch name.
// Tries: origin/HEAD symbolic ref → local "main" → local "master" →
// remote origin/main → remote origin/master → error.
func DefaultBranch(repoDir string) (string, error) {
	out, err := Output(repoDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out)
		if ref != "" {
			parts := strings.Split(ref, "/")
			return parts[len(parts)-1], nil
		}
	}

	for _, branch := range []string{"main", "master"} {
		exists, err := BranchExists(repoDir, branch)
		if err == nil && exists {
			return branch, nil
		}
	}

	for _, branch := range []string{"main", "master"} {
		out, err := Output(repoDir, "branch", "-r", "--list", "origin/"+branch)
		if err == nil && strings.TrimSpace(out) != "" {
			return branch, nil
		}
	}

	return "", fmt.Errorf("cannot determine default branch for repo at %s", repoDir)
}
