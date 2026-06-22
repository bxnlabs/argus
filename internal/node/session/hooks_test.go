package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
)

// mustMkdirAll is a test helper that creates directories or fails the test.
func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

// mustWriteFile is a test helper that writes a file or fails the test.
func mustWriteFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func TestValidateProfileName(t *testing.T) {
	valid := []string{"default", "work", "my-profile", "test_123", "0abc"}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
	}

	// Uppercase rejected (docker project names must be lowercase); leading
	// -/_ rejected by the anchored first char class.
	invalid := []string{"", "../evil", "has/slash", "has space", "..", "a..b", "A", "ClientA", "-lead", "_lead"}
	for _, name := range invalid {
		if err := ValidateProfileName(name); err == nil {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestProjectKeyForSession(t *testing.T) {
	// Session with git_parent_dir uses that for project key
	parentDir := "/Users/jeevb/repos/argus"
	sess := &db.Session{
		WorkingDirectory: "/some/worktree/path",
		GitParentDir:     &parentDir,
	}
	got := ProjectKeyForSession(sess)
	if got != "--Users--jeevb--repos--argus" {
		t.Errorf("expected --Users--jeevb--repos--argus, got %q", got)
	}

	// Session without git_parent_dir uses working directory
	sess2 := &db.Session{
		WorkingDirectory: "/Users/jeevb/repos/argus",
	}
	got2 := ProjectKeyForSession(sess2)
	if got2 != "--Users--jeevb--repos--argus" {
		t.Errorf("expected --Users--jeevb--repos--argus, got %q", got2)
	}
}

func TestResolveHookPaths(t *testing.T) {
	stateDir := t.TempDir()

	// Create profile hook
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	hookPath := filepath.Join(profileHookDir, "pre_create.sh")
	mustWriteFile(t, hookPath, []byte("#!/bin/bash\necho hello"), 0755)

	// Create project hook
	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	mustMkdirAll(t, projectHookDir)
	projHookPath := filepath.Join(projectHookDir, "pre_create.sh")
	mustWriteFile(t, projHookPath, []byte("#!/bin/bash\necho world"), 0755)

	hr := NewHookRunner(stateDir)

	// Setup order: profile first, then project
	paths := hr.ResolveHookPaths("pre_create.sh", "work", "--test--repo")
	if len(paths) != 2 {
		t.Fatalf("expected 2 hooks, got %d: %v", len(paths), paths)
	}
	if paths[0] != hookPath {
		t.Errorf("first hook = %q, want %q", paths[0], hookPath)
	}
	if paths[1] != projHookPath {
		t.Errorf("second hook = %q, want %q", paths[1], projHookPath)
	}
}

func TestResolveHookPathsTeardownOrder(t *testing.T) {
	stateDir := t.TempDir()

	// Create both hooks
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	mustWriteFile(t, filepath.Join(profileHookDir, "pre_destroy.sh"), []byte("#!/bin/bash"), 0755)

	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	mustMkdirAll(t, projectHookDir)
	mustWriteFile(t, filepath.Join(projectHookDir, "pre_destroy.sh"), []byte("#!/bin/bash"), 0755)

	hr := NewHookRunner(stateDir)

	// Teardown order: project first, then profile (LIFO)
	paths := hr.ResolveHookPathsTeardown("pre_destroy.sh", "work", "--test--repo")
	if len(paths) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(paths))
	}
	if !strings.Contains(paths[0], "projects") {
		t.Errorf("first teardown hook should be project, got %q", paths[0])
	}
	if !strings.Contains(paths[1], "profiles") {
		t.Errorf("second teardown hook should be profile, got %q", paths[1])
	}
}

func TestResolveHookPathsDefaultProfile(t *testing.T) {
	stateDir := t.TempDir()

	defaultHookDir := filepath.Join(stateDir, "profiles", "default", "hooks")
	mustMkdirAll(t, defaultHookDir)
	mustWriteFile(t, filepath.Join(defaultHookDir, "pre_create.sh"), []byte("#!/bin/bash"), 0755)

	hr := NewHookRunner(stateDir)

	// Empty profile name should resolve to default
	paths := hr.ResolveHookPaths("pre_create.sh", "", "--test--repo")
	if len(paths) != 1 {
		t.Fatalf("expected 1 hook (default profile), got %d", len(paths))
	}
}

