package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/provider"
	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/worktree"
)

func initTestGitRepo(t *testing.T) string {
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

func TestResolveSourceToCWD_ShellSkipsWorktree(t *testing.T) {
	gitRoot := initTestGitRepo(t)
	stateDir := t.TempDir()

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt)

	// Shell session with a local git repo as source — should NOT create worktree
	cwd, branch, _, err := mgr.resolveSourceToCWD(gitRoot, "my shell", provider.AgentShell)
	if err != nil {
		t.Fatalf("resolveSourceToCWD: %v", err)
	}
	if branch != nil {
		t.Errorf("expected nil worktree branch for shell, got %q", *branch)
	}
	if cwd != gitRoot {
		t.Errorf("expected cwd %q, got %q", gitRoot, cwd)
	}
}

func TestResolveSourceToCWD_SourceIsExistingWorktree(t *testing.T) {
	gitRoot := initTestGitRepo(t)
	stateDir := t.TempDir()

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt)

	// Create a worktree externally
	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "existing work")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Point an agent session at the worktree path — should reuse it
	cwd, gotBranch, _, err := mgr.resolveSourceToCWD(wtPath, "new session", provider.AgentClaude)
	if err != nil {
		t.Fatalf("resolveSourceToCWD: %v", err)
	}
	if cwd != wtPath {
		t.Errorf("expected cwd %q (existing worktree), got %q", wtPath, cwd)
	}
	if gotBranch == nil || *gotBranch != branch {
		var got string
		if gotBranch != nil {
			got = *gotBranch
		}
		t.Errorf("expected branch %q, got %q", branch, got)
	}
}

func TestResolveSourceToCWD_AgentCreatesWorktree(t *testing.T) {
	gitRoot := initTestGitRepo(t)
	stateDir := t.TempDir()

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt)

	// Agent session with a local git repo — SHOULD create worktree
	cwd, branch, cleanup, err := mgr.resolveSourceToCWD(gitRoot, "my agent", provider.AgentClaude)
	if err != nil {
		t.Fatalf("resolveSourceToCWD: %v", err)
	}
	defer cleanup()

	if branch == nil {
		t.Fatal("expected non-nil worktree branch for agent")
	}
	if cwd == gitRoot {
		t.Error("expected cwd to be worktree path, not git root")
	}
}
