package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/source"
)

// initGitRepo creates a temporary git repo with an initial commit on "main".
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	return dir
}

func TestCreateForLocalRepo(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})
	wtPath, branch, created, err := mgr.CreateForLocalRepo(gitRoot, "Fix Auth Bug")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	if branch != "jeev/fix-auth-bug" {
		t.Errorf("expected branch %q, got %q", "jeev/fix-auth-bug", branch)
	}
	if !created {
		t.Error("expected created=true for new worktree")
	}

	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree path %q does not exist: %v", wtPath, err)
	}
}

func TestCreateForLocalRepoNoBranchPrefix(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{})
	_, branch, _, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}
	if branch != "my-feature" {
		t.Errorf("expected branch %q, got %q", "my-feature", branch)
	}
}

func TestCreateForLocalRepoBranchConflict(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	wtPath1, branch1, created1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}
	if branch1 != "jeev/my-feature" {
		t.Errorf("expected first branch %q, got %q", "jeev/my-feature", branch1)
	}
	if !created1 {
		t.Error("expected created=true for first worktree")
	}

	// Second call with same name — should reuse the existing worktree
	wtPath2, branch2, created2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("second CreateForLocalRepo: %v", err)
	}
	if branch2 != "jeev/my-feature" {
		t.Errorf("expected reused branch %q, got %q", "jeev/my-feature", branch2)
	}
	if created2 {
		t.Error("expected created=false for reused worktree")
	}
	if realPath(t, wtPath2) != realPath(t, wtPath1) {
		t.Errorf("expected reused path %q, got %q", realPath(t, wtPath1), realPath(t, wtPath2))
	}
}

func TestEnsureClone(t *testing.T) {
	remoteRepo := initGitRepo(t)
	stateDir := t.TempDir()

	src := &source.Source{
		RemoteURL: remoteRepo,
		Host:      "github.com",
		Org:       "testorg",
		Repo:      "testrepo",
	}

	mgr := worktree.NewManager(stateDir, &config.Config{})

	// First call — clones
	cloneDir, err := mgr.EnsureClone(src, false)
	if err != nil {
		t.Fatalf("first EnsureClone: %v", err)
	}
	if _, err := os.Stat(cloneDir); err != nil {
		t.Fatalf("clone dir %q does not exist: %v", cloneDir, err)
	}

	// Second call — fetches, returns same dir
	cloneDir2, err := mgr.EnsureClone(src, false)
	if err != nil {
		t.Fatalf("second EnsureClone: %v", err)
	}
	if cloneDir2 != cloneDir {
		t.Errorf("expected same dir %q, got %q", cloneDir, cloneDir2)
	}
}

func TestCreateForRemoteRepo(t *testing.T) {
	// Use a local git repo as the "remote" — git clone works with local paths.
	remoteRepo := initGitRepo(t)
	stateDir := t.TempDir()

	src := &source.Source{
		RemoteURL: remoteRepo, // local path works as git clone target
		Host:      "github.com",
		Org:       "testorg",
		Repo:      "testrepo",
	}

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})
	wtPath, branch, created, err := mgr.CreateForRemoteRepo(src, "my feature")
	if err != nil {
		t.Fatalf("CreateForRemoteRepo: %v", err)
	}

	if branch != "jeev/my-feature" {
		t.Errorf("expected branch %q, got %q", "jeev/my-feature", branch)
	}
	if !created {
		t.Error("expected created=true for new remote worktree")
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree path %q does not exist: %v", wtPath, err)
	}
	// Clone dir should be at stateDir/projects/<parentKey>/gitrepo
	cloneDir := filepath.Join(stateDir, "projects", src.ParentKey(), "gitrepo")
	if _, err := os.Stat(cloneDir); err != nil {
		t.Errorf("clone dir %q does not exist: %v", cloneDir, err)
	}
}

