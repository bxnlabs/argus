package notifications

import (
	"regexp"
	"strings"
)

// sshURLPattern matches git@host:owner/repo.git style URLs.
var sshURLPattern = regexp.MustCompile(`^git@[^:]+:(.+?)(?:\.git)?$`)

// extractRepoName extracts "owner/repo" from a git remote URL.
// Handles both HTTPS and SSH URLs. Returns empty string if input is empty.
func extractRepoName(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}

	// Try SSH format first: git@github.com:owner/repo.git
	if m := sshURLPattern.FindStringSubmatch(remoteURL); len(m) == 2 {
		return m[1]
	}

	// HTTPS format: https://github.com/owner/repo.git
	// Strip scheme (e.g. "https://") before splitting on "/"
	p := strings.TrimSuffix(remoteURL, ".git")
	p = strings.TrimRight(p, "/")
	if idx := strings.Index(p, "://"); idx >= 0 {
		p = p[idx+3:] // skip past "://"
	}
	// p is now "github.com/owner/repo" or "github.com/repo"
	// Drop the host segment and take the remaining path segments
	parts := strings.SplitN(p, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		// Nothing after the host — shouldn't normally happen, but handle gracefully
		return ""
	}
	pathParts := strings.Split(parts[1], "/")
	if len(pathParts) >= 2 {
		return pathParts[len(pathParts)-2] + "/" + pathParts[len(pathParts)-1]
	}
	return pathParts[0]
}

// homePattern matches /home/<user>/ or /Users/<user>/ prefixes.
var homePattern = regexp.MustCompile(`^/(home|Users)/[^/]+/`)

// compressHomePath replaces /home/<user>/ or /Users/<user>/ with ~/.
func compressHomePath(p string) string {
	if p == "" {
		return ""
	}
	return homePattern.ReplaceAllString(p, "~/")
}

// buildLocationLine constructs the repo/path display string for a notification.
// Returns (repoLine, localPathLine, branchLine). Any may be empty.
func buildLocationLine(gitRemoteURL, gitParentDir, workingDir *string, worktreeBranch *string) (repo, localPath, branch string) {
	if gitRemoteURL != nil && *gitRemoteURL != "" {
		repo = extractRepoName(*gitRemoteURL)
		if gitParentDir != nil && *gitParentDir != "" {
			localPath = compressHomePath(*gitParentDir)
		}
	} else if gitParentDir != nil && *gitParentDir != "" {
		repo = compressHomePath(*gitParentDir)
		// localPath omitted — would be redundant
	} else if workingDir != nil && *workingDir != "" {
		repo = compressHomePath(*workingDir)
	}

	if worktreeBranch != nil && *worktreeBranch != "" {
		branch = *worktreeBranch
	}

	return repo, localPath, branch
}
