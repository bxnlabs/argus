package git

import (
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
