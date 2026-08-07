package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const fetchTimeout = 60 * time.Second

// Fetch runs `git fetch --prune` for the remotes whose tracking refs drive the
// UI's freshness signals: HEAD's upstream remote (so status ahead/behind stays
// fresh) and, when `base` is non-empty, the base branch's upstream remote (so
// the compare stale-base detector can be unstuck — without this, fork
// workflows where HEAD tracks `origin/*` but the compare base tracks
// `upstream/*` would never refresh `upstream/*` on click). When neither
// upstream resolves, falls back to `origin` if configured. When no suitable
// remote is configured at all, the call is a no-op and nil is returned.
//
// `base` is the local branch name selected as the compare base (e.g. "main"),
// or empty when the caller has no compare base in scope. An invalid `base`
// value is silently ignored — fetching HEAD's remote is more useful than
// failing the whole refresh on a malformed compare selector.
//
// Selected remotes are fetched best-effort: every remote is attempted even if
// an earlier one fails, so a broken secondary remote (dead URL, expired auth)
// can't block the other from advancing. Per-remote failures are aggregated
// and returned wrapped so errors.Is(err, ErrFetchFailed) matches and the
// underlying git stderr is preserved for the user-facing toast.
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
func Fetch(ctx context.Context, dir, base string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	targets, err := fetchTargets(ctx, dir, base)
	if err != nil {
		return err
	}
	var fetchErrs []error
	for _, remote := range targets {
		if _, err := runGit(ctx, dir, defaultMaxBuffer, "fetch", "--prune", remote); err != nil {
			fetchErrs = append(fetchErrs, wrapFetchError(remote, err))
		}
	}
	if len(fetchErrs) == 0 {
		return nil
	}
	if len(fetchErrs) == 1 {
		return fetchErrs[0]
	}
	// errors.Join preserves errors.Is matching against ErrFetchFailed via
	// each wrapped fetchFailureError's Is method, and produces a multi-line
	// message the toast can render verbatim.
	return errors.Join(fetchErrs...)
}

// fetchTargets returns the deduped, ordered set of remotes to fetch in Fetch.
// HEAD's upstream remote is placed first so the most user-visible freshness
// signal (status ahead/behind) advances even if a later remote fails. Base's
// upstream remote follows. See Fetch's doc comment for the selection rules.
func fetchTargets(ctx context.Context, dir, base string) ([]string, error) {
	remotesOut, err := runGit(ctx, dir, defaultMaxBuffer, "remote")
	if err != nil {
		return nil, err
	}
	remotes := parseRemotes(remotesOut)
	if len(remotes) == 0 {
		return nil, nil
	}

	var ordered []string
	seen := map[string]bool{}
	addUpstream := func(ref string) error {
		remote, err := upstreamRemote(ctx, dir, ref, remotes)
		if err != nil {
			return err
		}
		if remote == "" || seen[remote] {
			return nil
		}
		seen[remote] = true
		ordered = append(ordered, remote)
		return nil
	}

	if err := addUpstream("HEAD"); err != nil {
		return nil, err
	}
	if base != "" {
		switch err := validateBranchRef(ctx, dir, base); {
		case err == nil:
			if err := addUpstream(base); err != nil {
				return nil, err
			}
		case isOperationalError(err):
			return nil, err
		}
	}

	if len(ordered) == 0 {
		if _, ok := remotes["origin"]; ok {
			return []string{"origin"}, nil
		}
		return nil, nil
	}
	return ordered, nil
}

// upstreamRemote returns the remote backing ref's upstream, or "" when there
// is none to find.
//
// `ref@{upstream}` fails whenever the branch has no upstream configured
// (detached HEAD, branch without tracking, missing local ref), which is an
// ordinary answer of "none" rather than a failure. A blown deadline is not one
// of those: reported as "none" it would leave the caller with no targets and
// let Fetch report a fetch that never ran as a success.
func upstreamRemote(ctx context.Context, dir, ref string, remotes map[string]struct{}) (string, error) {
	out, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--abbrev-ref", ref+"@{upstream}")
	if err != nil {
		if isOperationalError(err) {
			return "", err
		}
		return "", nil
	}
	return matchRemote(strings.TrimSpace(out), remotes), nil
}

// matchRemote picks the longest-matching remote name whose `<name>/` is a
// prefix of upstream. Longest-match handles remote names that contain '/'
// (git permits e.g. "team/upstream"), where a naive first-slash split would
// pick "team" instead of the actual remote.
func matchRemote(upstream string, remotes map[string]struct{}) string {
	var best string
	for r := range remotes {
		if strings.HasPrefix(upstream, r+"/") && len(r) > len(best) {
			best = r
		}
	}
	return best
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

// wrapFetchError turns a runGit fetch failure into an error that satisfies
// errors.Is(err, ErrFetchFailed) and carries a clean, user-facing message
// (`<remote>: <git stderr>`). Strips runGit's "git fetch:" prefix because the
// UI's "Git fetch failed:" toast prefix already says it.
func wrapFetchError(remote string, err error) error {
	msg := strings.TrimPrefix(err.Error(), "git fetch: ")
	return &fetchFailureError{msg: fmt.Sprintf("%s: %s", remote, msg), cause: err}
}

type fetchFailureError struct {
	msg string
	// cause keeps the chain reachable. Building the message from err.Error()
	// alone dropped it, so a fetch that timed out matched only ErrFetchFailed
	// and answered 502 — respondGitError tests ErrTimeout first, but only if
	// it can still see it.
	cause error
}

func (e *fetchFailureError) Error() string { return e.msg }
func (e *fetchFailureError) Unwrap() error { return e.cause }
func (e *fetchFailureError) Is(target error) bool {
	return target == ErrFetchFailed
}
