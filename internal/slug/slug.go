// Package slug converts human-entered session names into identifiers safe for
// git branch names and for display.
//
// This rule began as an unexported helper in internal/git/worktree. It lives
// in its own leaf package because two unrelated layers need it: the worktree
// manager derives branch names from it, and the session read path exposes it
// on the API. Keeping it dependency-free is what lets internal/node/db use it
// without importing internal/git/worktree and inverting the layering.
package slug

import (
	"regexp"
	"strings"
)

var nonAlphanumRun = regexp.MustCompile(`[^a-z0-9]+`)

// Make converts a session name into a valid git branch name component.
// Lowercases, collapses non-alphanumeric runs to "-", trims leading/trailing
// "-". Returns "session" if the result is empty.
func Make(name string) string {
	lower := strings.ToLower(name)
	s := nonAlphanumRun.ReplaceAllString(lower, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "session"
	}
	return s
}
