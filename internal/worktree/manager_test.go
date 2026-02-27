package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/bxnlabs/argus/internal/worktree"
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
	wtPath, branch, err := mgr.CreateForLocalRepo(gitRoot, "Fix Auth Bug")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	if branch != "jeev/fix-auth-bug" {
		t.Errorf("expected branch %q, got %q", "jeev/fix-auth-bug", branch)
	}

	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree path %q does not exist: %v", wtPath, err)
	}
}

func TestCreateForLocalRepoNoBranchPrefix(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{})
	_, branch, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
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

	wtPath1, branch1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}
	if branch1 != "jeev/my-feature" {
		t.Errorf("expected first branch %q, got %q", "jeev/my-feature", branch1)
	}

	// Second call with same name — should reuse the existing worktree
	wtPath2, branch2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("second CreateForLocalRepo: %v", err)
	}
	if branch2 != "jeev/my-feature" {
		t.Errorf("expected reused branch %q, got %q", "jeev/my-feature", branch2)
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
	cloneDir, err := mgr.EnsureClone(src)
	if err != nil {
		t.Fatalf("first EnsureClone: %v", err)
	}
	if _, err := os.Stat(cloneDir); err != nil {
		t.Fatalf("clone dir %q does not exist: %v", cloneDir, err)
	}

	// Second call — fetches, returns same dir
	cloneDir2, err := mgr.EnsureClone(src)
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
	wtPath, branch, err := mgr.CreateForRemoteRepo(src, "my feature")
	if err != nil {
		t.Fatalf("CreateForRemoteRepo: %v", err)
	}

	if branch != "jeev/my-feature" {
		t.Errorf("expected branch %q, got %q", "jeev/my-feature", branch)
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
	wtPath, branch, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
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
	wtPath1, _, err := mgr.CreateForLocalRepo(gitRoot, "first")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}
	wtPath2, branch2, err := mgr.CreateForLocalRepo(gitRoot, "second")
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
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// First creation
	wtPath1, branch1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}

	// Second creation with same name — should reuse, not create "-2"
	wtPath2, branch2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("second CreateForLocalRepo: %v", err)
	}

	if branch2 != branch1 {
		t.Errorf("expected reused branch %q, got %q", branch1, branch2)
	}
	if realPath(t, wtPath2) != realPath(t, wtPath1) {
		t.Errorf("expected reused path %q, got %q", realPath(t, wtPath1), realPath(t, wtPath2))
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
	_, _, err := mgr.CreateForRemoteRepo(src, "first session")
	if err != nil {
		t.Fatalf("first CreateForRemoteRepo: %v", err)
	}

	// Second call — repo already cloned; should fetch and create another worktree.
	_, branch2, err := mgr.CreateForRemoteRepo(src, "second session")
	if err != nil {
		t.Fatalf("second CreateForRemoteRepo: %v", err)
	}
	if branch2 != "jeev/second-session" {
		t.Errorf("expected branch %q, got %q", "jeev/second-session", branch2)
	}
}
