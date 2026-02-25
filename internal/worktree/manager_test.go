package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
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

	mgr := worktree.NewManager(stateDir, &config.Config{BranchPrefix: "jeev"})
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

	mgr := worktree.NewManager(stateDir, &config.Config{BranchPrefix: "jeev"})

	_, branch1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}
	if branch1 != "jeev/my-feature" {
		t.Errorf("expected first branch %q, got %q", "jeev/my-feature", branch1)
	}

	_, branch2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("second CreateForLocalRepo: %v", err)
	}
	if branch2 != "jeev/my-feature-2" {
		t.Errorf("expected second branch %q, got %q", "jeev/my-feature-2", branch2)
	}
}
