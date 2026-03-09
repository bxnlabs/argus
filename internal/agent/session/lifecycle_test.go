package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/provider"
	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/source"
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
	mgr := NewManager(database, wt, stateDir)

	// Shell session with a local git repo as source — should NOT create worktree
	cwd, branch, _, _, err := mgr.resolveSourceToCWD(gitRoot, "my shell", provider.AgentShell)
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
	mgr := NewManager(database, wt, stateDir)

	// Create a worktree externally
	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "existing work")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Point an agent session at the worktree path — should reuse it
	cwd, gotBranch, _, _, err := mgr.resolveSourceToCWD(wtPath, "new session", provider.AgentClaude)
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

// resolveSymlinks resolves symlinks in a path. On macOS t.TempDir() returns
// /var/... which is a symlink to /private/var/...; without resolving, path
// comparisons in IsManaged (which uses EvalSymlinks) fail.
func resolveSymlinks(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", path, err)
	}
	return resolved
}

func TestDeleteDirtyWorktreeBlocksBeforeSideEffects(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	// Create a worktree-backed session via the manager's worktree infrastructure
	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "dirty-test")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID:               "sess-dirty-test",
		Name:             "dirty-test",
		TmuxName:         "claude-sess-dirty-test",
		WorkingDirectory: wtPath,
		AgentType:        "claude",
		WorktreeBranch:   &branch,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Dirty the worktree
	mustWriteFile(t, filepath.Join(wtPath, "uncommitted.txt"), []byte("dirty"), 0644)

	// Delete without force should fail with dirty error
	err = mgr.Delete(sess.ID, false)
	if err == nil {
		t.Fatal("expected dirty worktree error, got nil")
	}

	// Session should still exist in DB (no side effects)
	got, err := database.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession after failed delete: %v", err)
	}
	if got == nil {
		t.Fatal("session should still exist after dirty-worktree rejection")
	}

	// Worktree should still exist on disk
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree should still exist: %v", statErr)
	}
}

func TestDeletePreDestroyHookDirtyingWorktreeStillSucceeds(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "hook-dirty")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Derive the project key so we can create a project-level hook
	projectKey := source.ParentKeyFromPath(gitRoot)

	// Create a pre_destroy hook that dirties the worktree
	hookDir := filepath.Join(stateDir, "projects", projectKey, "hooks")
	mustMkdirAll(t, hookDir)
	hookScript := fmt.Sprintf("#!/bin/bash\necho dirty > %s/hook-created.txt\n", wtPath)
	mustWriteFile(t, filepath.Join(hookDir, HookPreDestroy), []byte(hookScript), 0755)

	sess := &db.Session{
		ID:               "sess-hook-dirty",
		Name:             "hook-dirty",
		TmuxName:         "claude-sess-hook-dirty",
		WorkingDirectory: wtPath,
		AgentType:        "claude",
		WorktreeBranch:   &branch,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Worktree is clean, pre_destroy hook will dirty it.
	// Delete without force should still succeed because the preflight
	// passed and we force-remove after hooks.
	if err := mgr.Delete(sess.ID, false); err != nil {
		t.Fatalf("Delete should succeed despite hook dirtying worktree: %v", err)
	}

	// Session should be gone from DB
	got, err := database.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Fatal("session should be deleted")
	}

	// Worktree should be removed
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree should be removed, got stat err: %v", statErr)
	}
}

func TestDeleteForceBypassesDirtyCheck(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "force-test")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID:               "sess-force-test",
		Name:             "force-test",
		TmuxName:         "claude-sess-force-test",
		WorkingDirectory: wtPath,
		AgentType:        "claude",
		WorktreeBranch:   &branch,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Dirty the worktree
	if err := os.WriteFile(filepath.Join(wtPath, "uncommitted.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	// Force delete should succeed despite dirty worktree
	if err := mgr.Delete(sess.ID, true); err != nil {
		t.Fatalf("force Delete should succeed: %v", err)
	}

	// Session should be gone
	got, err := database.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Fatal("session should be deleted")
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
	mgr := NewManager(database, wt, stateDir)

	// Agent session with a local git repo — SHOULD create worktree
	cwd, branch, _, cleanup, err := mgr.resolveSourceToCWD(gitRoot, "my agent", provider.AgentClaude)
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
