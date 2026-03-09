package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHookIntegrationSetupOrder(t *testing.T) {
	stateDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "order.txt")

	// Create profile hook that appends to marker file
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	mustWriteFile(t, filepath.Join(profileHookDir, "pre_create.sh"), []byte(
		"#!/bin/bash\necho profile >> "+markerFile+"\n"), 0755)

	// Create project hook that appends to marker file
	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	mustMkdirAll(t, projectHookDir)
	mustWriteFile(t, filepath.Join(projectHookDir, "pre_create.sh"), []byte(
		"#!/bin/bash\necho project >> "+markerFile+"\n"), 0755)

	hr := NewHookRunner(stateDir)
	env := HookEnv{SessionID: "test-1", WorkingDir: "/tmp", AgentType: "claude", Profile: "work"}

	// Run in setup order
	paths := hr.ResolveHookPaths("pre_create.sh", "work", "--test--repo")
	if len(paths) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(paths))
	}

	for _, p := range paths {
		if err := hr.RunHook(p, env); err != nil {
			t.Fatalf("hook failed: %v", err)
		}
	}

	// Verify order: profile first, then project
	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.TrimSpace(string(data))
	if lines != "profile\nproject" {
		t.Errorf("expected setup order 'profile\\nproject', got %q", lines)
	}
}

func TestHookIntegrationTeardownOrder(t *testing.T) {
	stateDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "teardown.txt")

	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	mustWriteFile(t, filepath.Join(profileHookDir, "pre_destroy.sh"), []byte(
		"#!/bin/bash\necho profile >> "+markerFile+"\n"), 0755)

	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	mustMkdirAll(t, projectHookDir)
	mustWriteFile(t, filepath.Join(projectHookDir, "pre_destroy.sh"), []byte(
		"#!/bin/bash\necho project >> "+markerFile+"\n"), 0755)

	hr := NewHookRunner(stateDir)
	env := HookEnv{SessionID: "test-1", WorkingDir: "/tmp", AgentType: "claude", Profile: "work"}

	// Run in teardown order (LIFO: project first, then profile)
	paths := hr.ResolveHookPathsTeardown("pre_destroy.sh", "work", "--test--repo")
	for _, p := range paths {
		if err := hr.RunHook(p, env); err != nil {
			t.Fatalf("hook failed: %v", err)
		}
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.TrimSpace(string(data))
	if lines != "project\nprofile" {
		t.Errorf("expected teardown order 'project\\nprofile', got %q", lines)
	}
}

func TestHookIntegrationEnvVars(t *testing.T) {
	stateDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "env.txt")

	hookDir := filepath.Join(stateDir, "profiles", "default", "hooks")
	mustMkdirAll(t, hookDir)
	mustWriteFile(t, filepath.Join(hookDir, "pre_create.sh"), []byte(
		"#!/bin/bash\n"+
			"echo \"SID=$ARGUS_SESSION_ID\" >> "+markerFile+"\n"+
			"echo \"WD=$ARGUS_WORKING_DIR\" >> "+markerFile+"\n"+
			"echo \"AT=$ARGUS_AGENT_TYPE\" >> "+markerFile+"\n"+
			"echo \"PROF=$ARGUS_PROFILE\" >> "+markerFile+"\n"), 0755)

	hr := NewHookRunner(stateDir)
	env := HookEnv{
		SessionID: "sess-abc", WorkingDir: t.TempDir(),
		AgentType: "claude", Profile: "default",
	}

	paths := hr.ResolveHookPaths("pre_create.sh", "default", "")
	for _, p := range paths {
		if err := hr.RunHook(p, env); err != nil {
			t.Fatalf("hook failed: %v", err)
		}
	}

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "SID=sess-abc") {
		t.Error("missing ARGUS_SESSION_ID")
	}
	if !strings.Contains(content, "WD="+env.WorkingDir) {
		t.Error("missing ARGUS_WORKING_DIR")
	}
	if !strings.Contains(content, "AT=claude") {
		t.Error("missing ARGUS_AGENT_TYPE")
	}
	if !strings.Contains(content, "PROF=default") {
		t.Error("missing ARGUS_PROFILE")
	}
}

func TestHookIntegrationTimeout(t *testing.T) {
	stateDir := t.TempDir()
	hookDir := filepath.Join(stateDir, "profiles", "slow", "hooks")
	mustMkdirAll(t, hookDir)
	// Use a loop instead of sleep so the process can be killed cleanly.
	mustWriteFile(t, filepath.Join(hookDir, "pre_create.sh"), []byte(
		"#!/bin/bash\nwhile true; do sleep 0.1; done"), 0755)

	hr := NewHookRunner(stateDir)
	hr.Timeout = 500 * time.Millisecond

	env := HookEnv{SessionID: "test", WorkingDir: "/tmp", AgentType: "shell"}
	paths := hr.ResolveHookPaths("pre_create.sh", "slow", "")
	if len(paths) != 1 {
		t.Fatalf("expected 1 hook, got %d", len(paths))
	}

	err := hr.RunHook(paths[0], env)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
}

func TestHookIntegrationNonZeroExit(t *testing.T) {
	stateDir := t.TempDir()
	hookDir := filepath.Join(stateDir, "profiles", "fail", "hooks")
	mustMkdirAll(t, hookDir)
	mustWriteFile(t, filepath.Join(hookDir, "pre_create.sh"), []byte(
		"#!/bin/bash\necho 'something went wrong' >&2\nexit 1"), 0755)

	hr := NewHookRunner(stateDir)
	env := HookEnv{SessionID: "test", WorkingDir: "/tmp", AgentType: "claude"}

	paths := hr.ResolveHookPaths("pre_create.sh", "fail", "")
	err := hr.RunHook(paths[0], env)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected 'failed' in error, got: %v", err)
	}
}

func TestHookIntegrationBestEffort(t *testing.T) {
	stateDir := t.TempDir()
	markerFile := filepath.Join(t.TempDir(), "best-effort.txt")

	// First hook (in teardown order = project) fails
	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	mustMkdirAll(t, projectHookDir)
	mustWriteFile(t, filepath.Join(projectHookDir, "pre_destroy.sh"), []byte(
		"#!/bin/bash\nexit 1"), 0755)

	// Second hook (in teardown order = profile) should still run
	profileHookDir := filepath.Join(stateDir, "profiles", "work", "hooks")
	mustMkdirAll(t, profileHookDir)
	mustWriteFile(t, filepath.Join(profileHookDir, "pre_destroy.sh"), []byte(
		"#!/bin/bash\necho survived >> "+markerFile+"\n"), 0755)

	hr := NewHookRunner(stateDir)
	env := HookEnv{SessionID: "test", WorkingDir: "/tmp", AgentType: "claude"}

	// Teardown order: project first (fails), then profile (should still run)
	paths := hr.ResolveHookPathsTeardown("pre_destroy.sh", "work", "--test--repo")
	hr.RunHooksBestEffort(paths, env)

	data, err := os.ReadFile(markerFile)
	if err != nil {
		t.Fatal("expected marker file to exist — profile hook should have run despite project hook failure")
	}
	if !strings.Contains(string(data), "survived") {
		t.Error("expected 'survived' marker from profile hook")
	}
}

func TestHookIntegrationProfileValidation(t *testing.T) {
	invalid := []string{"../escape", "has space", "has/slash", "", "a..b"}
	for _, name := range invalid {
		if err := ValidateProfileName(name); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}

	valid := []string{"default", "work", "my-profile", "test_123"}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Errorf("expected %q to be valid: %v", name, err)
		}
	}
}
