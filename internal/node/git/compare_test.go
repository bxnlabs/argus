package git

import (
	"errors"
	"os/exec"
	"strings"
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
		// Ensure a "main" branch exists (may already be the initial branch)
		cmd := exec.Command("git", "branch", "main")
		cmd.Dir = dir
		cmd.CombinedOutput() // ignore error if main already exists

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

func TestGetAllBranches(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa", "initial commit")

	cmd := exec.Command("git", "branch", "feature/local")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

	branches, err := GetAllBranches(dir)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, b := range branches {
		found[b] = true
	}
	if !found["feature/local"] {
		t.Errorf("expected feature/local in branches, got %v", branches)
	}
}

func TestGetAllBranchesFiltersOriginHead(t *testing.T) {
	remote := initTestRepo(t)
	commitFile(t, remote, "a.txt", "aaa", "initial commit")

	dir := t.TempDir()
	cmd := exec.Command("git", "clone", remote, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}

	branches, err := GetAllBranches(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, b := range branches {
		if b == "HEAD" {
			t.Error("origin/HEAD should be filtered out, but 'HEAD' appeared in branches")
		}
	}
}

func TestValidateBranchName(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa", "initial commit")

	if err := ValidateBranchName(dir, "valid-branch"); err != nil {
		t.Errorf("expected valid branch name to pass, got: %v", err)
	}

	if err := ValidateBranchName(dir, "-invalid"); err == nil {
		t.Error("expected leading dash to fail validation")
	}

	if err := ValidateBranchName(dir, "has spaces"); err == nil {
		t.Error("expected branch with spaces to fail validation")
	}
}

func TestGetCompare(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "base.txt", "base content", "base commit")

	gitInDir(t, dir, "checkout", "-b", "feature")
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
		gitInDir(t, dir, "checkout", "--orphan", "orphan-branch")
		commitFile(t, dir, "orphan.txt", "orphan", "orphan commit")
		gitInDir(t, dir, "checkout", "feature")

		_, err := GetCompare(dir, "orphan-branch")
		if err == nil {
			t.Fatal("expected error for disconnected histories")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

// setupStaleBaseRepo creates a clone whose local `main` is `nCommitsAhead`
// commits behind `origin/main`, with a `feature` branch cut from the freshest
// upstream tip and carrying one `feature.txt` commit. Shared by the tests that
// exercise the stale-base substitution.
func setupStaleBaseRepo(t *testing.T, nCommitsAhead int) string {
	t.Helper()
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)

	for i := 0; i < nCommitsAhead; i++ {
		name := string(rune('a' + i)) + ".txt"
		commitFile(t, remote, name, name, "advance "+name)
	}
	gitInDir(t, dir, "fetch", "origin")
	gitInDir(t, dir, "checkout", "-b", "feature", "origin/main")
	commitFile(t, dir, "feature.txt", "mine", "feature commit")
	return dir
}

func TestGetCompare_FallsBackToUpstreamWhenLocalBaseIsStale(t *testing.T) {
	// Simulate the real-world bug: local `main` is behind `origin/main`, and the
	// feature branch was cut from the upstream tip. The compare must show only
	// the feature branch's own changes, not the commits the local base is
	// missing.
	dir := setupStaleBaseRepo(t, 1)

	result, err := GetCompare(dir, "main")
	if err != nil {
		t.Fatalf("GetCompare: %v", err)
	}

	// The diff must contain feature.txt (own work) but must NOT contain the
	// upstream-only commit local main is missing.
	if !strings.Contains(result.Diff, "feature.txt") {
		t.Errorf("expected feature.txt in diff, got:\n%s", result.Diff)
	}
	if strings.Contains(result.Diff, "a.txt") {
		t.Errorf("diff should NOT contain a.txt (upstream commit), got:\n%s", result.Diff)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "feature.txt" {
		t.Errorf("expected exactly [feature.txt], got %+v", result.Files)
	}

	// Staleness metadata must be populated.
	if result.BaseUpstream != "origin/main" {
		t.Errorf("BaseUpstream = %q, want %q", result.BaseUpstream, "origin/main")
	}
	if result.BaseBehindBy != 1 {
		t.Errorf("BaseBehindBy = %d, want 1", result.BaseBehindBy)
	}
}

func TestGetCompare_BehindByMultipleCommits(t *testing.T) {
	// Guards the rev-list --count parsing for behindBy > 1: every other test
	// exercises behindBy == 1, which can't catch a regression like an off-by-one
	// or a hard-coded "1".
	dir := setupStaleBaseRepo(t, 3)

	result, err := GetCompare(dir, "main")
	if err != nil {
		t.Fatalf("GetCompare: %v", err)
	}
	if result.BaseBehindBy != 3 {
		t.Errorf("BaseBehindBy = %d, want 3", result.BaseBehindBy)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if strings.Contains(result.Diff, name) {
			t.Errorf("diff should NOT contain %s (upstream commit), got:\n%s", name, result.Diff)
		}
	}
}

func TestGetCompare_KeepsLocalBaseWhenDiverged(t *testing.T) {
	// When local base is both ahead of and behind its upstream, the substitution
	// must not fire — the user selected the local ref and its local-only commits
	// belong on the base side of the diff, not in the feature changes.
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)

	// Local-only commit on main → local main is ahead of origin/main by 1.
	commitFile(t, dir, "local.txt", "local-only", "local main commit")

	// Upstream advances independently → now local main is also behind by 1.
	commitFile(t, remote, "ham.txt", "ham", "upstream advances")
	gitInDir(t, dir, "fetch", "origin")

	// Feature branch cut from local main (carries the local-only commit).
	gitInDir(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "mine", "feature commit")

	result, err := GetCompare(dir, "main")
	if err != nil {
		t.Fatalf("GetCompare: %v", err)
	}

	// Diff must contain the feature commit, must NOT contain the local-only
	// base commit (it belongs to the base), and must NOT contain the upstream
	// commit (it's not in either history from local main's perspective).
	if !strings.Contains(result.Diff, "feature.txt") {
		t.Errorf("expected feature.txt in diff, got:\n%s", result.Diff)
	}
	if strings.Contains(result.Diff, "local.txt") {
		t.Errorf("diff should NOT contain local.txt (base commit), got:\n%s", result.Diff)
	}
	if strings.Contains(result.Diff, "ham.txt") {
		t.Errorf("diff should NOT contain ham.txt (upstream commit), got:\n%s", result.Diff)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "feature.txt" {
		t.Errorf("expected exactly [feature.txt], got %+v", result.Files)
	}

	// Divergence (ahead > 0) must suppress the upstream substitution.
	if result.BaseUpstream != "" {
		t.Errorf("BaseUpstream = %q, want empty when local base is diverged", result.BaseUpstream)
	}
	if result.BaseBehindBy != 0 {
		t.Errorf("BaseBehindBy = %d, want 0 when local base is diverged", result.BaseBehindBy)
	}
}

func TestGetCompare_NoUpstreamLeavesFieldsEmpty(t *testing.T) {
	// A repo with no remote has no upstream; the new fields must stay zero-valued.
	dir := initTestRepo(t)
	commitFile(t, dir, "base.txt", "base", "base commit")

	gitInDir(t, dir, "checkout", "-B", "main")
	gitInDir(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "mine", "feature commit")

	result, err := GetCompare(dir, "main")
	if err != nil {
		t.Fatalf("GetCompare: %v", err)
	}
	if result.BaseUpstream != "" {
		t.Errorf("BaseUpstream = %q, want empty", result.BaseUpstream)
	}
	if result.BaseBehindBy != 0 {
		t.Errorf("BaseBehindBy = %d, want 0", result.BaseBehindBy)
	}
}

func TestGetCompare_UpstreamEqualToLocalLeavesFieldsEmpty(t *testing.T) {
	// Local base tracks an upstream that hasn't advanced — no substitution.
	remote := initTestRepo(t)
	commitFile(t, remote, "base.txt", "base", "base commit")

	dir := cloneTestRepo(t, remote)
	gitInDir(t, dir, "checkout", "-b", "feature")
	commitFile(t, dir, "feature.txt", "mine", "feature commit")

	result, err := GetCompare(dir, "main")
	if err != nil {
		t.Fatalf("GetCompare: %v", err)
	}
	if result.BaseUpstream != "" {
		t.Errorf("BaseUpstream = %q, want empty when upstream == local", result.BaseUpstream)
	}
	if result.BaseBehindBy != 0 {
		t.Errorf("BaseBehindBy = %d, want 0 when upstream == local", result.BaseBehindBy)
	}
}
