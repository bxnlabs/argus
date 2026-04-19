package git

import (
	"os/exec"
	"strings"
	"testing"
)

// TestFetch_AdvancesOriginWhenRemoteMoved verifies that Fetch updates the
// local origin/<branch> tracking ref after the remote advances.
func TestFetch_AdvancesOriginWhenRemoteMoved(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	// Advance remote.
	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(dir); err != nil {
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

	if err := Fetch(dir); err != nil {
		t.Fatalf("Fetch with no remote: %v", err)
	}
}

// TestFetch_BadDirReturnsError verifies that Fetch surfaces git's error when
// the directory is not a git working tree.
func TestFetch_BadDirReturnsError(t *testing.T) {
	if err := Fetch(t.TempDir()); err == nil {
		t.Error("expected error for non-git directory, got nil")
	}
}

// TestFetch_UsesHEADUpstreamRemote verifies that Fetch targets the remote
// HEAD's upstream points at, so fork-style workflows (where HEAD tracks
// upstream/*, not origin/*) keep their tracking refs fresh.
func TestFetch_UsesHEADUpstreamRemote(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	// Rename the only remote from "origin" to "upstream"; main now tracks
	// upstream/main (git remote rename rewrites branch.*.remote config too).
	rename := exec.Command("git", "remote", "rename", "origin", "upstream")
	rename.Dir = dir
	if out, err := rename.CombinedOutput(); err != nil {
		t.Fatalf("git remote rename: %v\n%s", err, out)
	}

	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(dir); err != nil {
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

	dir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	rename := exec.Command("git", "remote", "rename", "origin", "team/upstream")
	rename.Dir = dir
	if out, err := rename.CombinedOutput(); err != nil {
		t.Fatalf("git remote rename: %v\n%s", err, out)
	}

	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(dir); err != nil {
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

	dir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	// Create a local branch with no upstream configured.
	checkout := exec.Command("git", "checkout", "-b", "no-upstream")
	checkout.Dir = dir
	if out, err := checkout.CombinedOutput(); err != nil {
		t.Fatalf("git checkout: %v\n%s", err, out)
	}

	commitFile(t, remote, "ham.txt", "ham", "advance")

	if err := Fetch(dir); err != nil {
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
	for _, args := range [][]string{
		{"git", "checkout", "-b", "feature"},
		{"git", "checkout", "main"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = remote
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}

	dir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	// Precondition: origin/feature exists locally after clone.
	pre := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/feature")
	pre.Dir = dir
	if err := pre.Run(); err != nil {
		t.Fatalf("origin/feature missing after clone: %v", err)
	}

	// Delete the branch on the remote.
	del := exec.Command("git", "branch", "-D", "feature")
	del.Dir = remote
	if out, err := del.CombinedOutput(); err != nil {
		t.Fatalf("git branch -D feature: %v\n%s", err, out)
	}

	if err := Fetch(dir); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// After prune, the tracking ref must be gone.
	post := exec.Command("git", "rev-parse", "--verify", "refs/remotes/origin/feature")
	post.Dir = dir
	if err := post.Run(); err == nil {
		t.Error("origin/feature should have been pruned")
	}
}
