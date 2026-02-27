package worktree

import (
	"regexp"
	"strings"
)

var nonAlphanumRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a session name into a valid git branch name component.
// Lowercases, collapses non-alphanumeric runs to "-", trims leading/trailing
// "-". Returns "session" if the result is empty.
func slugify(name string) string {
	lower := strings.ToLower(name)
	slug := nonAlphanumRun.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "session"
	}
	return slug
}

// worktreeDirName converts a branch name to a safe directory name by
// replacing "/" with "--". e.g. "jeev/fix-auth" → "jeev--fix-auth".
func worktreeDirName(branch string) string {
	return strings.ReplaceAll(branch, "/", "--")
}
