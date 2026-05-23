package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/provider"
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
	cwd, branch, _, _, _, err := mgr.resolveSourceToCWD(gitRoot, "my shell", provider.ProviderShell, "")
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
	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "existing work", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Point an agent session at the worktree path — should reuse it
	cwd, gotBranch, _, _, _, err := mgr.resolveSourceToCWD(wtPath, "new session", provider.ProviderClaude, "")
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
// comparisons in IsManagedPath and FindWorktreeByPath fail.
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
	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "dirty-test", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID:               "sess-dirty-test",
		Name:             "dirty-test",
		TmuxName:         "claude-sess-dirty-test",
		WorkingDirectory: wtPath,
		ProviderType:        "claude",
		WorktreeBranch:   &branch,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Dirty the worktree
	mustWriteFile(t, filepath.Join(wtPath, "uncommitted.txt"), []byte("dirty"), 0644)

	// Delete without force should fail with dirty error
	_, err = mgr.Delete(sess.ID, false, false)
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

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "hook-dirty", "")
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
		ProviderType:        "claude",
		WorktreeBranch:   &branch,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Worktree is clean, pre_destroy hook will dirty it.
	// Delete without force should still succeed because the preflight
	// passed and we force-remove after hooks.
	if _, err := mgr.Delete(sess.ID, false, false); err != nil {
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

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "force-test", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID:               "sess-force-test",
		Name:             "force-test",
		TmuxName:         "claude-sess-force-test",
		WorkingDirectory: wtPath,
		ProviderType:        "claude",
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
	if _, err := mgr.Delete(sess.ID, true, false); err != nil {
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

func TestListProfiles(t *testing.T) {
	stateDir := t.TempDir()

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	// No profiles directory — should return empty slice, no error
	profiles, err := mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles (no dir): %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty profiles, got %v", profiles)
	}

	// Empty profiles directory — should return empty slice (not nil)
	mustMkdirAll(t, filepath.Join(stateDir, "profiles"))
	profiles, err = mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles (empty dir): %v", err)
	}
	if profiles == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(profiles) != 0 {
		t.Errorf("expected empty profiles, got %v", profiles)
	}

	// Create profiles directory with valid and invalid entries
	mustMkdirAll(t, filepath.Join(stateDir, "profiles", "work", "hooks"))
	mustMkdirAll(t, filepath.Join(stateDir, "profiles", "default", "hooks"))
	mustMkdirAll(t, filepath.Join(stateDir, "profiles", "has space", "hooks"))  // invalid name
	mustMkdirAll(t, filepath.Join(stateDir, "profiles", "..evil", "hooks"))     // invalid name
	mustMkdirAll(t, filepath.Join(stateDir, "profiles", "no-hooks-dir"))        // no hooks/ subdir
	mustWriteFile(t, filepath.Join(stateDir, "profiles", "a-file"), []byte("not a dir"), 0644)

	profiles, err = mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}

	// Should only contain valid profile names with hooks/ subdirs
	want := map[string]bool{"work": true, "default": true}
	got := map[string]bool{}
	for _, p := range profiles {
		got[p] = true
	}
	if len(got) != len(want) {
		t.Errorf("expected profiles %v, got %v", want, got)
	}
	for name := range want {
		if !got[name] {
			t.Errorf("missing expected profile %q in %v", name, profiles)
		}
	}
	// Verify invalid names are excluded
	for _, p := range profiles {
		if p == "has space" || p == "..evil" || p == "no-hooks-dir" || p == "a-file" {
			t.Errorf("unexpected profile %q should have been filtered", p)
		}
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
	cwd, branch, _, _, cleanup, err := mgr.resolveSourceToCWD(gitRoot, "my agent", provider.ProviderClaude, "")
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

func TestDeleteWithBranchDeletion(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "branch-del", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID:               "sess-branch-del",
		Name:             "branch-del",
		TmuxName:         "claude-sess-branch-del",
		WorkingDirectory: wtPath,
		ProviderType:     "claude",
		WorktreeBranch:   &branch,
		BranchCreated:    true,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.Delete(sess.ID, true, true)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !result.BranchDeleted {
		t.Error("expected BranchDeleted=true")
	}

	// Session should be gone
	got, err := database.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Fatal("session should be deleted")
	}

	// Worktree should be gone
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree should be removed")
	}

	// Branch should be gone
	out, gitErr := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if gitErr != nil {
		t.Fatalf("git branch --list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("branch %q should not exist after delete with deleteBranch=true", branch)
	}
}

