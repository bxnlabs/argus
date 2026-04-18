package git

import (
	"context"
	"fmt"
)

// Fetch runs `git fetch --prune` against all configured remotes in dir. It is
// the caller's responsibility to ensure dir is a valid git working tree. When
// the repository has no remotes configured, the underlying command is a no-op
// and nil is returned. Authentication is inherited from the ambient
// environment (credential helpers, SSH agent) — the caller must ensure those
// are usable in this process.
func Fetch(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	if _, err := runGit(ctx, dir, defaultMaxBuffer, "fetch", "--all", "--prune"); err != nil {
		return fmt.Errorf("git fetch failed: %w", err)
	}
	return nil
}
