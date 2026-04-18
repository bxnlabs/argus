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
