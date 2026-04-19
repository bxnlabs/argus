package git

import (
	"context"
	"fmt"
	"strings"
)

// Fetch runs `git fetch --prune origin` in dir. It is the caller's
// responsibility to ensure dir is a valid git working tree. When the
// repository has no `origin` remote configured, the call is a no-op and nil is
// returned — this covers both fresh repos with no remotes and repos that use
// non-standard remote names. Authentication is inherited from the ambient
// environment (credential helpers, SSH agent) — the caller must ensure those
// are usable in this process.
func Fetch(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	remotesOut, err := runGit(ctx, dir, defaultMaxBuffer, "remote")
	if err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	if !hasOrigin(remotesOut) {
		return nil
	}

	if _, err := runGit(ctx, dir, defaultMaxBuffer, "fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	return nil
}

func hasOrigin(remotesOut string) bool {
	for _, line := range strings.Split(remotesOut, "\n") {
		if strings.TrimSpace(line) == "origin" {
			return true
		}
	}
	return false
}