func TestDeleteWithBranchDeletionSharedWorktree(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "shared-wt", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Create two sessions pointing at the same worktree
	sess1 := &db.Session{
		ID: "sess-shared-1", Name: "shared-1", TmuxName: "claude-sess-shared-1",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch, GitParentDir: &gitRoot,
	}
	sess2 := &db.Session{
		ID: "sess-shared-2", Name: "shared-2", TmuxName: "claude-sess-shared-2",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch, GitParentDir: &gitRoot,
	}
	if err := database.CreateSession(sess1); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(sess2); err != nil {
		t.Fatal(err)
	}

	// Delete first session with deleteBranch — should skip branch deletion
	result, err := mgr.Delete(sess1.ID, true, true)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if result.BranchDeleted {
		t.Error("expected BranchDeleted=false for shared worktree")
	}

	// Branch should still exist
	out, gitErr := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if gitErr != nil {
		t.Fatalf("git branch --list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Error("branch should still exist when other sessions share the worktree")
	}
}

func TestDeleteWithBranchDeletionNilGitParentDir(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "no-parent", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID: "sess-no-parent", Name: "no-parent", TmuxName: "claude-sess-no-parent",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch,
		BranchCreated:  true,
		GitParentDir:   nil, // intentionally nil
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Should fail because GitParentDir is nil
	_, err = mgr.Delete(sess.ID, true, true)
	if err == nil {
		t.Fatal("expected error for nil GitParentDir, got nil")
	}

	// Session should still exist (preflight failed, no side effects)
	got, dbErr := database.GetSession(sess.ID)
	if dbErr != nil {
		t.Fatalf("GetSession: %v", dbErr)
	}
	if got == nil {
		t.Fatal("session should still exist after preflight failure")
	}
}

func TestDeleteWithBranchDeletionFailureIsBestEffort(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "best-effort", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Point GitParentDir at a bogus path so git branch -D will fail.
	bogusDir := filepath.Join(t.TempDir(), "nonexistent")
	sess := &db.Session{
		ID:               "sess-best-effort",
		Name:             "best-effort",
		TmuxName:         "claude-sess-best-effort",
		WorkingDirectory: wtPath,
		ProviderType:     "claude",
		WorktreeBranch:   &branch,
		BranchCreated:    true,
		GitParentDir:     &bogusDir,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Delete with branch — git branch -D will fail because bogusDir
	// is not a git repo. Session should still be deleted.
	result, err := mgr.Delete(sess.ID, true, true)
	if err != nil {
		t.Fatalf("Delete should succeed even when branch deletion fails: %v", err)
	}
	if result.BranchDeleted {
		t.Error("expected BranchDeleted=false when git branch -D fails")
	}

	// Session should be gone (not blocked by branch failure)
	got, err := database.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Fatal("session should be deleted even when branch deletion fails")
	}
}

func TestChangeProfile(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	stateDir := t.TempDir()
	workDir := t.TempDir()

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	preDestroyMarker := filepath.Join(t.TempDir(), "old-pre-destroy.txt")
	preCreateMarker := filepath.Join(t.TempDir(), "new-pre-create.txt")

	// Old profile: pre_destroy writes a marker on teardown.
	oldHooks := filepath.Join(stateDir, "profiles", "old", "hooks")
	if err := os.MkdirAll(oldHooks, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldHooks, "pre_destroy.sh"),
		[]byte("#!/bin/bash\necho torn >> "+preDestroyMarker+"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	// New profile: pre_create writes a marker on setup.
	newHooks := filepath.Join(stateDir, "profiles", "new", "hooks")
	if err := os.MkdirAll(newHooks, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newHooks, "pre_create.sh"),
		[]byte("#!/bin/bash\necho setup >> "+preCreateMarker+"\n"), 0755); err != nil {
		t.Fatal(err)
	}

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	oldProfile := "old"
	tmuxName := fmt.Sprintf("shell-test-%d", time.Now().UnixNano())
	if err := database.CreateSession(&db.Session{
		ID: "sess-cp", Name: "cp", TmuxName: tmuxName,
		WorkingDirectory: workDir, ProviderType: "shell", Profile: &oldProfile,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { KillSession(tmuxName) })

	newProfile := "new"
	updated, err := mgr.ChangeProfile("sess-cp", &newProfile)
	if err != nil {
		t.Fatalf("ChangeProfile: %v", err)
	}

	if updated.Profile == nil || *updated.Profile != "new" {
		t.Errorf("profile = %v, want %q", updated.Profile, "new")
	}
	if _, err := os.Stat(preDestroyMarker); err != nil {
		t.Error("expected old profile pre_destroy to run")
	}
	if _, err := os.Stat(preCreateMarker); err != nil {
		t.Error("expected new profile pre_create to run")
	}
	if !HasSession(tmuxName) {
		t.Error("expected tmux session to be respawned")
	}
}

func TestChangeProfileInvalidName(t *testing.T) {
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	bad := "../evil"
	if _, err := mgr.ChangeProfile("any-id", &bad); err == nil {
		t.Fatal("expected ErrInvalidInput for bad profile name")
	} else if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestChangeProfileMissingWorkingDirBlocksBeforeSideEffects(t *testing.T) {
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Both profiles must exist so validation passes and we reach the
	// working-directory preflight.
	for _, name := range []string{"old", "new"} {
		if err := os.MkdirAll(filepath.Join(stateDir, "profiles", name, "hooks"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	oldProfile := "old"
	missingDir := filepath.Join(t.TempDir(), "gone")
	if err := database.CreateSession(&db.Session{
		ID: "sess-missing-cwd", Name: "missing", TmuxName: "shell-sess-missing-cwd",
		WorkingDirectory: missingDir, ProviderType: "shell", Profile: &oldProfile,
	}); err != nil {
		t.Fatal(err)
	}

	newProfile := "new"
	if _, err := mgr.ChangeProfile("sess-missing-cwd", &newProfile); err == nil {
		t.Fatal("expected error when working directory is missing, got nil")
	}

	// Profile must be unchanged — preflight failed before persisting.
	got, err := database.GetSession("sess-missing-cwd")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Profile == nil || *got.Profile != "old" {
		t.Errorf("profile should be unchanged after preflight failure, got %v", got.Profile)
	}
}

func TestChangeProfileRespawnFailureLeavesProfileUnchanged(t *testing.T) {
	stateDir := t.TempDir()
	workDir := t.TempDir() // exists, so the preflight passes
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	for _, name := range []string{"old", "new"} {
		if err := os.MkdirAll(filepath.Join(stateDir, "profiles", name, "hooks"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	// An invalid provider type makes respawnTmux fail at BuildCommand, after
	// the preflight has passed. Since the profile is persisted only after a
	// successful respawn, the stored profile must remain unchanged.
	oldProfile := "old"
	if err := database.CreateSession(&db.Session{
		ID: "sess-respawn-fail", Name: "rf", TmuxName: "shell-sess-respawn-fail",
		WorkingDirectory: workDir, ProviderType: "bogus-provider", Profile: &oldProfile,
	}); err != nil {
		t.Fatal(err)
	}

	newProfile := "new"
	if _, err := mgr.ChangeProfile("sess-respawn-fail", &newProfile); err == nil {
		t.Fatal("expected respawn failure for invalid provider, got nil")
	}

	// Profile must be unchanged — it is never persisted when respawn fails.
	got, err := database.GetSession("sess-respawn-fail")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.Profile == nil || *got.Profile != "old" {
		t.Errorf("profile should be unchanged at %q, got %v", "old", got.Profile)
	}
}

func TestDeleteSharedWorktreeBranchOwnershipSurvives(t *testing.T) {
	// Regression test: session A creates branch (BranchCreated=true),
	// session B reuses the worktree (BranchCreated=false). Deleting A
	// first should NOT lose the ownership signal — when B is deleted
	// last with deleteBranch=true, the branch should still be cleaned up.
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "ownership-test", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Session A created the branch
	sessA := &db.Session{
		ID: "sess-owner-a", Name: "owner-a", TmuxName: "claude-sess-owner-a",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch, BranchCreated: true, GitParentDir: &gitRoot,
	}
	// Session B reused the worktree
	sessB := &db.Session{
		ID: "sess-owner-b", Name: "owner-b", TmuxName: "claude-sess-owner-b",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch, BranchCreated: false, GitParentDir: &gitRoot,
	}
	if err := database.CreateSession(sessA); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(sessB); err != nil {
		t.Fatal(err)
	}

	// Delete A first (not last session, branch/worktree preserved)
	result, err := mgr.Delete(sessA.ID, true, true)
	if err != nil {
		t.Fatalf("Delete A: %v", err)
	}
	if result.BranchDeleted {
		t.Error("A: expected BranchDeleted=false (B still shares worktree)")
	}

	// Branch should still exist
	out, gitErr := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if gitErr != nil {
		t.Fatalf("git branch --list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatal("branch should still exist after deleting A")
	}

	// Delete B last with deleteBranch=true — should succeed despite
	// B having BranchCreated=false, because A's ownership is visible.
	result, err = mgr.Delete(sessB.ID, true, true)
	if err != nil {
		t.Fatalf("Delete B: %v", err)
	}
	if !result.BranchDeleted {
		t.Error("B: expected BranchDeleted=true (A created the branch, ownership should survive)")
	}

	// Branch should be gone
	out, gitErr = exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if gitErr != nil {
		t.Fatalf("git branch --list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("branch %q should not exist after last session deleted with deleteBranch=true", branch)
	}
}
