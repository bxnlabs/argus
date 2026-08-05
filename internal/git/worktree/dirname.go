package worktree

import (
	"strings"
)

// worktreeDirName converts a branch name to a safe directory name by
// replacing "/" with "--". e.g. "jeev/fix-auth" → "jeev--fix-auth".
//
// NOTE: this mapping is lossy and therefore not injective — "a/b" and "a--b"
// both map to "a--b". Two such branches would collide on the same worktree
// directory; the second worktree creation fails with a git error surfaced as a
// generic 500 (see BXN-121). A branch named literally "a--b" is pathological, so
// this is tracked debt rather than a fix here; a collision-resistant scheme
// (e.g. a short hash suffix) would resolve both the collision and the error
// classification.
func worktreeDirName(branch string) string {
	return strings.ReplaceAll(branch, "/", "--")
}
