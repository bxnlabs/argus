package git

import (
	"errors"
	"os/exec"
	"testing"
)

func TestGetBranches(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa", "initial commit")

	t.Run("returns branches", func(t *testing.T) {
		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Branches) == 0 {
			t.Error("expected at least one branch")
		}
	})

	t.Run("default base with no upstream", func(t *testing.T) {
		// Use a separate repo whose only branch is "develop" (neither main nor master)
		noDefaultDir := initTestRepo(t)
		cmd := exec.Command("git", "checkout", "-b", "develop")
		cmd.Dir = noDefaultDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git checkout -b develop failed: %s: %s", err, out)
		}
		commitFile(t, noDefaultDir, "a.txt", "aaa", "initial commit")

		result, err := GetBranches(noDefaultDir)
		if err != nil {
			t.Fatal(err)
		}
		// No upstream configured, no main/master branch => empty default
		if result.DefaultBase != "" {
			t.Errorf("expected empty default base, got %q", result.DefaultBase)
		}
	})

	t.Run("default base falls back to main", func(t *testing.T) {
		// Create a "main" branch
		cmd := exec.Command("git", "branch", "main")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch main failed: %s: %s", err, out)
		}

		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.DefaultBase != "main" {
			t.Errorf("expected default base %q, got %q", "main", result.DefaultBase)
		}
	})

	t.Run("includes feature branches", func(t *testing.T) {
		cmd := exec.Command("git", "branch", "feature/test")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git branch failed: %s: %s", err, out)
		}

		result, err := GetBranches(dir)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, b := range result.Branches {
			if b == "feature/test" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected feature/test in branches, got %v", result.Branches)
		}
	})
}

func TestGetCompare(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "base.txt", "base content", "base commit")

	// Create a feature branch and add changes
	for _, args := range [][]string{
		{"git", "checkout", "-b", "feature"},
		{"git", "checkout", "-b", "main", "HEAD~0"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.CombinedOutput()
	}
	// Switch back to set up main as a branch at current commit
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s: %s", args, err, out)
		}
	}
	run("git", "checkout", "feature")
	commitFile(t, dir, "new.txt", "new content", "add new file")
	commitFile(t, dir, "base.txt", "modified content", "modify base file")

	t.Run("returns diff and files", func(t *testing.T) {
		result, err := GetCompare(dir, "main")
		if err != nil {
			t.Fatal(err)
		}
		if result.Diff == "" {
			t.Error("expected non-empty diff")
		}
		if len(result.Files) != 2 {
			t.Fatalf("expected 2 files, got %d", len(result.Files))
		}
		if result.TotalAdditions == 0 {
			t.Error("expected additions > 0")
		}
		if result.BaseRef == "" || result.HeadRef == "" {
			t.Error("expected baseRef and headRef")
		}
	})

	t.Run("invalid branch name wraps ErrInvalidInput", func(t *testing.T) {
		_, err := GetCompare(dir, "-bad-branch")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("nonexistent branch wraps ErrNotFound", func(t *testing.T) {
		_, err := GetCompare(dir, "nonexistent")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("invalid base ref", func(t *testing.T) {
		_, err := GetCompare(dir, "nonexistent-branch")
		if err == nil {
			t.Error("expected error for invalid base ref")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("disconnected histories wraps ErrNotFound", func(t *testing.T) {
		// Create an orphan branch with no common ancestor
		run("git", "checkout", "--orphan", "orphan-branch")
		commitFile(t, dir, "orphan.txt", "orphan", "orphan commit")
		run("git", "checkout", "feature")

		_, err := GetCompare(dir, "orphan-branch")
		if err == nil {
			t.Fatal("expected error for disconnected histories")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}