// realPath resolves symlinks so that paths from git worktree list
// (which resolves symlinks, e.g. /var -> /private/var on macOS)
// can be compared with paths returned by CreateForLocalRepo.
func realPath(t *testing.T, p string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", p, err)
	}
	return resolved
}

func TestFindWorktreeExists(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// Create a worktree first
	wtPath, branch, _, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// FindWorktree should find it
	found, err := mgr.FindWorktree(gitRoot, branch)
	if err != nil {
		t.Fatalf("FindWorktree: %v", err)
	}
	if found != realPath(t, wtPath) {
		t.Errorf("FindWorktree = %q, want %q", found, realPath(t, wtPath))
	}
}

func TestFindWorktreeNotExists(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{})

	found, err := mgr.FindWorktree(gitRoot, "nonexistent-branch")
	if err != nil {
		t.Fatalf("FindWorktree: %v", err)
	}
	if found != "" {
		t.Errorf("FindWorktree = %q, want empty", found)
	}
}

func TestFindWorktreeFromWorktreeDir(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// Create two worktrees
	wtPath1, _, _, err := mgr.CreateForLocalRepo(gitRoot, "first")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}
	wtPath2, branch2, _, err := mgr.CreateForLocalRepo(gitRoot, "second")
	if err != nil {
		t.Fatalf("second CreateForLocalRepo: %v", err)
	}

	// FindWorktree called from FIRST worktree should still find second
	found, err := mgr.FindWorktree(wtPath1, branch2)
	if err != nil {
		t.Fatalf("FindWorktree from worktree dir: %v", err)
	}
	if found != realPath(t, wtPath2) {
		t.Errorf("FindWorktree = %q, want %q", found, realPath(t, wtPath2))
	}
}

func TestCreateForLocalRepoReusesExistingWorktree(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
		want   string
	}{
		{"with prefix", "jeev", "jeev/my-feature"},
		{"no prefix", "", "my-feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gitRoot := initGitRepo(t)
			stateDir := t.TempDir()

			mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: tc.prefix}})

			// First creation
			wtPath1, branch1, created1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
			if err != nil {
				t.Fatalf("first CreateForLocalRepo: %v", err)
			}
			if branch1 != tc.want {
				t.Fatalf("expected branch %q, got %q", tc.want, branch1)
			}
			if !created1 {
				t.Error("expected created=true for first worktree")
			}

			// Second creation with same name — should reuse existing worktree.
			wtPath2, branch2, created2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
			if err != nil {
				t.Fatalf("second CreateForLocalRepo: %v", err)
			}

			if branch2 != branch1 {
				t.Errorf("expected reused branch %q, got %q", branch1, branch2)
			}
			if created2 {
				t.Error("expected created=false for reused worktree")
			}
			if realPath(t, wtPath2) != realPath(t, wtPath1) {
				t.Errorf("expected reused path %q, got %q", realPath(t, wtPath1), realPath(t, wtPath2))
			}
		})
	}
}

func TestFindWorktreeByPath(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	wtPath, branch, _, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	got, err := mgr.FindWorktreeByPath(wtPath)
	if err != nil {
		t.Fatalf("FindWorktreeByPath: %v", err)
	}
	if got != branch {
		t.Errorf("FindWorktreeByPath = %q, want %q", got, branch)
	}
}

func TestFindWorktreeByPathMainWorktree(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{})

	// Main repo root is not a "linked" worktree — should return empty
	got, err := mgr.FindWorktreeByPath(gitRoot)
	if err != nil {
		t.Fatalf("FindWorktreeByPath: %v", err)
	}
	if got != "" {
		t.Errorf("FindWorktreeByPath on main = %q, want empty", got)
	}
}

