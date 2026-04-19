package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestFetch_AdvancesOriginWhenRemoteMoved verifies that Fetch updates the
// local origin/<branch> tracking ref after the remote advances.
func TestFetch_AdvancesOriginWhenRemoteMoved(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)

	// Advance remote.
	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(context.Background(), dir, ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// After fetch, local origin/main must include the advance.
	cmd := exec.Command("git", "log", "--oneline", "origin/main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "advance") {
		t.Errorf("origin/main did not advance; log:\n%s", out)
	}
}

// TestFetch_NoRemoteIsNoop verifies that fetching a repo with no remotes
// configured succeeds and does nothing.
func TestFetch_NoRemoteIsNoop(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "a", "init")

	if err := Fetch(context.Background(), dir, ""); err != nil {
		t.Fatalf("Fetch with no remote: %v", err)
	}
}

// TestFetch_BadDirReturnsError verifies that Fetch surfaces git's error when
// the directory is not a git working tree.
func TestFetch_BadDirReturnsError(t *testing.T) {
	if err := Fetch(context.Background(), t.TempDir(), ""); err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

// TestFetch_UsesHEADUpstreamRemote verifies that Fetch targets the remote
// HEAD's upstream points at, so fork-style workflows (where HEAD tracks
// upstream/*, not origin/*) keep their tracking refs fresh.
func TestFetch_UsesHEADUpstreamRemote(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)

	// Rename the only remote from "origin" to "upstream"; main now tracks
	// upstream/main (git remote rename rewrites branch.*.remote config too).
	gitInDir(t, dir, "remote", "rename", "origin", "upstream")

	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(context.Background(), dir, ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	cmd := exec.Command("git", "log", "--oneline", "upstream/main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "advance") {
		t.Errorf("upstream/main did not advance; log:\n%s", out)
	}
}

// TestFetch_HandlesRemoteNameContainingSlash verifies that Fetch correctly
// resolves an upstream when the remote's name itself contains '/' — git
// permits names like "team/upstream", which would produce an upstream ref
// of "team/upstream/main" that a naive first-slash split would misparse.
func TestFetch_HandlesRemoteNameContainingSlash(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)

	gitInDir(t, dir, "remote", "rename", "origin", "team/upstream")

	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(context.Background(), dir, ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	cmd := exec.Command("git", "log", "--oneline", "team/upstream/main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "advance") {
		t.Errorf("team/upstream/main did not advance; log:\n%s", out)
	}
}

// TestFetch_FallsBackToOriginWhenHEADHasNoUpstream verifies that when HEAD has
// no upstream tracking branch, Fetch falls back to `origin` if it exists.
func TestFetch_FallsBackToOriginWhenHEADHasNoUpstream(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)

	// Create a local branch with no upstream configured.
	gitInDir(t, dir, "checkout", "-b", "no-upstream")

	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(context.Background(), dir, ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	cmd := exec.Command("git", "log", "--oneline", "origin/main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "advance") {
		t.Errorf("origin/main did not advance via origin fallback; log:\n%s", out)
	}
}

// TestFetch_PrunesDeletedRemoteBranch verifies that Fetch removes the local
// tracking ref when the corresponding branch has been deleted on the remote.
// This guards the --prune contract — a regression to plain `git fetch` would
// leave origin/<branch> in place and this test would catch it.
func TestFetch_PrunesDeletedRemoteBranch(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	// Create a branch on the remote so the client has something to prune.
	gitInDir(t, remote, "checkout", "-b", "feature")
	gitInDir(t, remote, "checkout", "main")

	dir := cloneTestRepo(t, remote)

	// Precondition: origin/feature exists locally after clone.
	pre := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/feature")
	pre.Dir = dir
	if err := pre.Run(); err != nil {
		t.Fatalf("origin/feature missing after clone: %v", err)
	}

	// Delete the branch on the remote.
	gitInDir(t, remote, "branch", "-D", "feature")

	if err := Fetch(context.Background(), dir, ""); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// After prune, the tracking ref must be gone.
	post := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/feature")
	post.Dir = dir
	if err := post.Run(); err == nil {
		t.Error("origin/feature should have been pruned")
	}
}

// TestFetch_FetchesBaseUpstreamRemote covers the fork workflow: HEAD's
// upstream and the compare base's upstream live on different remotes. Without
// fetching the base's remote, the compare stale-base banner can never be
// unstuck — clicking refresh would only update HEAD's remote.
func TestFetch_FetchesBaseUpstreamRemote(t *testing.T) {
	mainRemote := initTestRepo(t)
	commitFile(t, mainRemote, "base.txt", "base", "base commit")

	// Clone from mainRemote so origin == mainRemote and local main tracks it.
	dir := cloneTestRepo(t, mainRemote)

	// Add a second remote that holds the user's fork branch and create a
	// local feature branch tracking it. Now: main → origin (mainRemote),
	// feature → fork (forkRemote).
	forkRemote := initTestRepo(t)
	commitFile(t, forkRemote, "fork-base.txt", "fork base", "fork base")
	gitInDir(t, forkRemote, "checkout", "-b", "feature")
	commitFile(t, forkRemote, "feature.txt", "feature", "feature commit")
	gitInDir(t, forkRemote, "checkout", "main")

	gitInDir(t, dir, "remote", "add", "fork", forkRemote)
	gitInDir(t, dir, "fetch", "fork")
	gitInDir(t, dir, "checkout", "-b", "feature", "--track", "fork/feature")

	// Advance both remotes independently so we can verify each gets fetched.
	// fork-advance lands on `feature` (the branch the local feature tracks),
	// not on fork's main — otherwise fork/feature wouldn't change.
	commitFile(t, mainRemote, "main-advance.txt", "main", "main advance")
	gitInDir(t, forkRemote, "checkout", "feature")
	commitFile(t, forkRemote, "fork-advance.txt", "fork", "fork advance")

	// HEAD == feature (tracks fork/feature). Compare base == main (tracks
	// origin/main). Fetch must update both remote tracking refs.
	if err := Fetch(context.Background(), dir, "main"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	mainLog := exec.Command("git", "log", "--oneline", "origin/main")
	mainLog.Dir = dir
	if out, err := mainLog.CombinedOutput(); err != nil {
		t.Fatalf("git log origin/main: %v\n%s", err, out)
	} else if !strings.Contains(string(out), "main advance") {
		t.Errorf("origin/main did not advance; log:\n%s", out)
	}

	forkLog := exec.Command("git", "log", "--oneline", "fork/feature")
	forkLog.Dir = dir
	if out, err := forkLog.CombinedOutput(); err != nil {
		t.Fatalf("git log fork/feature: %v\n%s", err, out)
	} else if !strings.Contains(string(out), "fork advance") {
		t.Errorf("fork/feature did not advance; log:\n%s", out)
	}
}

// TestFetch_DedupesWhenHEADAndBaseShareRemote verifies that we don't fetch
// the same remote twice when HEAD and base both track it (the common case for
// non-fork workflows).
func TestFetch_DedupesWhenHEADAndBaseShareRemote(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")
	gitInDir(t, remote, "checkout", "-b", "feature")
	commitFile(t, remote, "feature.txt", "feature", "feature commit")
	gitInDir(t, remote, "checkout", "main")

	dir := cloneTestRepo(t, remote)
	gitInDir(t, dir, "checkout", "-b", "feature", "--track", "origin/feature")

	targets, err := fetchTargets(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("fetchTargets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "origin" {
		t.Errorf("expected [origin], got %v", targets)
	}
}

// TestFetch_IgnoresInvalidBase verifies that a malformed base value (here, a
// leading dash that could be misinterpreted as a git option) doesn't poison
// the fetch — HEAD's upstream still gets refreshed.
func TestFetch_IgnoresInvalidBase(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)
	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(context.Background(), dir, "-malicious"); err != nil {
		t.Fatalf("Fetch with invalid base should still succeed: %v", err)
	}

	cmd := exec.Command("git", "log", "--oneline", "origin/main")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "advance") {
		t.Errorf("origin/main did not advance; invalid base poisoned fetch:\n%s", out)
	}
}

// TestFetch_WrapsErrorAsErrFetchFailed verifies that an underlying `git fetch`
// failure (here, a remote URL pointing at a non-existent path) returns an
// error matching ErrFetchFailed and carrying the remote name plus git's
// stderr — the only signal the user has for what to fix.
func TestFetch_WrapsErrorAsErrFetchFailed(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "a", "init")

	// Configure a remote pointing at a directory that doesn't exist.
	gitInDir(t, dir, "remote", "add", "origin", t.TempDir()+"/does-not-exist")

	err := Fetch(context.Background(), dir, "")
	if err == nil {
		t.Fatal("expected error from fetch against missing remote, got nil")
	}
	if !errors.Is(err, ErrFetchFailed) {
		t.Errorf("expected errors.Is(err, ErrFetchFailed), got: %v", err)
	}
	if !strings.HasPrefix(err.Error(), "origin: ") {
		t.Errorf("expected message to start with remote name, got: %q", err.Error())
	}
	// runGit's "git fetch:" prefix should be stripped — the toast prefix at
	// the UI already conveys that.
	if strings.Contains(err.Error(), "git fetch:") {
		t.Errorf("error message should not contain duplicated 'git fetch:' prefix, got: %q", err.Error())
	}
}

