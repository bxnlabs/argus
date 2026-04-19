package git

import (
	"context"
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

	if err := Fetch(context.Background(), dir); err != nil {
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

	if err := Fetch(context.Background(), dir); err != nil {
		t.Fatalf("Fetch with no remote: %v", err)
	}
}

// TestFetch_BadDirReturnsError verifies that Fetch surfaces git's error when
// the directory is not a git working tree.
func TestFetch_BadDirReturnsError(t *testing.T) {
	if err := Fetch(context.Background(), t.TempDir()); err == nil {
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

	if err := Fetch(context.Background(), dir); err != nil {
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

	if err := Fetch(context.Background(), dir); err != nil {
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

	if err := Fetch(context.Background(), dir); err != nil {
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

	if err := Fetch(context.Background(), dir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// After prune, the tracking ref must be gone.
	post := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/feature")
	post.Dir = dir
	if err := post.Run(); err == nil {
		t.Error("origin/feature should have been pruned")
	}
}