func TestResolveHookPathsSkipsMissingAndNonExecutable(t *testing.T) {
	stateDir := t.TempDir()

	// Create a non-executable hook
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	mustWriteFile(t, filepath.Join(profileHookDir, "pre_create.sh"), []byte("#!/bin/bash"), 0644) // not executable

	hr := NewHookRunner(stateDir)

	paths := hr.ResolveHookPaths("pre_create.sh", "work", "--test--repo")
	if len(paths) != 0 {
		t.Errorf("expected 0 hooks (non-executable should be skipped), got %d", len(paths))
	}
}

func TestResolvePostCreateHookPathsSkipsExecutableCheck(t *testing.T) {
	stateDir := t.TempDir()

	// Create a non-executable post_create hook (sourced, so OK)
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	mustWriteFile(t, filepath.Join(profileHookDir, "post_create.sh"), []byte("export FOO=bar"), 0644)

	hr := NewHookRunner(stateDir)

	// post_create is sourced, so executable bit is not required
	paths := hr.ResolvePostCreateHookPaths("work", "--test--repo")
	if len(paths) != 1 {
		t.Fatalf("expected 1 hook (post_create doesn't need exec bit), got %d", len(paths))
	}
}

func TestResolveProfileHookPath(t *testing.T) {
	stateDir := t.TempDir()

	// Profile-level pre_create (executable).
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	profileHook := filepath.Join(profileHookDir, "pre_create.sh")
	mustWriteFile(t, profileHook, []byte("#!/bin/bash"), 0755)

	// Project-level pre_create — must be excluded.
	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	mustMkdirAll(t, projectHookDir)
	mustWriteFile(t, filepath.Join(projectHookDir, "pre_create.sh"), []byte("#!/bin/bash"), 0755)

	hr := NewHookRunner(stateDir)

	got := hr.ResolveProfileHookPath("pre_create.sh", "work", true)
	if got != profileHook {
		t.Errorf("got %q, want profile-level %q (project must be excluded)", got, profileHook)
	}
}

func TestResolveProfileHookPathDefaultFallback(t *testing.T) {
	stateDir := t.TempDir()
	defaultHookDir := filepath.Join(stateDir, "profiles", "default", "hooks")
	mustMkdirAll(t, defaultHookDir)
	want := filepath.Join(defaultHookDir, "pre_destroy.sh")
	mustWriteFile(t, want, []byte("#!/bin/bash"), 0755)

	hr := NewHookRunner(stateDir)
	got := hr.ResolveProfileHookPath("pre_destroy.sh", "", true)
	if got != want {
		t.Errorf("empty profile should fall back to default: got %q, want %q", got, want)
	}
}

func TestResolveProfileHookPathMissing(t *testing.T) {
	stateDir := t.TempDir()
	hr := NewHookRunner(stateDir)
	if got := hr.ResolveProfileHookPath("pre_create.sh", "ghost", true); got != "" {
		t.Errorf("expected empty for missing hook, got %q", got)
	}
}

func TestResolveProfileHookPathRequiresExec(t *testing.T) {
	stateDir := t.TempDir()
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	// Not executable.
	mustWriteFile(t, filepath.Join(profileHookDir, "pre_create.sh"), []byte("#!/bin/bash"), 0644)

	hr := NewHookRunner(stateDir)
	if got := hr.ResolveProfileHookPath("pre_create.sh", "work", true); got != "" {
		t.Errorf("expected empty for non-executable hook when requireExec=true, got %q", got)
	}
}

func TestRunHookTimeout(t *testing.T) {
	stateDir := t.TempDir()
	hookDir := filepath.Join(stateDir, "profiles", "slow", "hooks")
	mustMkdirAll(t, hookDir)
	hookPath := filepath.Join(hookDir, "pre_create.sh")
	mustWriteFile(t, hookPath, []byte("#!/bin/bash\nsleep 60"), 0755)

	hr := NewHookRunner(stateDir)
	hr.Timeout = 100 * time.Millisecond // very short for test

	env := HookEnv{SessionID: "test", WorkingDir: "/tmp", ProviderType: "shell"}
	err := hr.RunHook(hookPath, env)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