// TestFetch_HEADFirstThenBase verifies that fetchTargets places HEAD's
// upstream remote before the base's, so a downstream best-effort iteration
// still advances HEAD's tracking refs (the most user-visible freshness
// signal) before reaching a potentially-broken base remote.
//
// The HEAD remote name is chosen to sort AFTER "origin" alphabetically — a
// regression to alphabetical ordering would flip the order to
// [origin, zeta-fork] and fail this test.
func TestFetch_HEADFirstThenBase(t *testing.T) {
	mainRemote := initTestRepo(t)
	commitFile(t, mainRemote, "base.txt", "base", "base commit")
	dir := cloneTestRepo(t, mainRemote)

	forkRemote := initTestRepo(t)
	commitFile(t, forkRemote, "fork.txt", "fork", "fork commit")
	gitInDir(t, forkRemote, "checkout", "-b", "feature")

	gitInDir(t, dir, "remote", "add", "zeta-fork", forkRemote)
	gitInDir(t, dir, "fetch", "zeta-fork")
	gitInDir(t, dir, "checkout", "-b", "feature", "--track", "zeta-fork/feature")

	// HEAD == feature → zeta-fork; base == main → origin. Expect
	// [zeta-fork, origin] under insertion order; alphabetical sort would
	// produce [origin, zeta-fork] and fail this assertion.
	targets, err := fetchTargets(context.Background(), dir, "main")
	if err != nil {
		t.Fatalf("fetchTargets: %v", err)
	}
	want := []string{"zeta-fork", "origin"}
	if len(targets) != len(want) {
		t.Fatalf("expected %v, got %v", want, targets)
	}
	for i, r := range targets {
		if r != want[i] {
			t.Errorf("position %d: expected %q, got %q (full: %v)", i, want[i], r, targets)
		}
	}
}

