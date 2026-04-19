package git

import (
	"context"
	"strings"
	"time"
)

const fetchTimeout = 60 * time.Second

// Fetch runs `git fetch --prune` for the remote whose tracking refs drive the
// UI's freshness signals (status ahead/behind, compare stale-base). It prefers
// HEAD's upstream remote — so fork workflows where HEAD tracks `upstream/*`
// update the right refs — and falls back to `origin` when HEAD has no
// upstream. When no suitable remote is configured the call is a no-op and nil
// is returned.
//
// Unlike other operations in this package, Fetch accepts a context: it is the
// only network-bound op and the only one with a 60s budget, so cancellation
// (e.g. an HTTP client disconnect) should kill the in-flight git subprocess
// rather than letting it run to completion.
//
// It is the caller's responsibility to ensure dir is a valid git working
// tree. Authentication is inherited from the ambient environment (credential
// helpers, SSH agent) — the caller must ensure those are usable in this
// process.
func Fetch(ctx context.Context, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	remote, err := refreshRemote(ctx, dir)
	if err != nil {
		return err
	}
	if remote == "" {
		return nil
	}

	_, err = runGit(ctx, dir, defaultMaxBuffer, "fetch", "--prune", remote)
	return err
}

// refreshRemote selects the remote to fetch in Fetch. See Fetch's doc comment
// for the selection rules.
func refreshRemote(ctx context.Context, dir string) (string, error) {
	remotesOut, err := runGit(ctx, dir, defaultMaxBuffer, "remote")
	if err != nil {
		return "", err
	}
	remotes := parseRemotes(remotesOut)
	if len(remotes) == 0 {
		return "", nil
	}

	// HEAD's upstream typically looks like "origin/main" or "upstream/main".
	// An error here usually means HEAD has no upstream configured (detached,
	// or a branch without tracking) — fall through to the origin fallback.
	// Match the upstream against configured remote names by longest prefix so
	// names containing `/` (e.g. "team/upstream") resolve correctly.
	if out, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--abbrev-ref", "HEAD@{upstream}"); err == nil {
		upstream := strings.TrimSpace(out)
		var best string
		for r := range remotes {
			if strings.HasPrefix(upstream, r+"/") && len(r) > len(best) {
				best = r
			}
		}
		if best != "" {
			return best, nil
		}
	}

	if _, ok := remotes["origin"]; ok {
		return "origin", nil
	}
	return "", nil
}

func parseRemotes(remotesOut string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, line := range strings.Split(remotesOut, "\n") {
		if name := strings.TrimSpace(line); name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}