func TestCreateForLocalRepoReusesExistingBranch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
		want   string
	}{
		{"with prefix", "jeev", "jeev/my-feature"},
		{"no prefix", "", "my-feature"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gitRoot := initGitRepo(t)
			stateDir := t.TempDir()

			mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: tc.prefix}})

			// Create a worktree.
			wtPath1, branch1, created1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
			if err != nil {
				t.Fatalf("first CreateForLocalRepo: %v", err)
			}
			if branch1 != tc.want {
				t.Fatalf("expected branch %q, got %q", tc.want, branch1)
			}
			if !created1 {
				t.Fatal("expected created=true for first worktree")
			}

			// Remove the worktree but preserve the branch (mirrors session delete behavior).
			if err := mgr.RemoveWorktree(wtPath1, true); err != nil {
				t.Fatalf("RemoveWorktree: %v", err)
			}

			// Create again with the same name. The branch still exists but has no
			// worktree. Should reuse the branch, NOT create a suffixed variant.
			_, branch2, created2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
			if err != nil {
				t.Fatalf("second CreateForLocalRepo: %v", err)
			}
			if branch2 != tc.want {
				t.Errorf("expected reused branch %q, got %q", tc.want, branch2)
			}
			if !created2 {
				t.Error("expected created=true for newly created worktree (reusing existing branch)")
			}
		})
	}
}

func TestCreateForLocalRepoExistingBranchCheckedOutInMain(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// Create branch "jeev/my-feature" directly in the main worktree.
	if err := exec.Command("git", "-C", gitRoot, "checkout", "-b", "jeev/my-feature").Run(); err != nil {
		t.Fatalf("create branch in main worktree: %v", err)
	}

	// Attempting to create a session with the same name should fail with a
	// clear error because the branch is checked out in the main worktree.
	wtPath, _, _, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err == nil {
		t.Fatal("expected error when branch is checked out in main worktree, got nil")
	}
	if !strings.Contains(err.Error(), "currently checked out in the main working tree") {
		t.Errorf("expected main-worktree error, got: %v", err)
	}
	// No worktree directory should have been created.
	if wtPath != "" {
		t.Errorf("expected empty worktree path on error, got %q", wtPath)
	}
}

func TestDeleteBranch(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// Create a worktree (which creates the branch), then remove it (preserving the branch).
	wtPath, branch, _, err := mgr.CreateForLocalRepo(gitRoot, "delete-me")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}
	if err := mgr.RemoveWorktree(wtPath, true); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	// Branch should still exist after worktree removal.
	out, err := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatalf("branch %q should exist after worktree removal", branch)
	}

	// Delete the branch.
	if err := mgr.DeleteBranch(gitRoot, branch); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	// Branch should be gone.
	out, err = exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if err != nil {
		t.Fatalf("git branch --list after delete: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("branch %q should not exist after DeleteBranch", branch)
	}
}

func TestDeleteBranchNonexistent(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{})

	err := mgr.DeleteBranch(gitRoot, "nonexistent-branch")
	if err == nil {
		t.Fatal("expected error deleting nonexistent branch, got nil")
	}
}

func TestCreateForRemoteRepoAlreadyCloned(t *testing.T) {
	// Verify that calling CreateForRemoteRepo a second time (repo already cloned)
	// fetches and creates a new worktree without error.
	remoteRepo := initGitRepo(t)
	stateDir := t.TempDir()

	src := &source.Source{
		RemoteURL: remoteRepo,
		Host:      "github.com",
		Org:       "testorg",
		Repo:      "testrepo",
	}

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// First call — clones the repo.
	_, _, _, err := mgr.CreateForRemoteRepo(src, "first session")
	if err != nil {
		t.Fatalf("first CreateForRemoteRepo: %v", err)
	}

	// Second call — repo already cloned; should fetch and create another worktree.
	_, branch2, _, err := mgr.CreateForRemoteRepo(src, "second session")
	if err != nil {
		t.Fatalf("second CreateForRemoteRepo: %v", err)
	}
	if branch2 != "jeev/second-session" {
		t.Errorf("expected branch %q, got %q", "jeev/second-session", branch2)
	}
}