// TestFetch_BestEffortContinuesPastFailure verifies that one failing remote
// doesn't block the other from being fetched. A short-circuit regression
// here would let a dead/auth-broken secondary remote keep the primary's
// tracking refs from advancing — exactly the scenario the multi-remote work
// was meant to fix.
func TestFetch_BestEffortContinuesPastFailure(t *testing.T) {
	mainRemote := initTestRepo(t)
	commitFile(t, mainRemote, "base.txt", "base", "base commit")
	dir := cloneTestRepo(t, mainRemote)

	// Set up tracking against a working fork first so we can configure
	// upstream cleanly, then point the fork remote at a missing path so the
	// next fetch against it fails. This simulates auth/network breakage
	// without needing a credential-rejecting remote.
	forkRemote := initTestRepo(t)
	commitFile(t, forkRemote, "fork.txt", "fork", "fork commit")
	gitInDir(t, forkRemote, "checkout", "-b", "feature")

	gitInDir(t, dir, "remote", "add", "fork", forkRemote)
	gitInDir(t, dir, "fetch", "fork")
	gitInDir(t, dir, "checkout", "-b", "feature", "--track", "fork/feature")
	gitInDir(t, dir, "remote", "set-url", "fork", t.TempDir()+"/does-not-exist")

	// Advance the healthy remote so the post-fetch assertion has something to
	// observe.
	commitFile(t, mainRemote, "advance.txt", "x", "advance")

	err := Fetch(context.Background(), dir, "main")
	if err == nil {
		t.Fatal("expected error from broken fork remote, got nil")
	}
	if !errors.Is(err, ErrFetchFailed) {
		t.Errorf("expected errors.Is(err, ErrFetchFailed), got: %v", err)
	}
	if !strings.Contains(err.Error(), "fork") {
		t.Errorf("expected error message to mention failing 'fork' remote, got: %q", err.Error())
	}

	// The crucial assertion: origin/main must still have advanced even though
	// the fork fetch failed. A short-circuit regression would skip this fetch
	// entirely (HEAD-first ordering means fork is attempted first).
	cmd := exec.Command("git", "log", "--oneline", "origin/main")
	cmd.Dir = dir
	out, logErr := cmd.CombinedOutput()
	if logErr != nil {
		t.Fatalf("git log origin/main: %v\n%s", logErr, out)
	}
	if !strings.Contains(string(out), "advance") {
		t.Errorf("origin/main did not advance despite healthy remote; log:\n%s", out)
	}
}
