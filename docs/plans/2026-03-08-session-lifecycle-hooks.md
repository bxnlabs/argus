# Session Lifecycle Hooks Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add profile-based lifecycle hooks that bootstrap session environments via shell scripts at key points in the session lifecycle.

**Architecture:** New `hooks.go` in `internal/agent/session/` handles hook discovery, validation, and execution. The session `Manager` gains a `stateDir` field for resolving hook paths. Profile names are persisted in the DB for teardown resolution. Init scripts are extended to source `post_create` hooks.

**Tech Stack:** Go, SQLite, tmux, bash shell scripts

**Design doc:** `docs/plans/2026-03-08-session-lifecycle-hooks-design.md`

---

### Task 1: Add `profile` column to database

**Files:**
- Modify: `internal/agent/db/models.go:9-23`
- Modify: `internal/agent/db/schema.go:3-27`
- Modify: `internal/agent/db/migrations.go:4-15`
- Modify: `internal/agent/db/sessions.go:10-12,14-29,31-48`
- Modify: `internal/agent/db/db_test.go`

**Step 1: Write the failing test**

Add to `internal/agent/db/db_test.go`:

```go
func TestSessionProfile(t *testing.T) {
	db := testDB(t)

	profile := "work"
	if err := db.CreateSession(&Session{
		ID: "s1", Name: "profiled", TmuxName: "claude-s1",
		WorkingDirectory: "~", AgentType: "claude", Profile: &profile,
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if got.Profile == nil || *got.Profile != "work" {
		t.Errorf("expected profile %q, got %v", "work", got.Profile)
	}
}

func TestSessionProfileNull(t *testing.T) {
	db := testDB(t)

	if err := db.CreateSession(&Session{
		ID: "s1", Name: "legacy", TmuxName: "claude-s1",
		WorkingDirectory: "~", AgentType: "claude",
	}); err != nil {
		t.Fatal(err)
	}

	got, _ := db.GetSession("s1")
	if got.Profile != nil {
		t.Errorf("expected nil profile, got %v", got.Profile)
	}
}

func TestMigrationAddProfile(t *testing.T) {
	db := testDB(t)

	// Running migrations a second time should not error (idempotent)
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/db/ -run "TestSessionProfile$|TestSessionProfileNull|TestMigrationAddProfile" -v`
Expected: FAIL — `Session` struct has no `Profile` field

**Step 3: Implement the database changes**

In `internal/agent/db/models.go`, add `Profile` field to `Session`:

```go
type Session struct {
	// ... existing fields ...
	GitParentDir     *string `json:"git_parent_dir"`
	Profile          *string `json:"profile"`
}
```

In `internal/agent/db/schema.go`, add `profile` column to the CREATE TABLE:

```sql
  git_parent_dir TEXT,
  profile TEXT
```

In `internal/agent/db/migrations.go`, add the migration:

```go
func (d *DB) RunMigrations() error {
	// ... existing migrations ...
	if err := d.migrate("add_git_parent_dir", func() error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN git_parent_dir TEXT`)
		return err
	}); err != nil {
		return err
	}
	return d.migrate("add_profile", func() error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN profile TEXT`)
		return err
	})
}
```

In `internal/agent/db/sessions.go`, update `sessionColumns`, `scanSession`, and `CreateSession`:

```go
const sessionColumns = `id, name, tmux_name, created_at, updated_at,
	working_directory, provider_session_id, model, system_prompt,
	agent_type, auto_approve, worktree_branch, git_parent_dir, profile`
```

In `scanSession`, add `&s.Profile` to the Scan call after `&s.GitParentDir`.

In `CreateSession`, add `profile` to the INSERT column list and `s.Profile` to the values.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/db/ -v`
Expected: ALL PASS

**Step 5: Commit**

```
git add internal/agent/db/
git commit -m "feat: add profile column to sessions table [BXN-44]"
```

---

### Task 2: Create `hooks.go` — hook discovery, validation, and subprocess execution

**Files:**
- Create: `internal/agent/session/hooks.go`
- Create: `internal/agent/session/hooks_test.go`

**Step 1: Write the failing tests**

Create `internal/agent/session/hooks_test.go`:

```go
package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/db"
)

func TestValidateProfileName(t *testing.T) {
	valid := []string{"default", "work", "my-profile", "test_123", "A"}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", name, err)
		}
	}

	invalid := []string{"", "../evil", "has/slash", "has space", "..", "a..b"}
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
	os.MkdirAll(profileHookDir, 0755)
	hookPath := filepath.Join(profileHookDir, "pre_create.sh")
	os.WriteFile(hookPath, []byte("#!/bin/bash\necho hello"), 0755)

	// Create project hook
	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	os.MkdirAll(projectHookDir, 0755)
	projHookPath := filepath.Join(projectHookDir, "pre_create.sh")
	os.WriteFile(projHookPath, []byte("#!/bin/bash\necho world"), 0755)

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
	os.MkdirAll(profileHookDir, 0755)
	os.WriteFile(filepath.Join(profileHookDir, "pre_destroy.sh"), []byte("#!/bin/bash"), 0755)

	projectHookDir := filepath.Join(stateDir, "projects", "--test--repo", "hooks")
	os.MkdirAll(projectHookDir, 0755)
	os.WriteFile(filepath.Join(projectHookDir, "pre_destroy.sh"), []byte("#!/bin/bash"), 0755)

	hr := NewHookRunner(stateDir)

	// Teardown order: project first, then profile (LIFO)
	paths := hr.ResolveHookPathsTeardown("pre_destroy.sh", "work", "--test--repo")
	if len(paths) != 2 {
		t.Fatalf("expected 2 hooks, got %d", len(paths))
	}
	if !contains(paths[0], "projects") {
		t.Errorf("first teardown hook should be project, got %q", paths[0])
	}
	if !contains(paths[1], "profiles") {
		t.Errorf("second teardown hook should be profile, got %q", paths[1])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestResolveHookPathsDefaultProfile(t *testing.T) {
	stateDir := t.TempDir()

	defaultHookDir := filepath.Join(stateDir, "profiles", "default", "hooks")
	os.MkdirAll(defaultHookDir, 0755)
	os.WriteFile(filepath.Join(defaultHookDir, "pre_create.sh"), []byte("#!/bin/bash"), 0755)

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
	os.MkdirAll(profileHookDir, 0755)
	os.WriteFile(filepath.Join(profileHookDir, "pre_create.sh"), []byte("#!/bin/bash"), 0644) // not executable

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
	os.MkdirAll(profileHookDir, 0755)
	os.WriteFile(filepath.Join(profileHookDir, "post_create.sh"), []byte("export FOO=bar"), 0644)

	hr := NewHookRunner(stateDir)

	// post_create is sourced, so executable bit is not required
	paths := hr.ResolvePostCreateHookPaths("work", "--test--repo")
	if len(paths) != 1 {
		t.Fatalf("expected 1 hook (post_create doesn't need exec bit), got %d", len(paths))
	}
}

func TestRunHookTimeout(t *testing.T) {
	stateDir := t.TempDir()
	hookDir := filepath.Join(stateDir, "profiles", "slow", "hooks")
	os.MkdirAll(hookDir, 0755)
	hookPath := filepath.Join(hookDir, "pre_create.sh")
	os.WriteFile(hookPath, []byte("#!/bin/bash\nsleep 60"), 0755)

	hr := NewHookRunner(stateDir)
	hr.Timeout = 100 * time.Millisecond // very short for test

	env := HookEnv{SessionID: "test", WorkingDir: "/tmp", AgentType: "shell"}
	err := hr.RunHook(hookPath, env)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/session/ -run "TestValidateProfileName|TestProjectKey|TestResolveHook|TestRunHook" -v`
Expected: FAIL — functions don't exist yet

**Step 3: Implement `hooks.go`**

Create `internal/agent/session/hooks.go`:

```go
package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/source"
)

var profileNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateProfileName checks that a profile name is safe for use as a
// directory name. Empty names are rejected; use "" to mean "no profile".
func ValidateProfileName(name string) error {
	if name == "" || !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must match [a-zA-Z0-9_-]+", name)
	}
	return nil
}

// ProjectKeyForSession derives the project key from session state.
// Worktree-backed sessions use git_parent_dir; others use working_directory.
func ProjectKeyForSession(sess *db.Session) string {
	if sess.GitParentDir != nil {
		return source.ParentKeyFromPath(*sess.GitParentDir)
	}
	return source.ParentKeyFromPath(sess.WorkingDirectory)
}

// HookEnv holds context variables passed to subprocess hooks.
type HookEnv struct {
	SessionID    string
	WorkingDir   string
	AgentType    string
	WorktreePath string
	Profile      string
}

// HookRunner resolves and executes lifecycle hooks.
type HookRunner struct {
	stateDir string
	Timeout  time.Duration
}

// NewHookRunner creates a HookRunner rooted at the given state directory.
func NewHookRunner(stateDir string) *HookRunner {
	return &HookRunner{stateDir: stateDir, Timeout: 30 * time.Second}
}

// ResolveHookPaths returns hook script paths in setup order (profile then project).
// Only existing, executable files are returned. Missing hooks are silently skipped.
func (hr *HookRunner) ResolveHookPaths(hookName, profileName, projectKey string) []string {
	return hr.resolveHooks(hookName, profileName, projectKey, false, true)
}

// ResolveHookPathsTeardown returns hook script paths in teardown order
// (project first, then profile — LIFO).
func (hr *HookRunner) ResolveHookPathsTeardown(hookName, profileName, projectKey string) []string {
	return hr.resolveHooks(hookName, profileName, projectKey, true, true)
}

// ResolvePostCreateHookPaths returns post_create hook paths in setup order.
// Unlike other hooks, post_create does not require the executable bit.
func (hr *HookRunner) ResolvePostCreateHookPaths(profileName, projectKey string) []string {
	return hr.resolveHooks("post_create.sh", profileName, projectKey, false, false)
}

func (hr *HookRunner) resolveHooks(hookName, profileName, projectKey string, teardown, requireExec bool) []string {
	// Resolve profile name: explicit or default
	if profileName == "" {
		profileName = "default"
	}

	profilePath := filepath.Join(hr.stateDir, "profiles", profileName, "hooks", hookName)
	projectPath := filepath.Join(hr.stateDir, "projects", projectKey, "hooks", hookName)

	var setupOrder []string
	if hr.hookExists(profilePath, requireExec) {
		setupOrder = append(setupOrder, profilePath)
	}
	if hr.hookExists(projectPath, requireExec) {
		setupOrder = append(setupOrder, projectPath)
	}

	if teardown {
		// Reverse for LIFO
		for i, j := 0, len(setupOrder)-1; i < j; i, j = i+1, j-1 {
			setupOrder[i], setupOrder[j] = setupOrder[j], setupOrder[i]
		}
	}
	return setupOrder
}

func (hr *HookRunner) hookExists(path string, requireExec bool) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if requireExec && info.Mode()&0111 == 0 {
		return false
	}
	return true
}

// RunHook executes a subprocess hook with timeout and environment variables.
// Returns an error if the hook exits non-zero or times out.
func (hr *HookRunner) RunHook(hookPath string, env HookEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), hr.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, hookPath)
	cmd.Dir = env.WorkingDir
	cmd.Env = append(os.Environ(),
		"ARGUS_SESSION_ID="+env.SessionID,
		"ARGUS_WORKING_DIR="+env.WorkingDir,
		"ARGUS_AGENT_TYPE="+env.AgentType,
		"ARGUS_WORKTREE_PATH="+env.WorktreePath,
		"ARGUS_PROFILE="+env.Profile,
	)

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("hook %s timed out after %s", filepath.Base(hookPath), hr.Timeout)
	}
	if err != nil {
		return fmt.Errorf("hook %s failed: %w\n%s", filepath.Base(hookPath), err, string(out))
	}
	return nil
}

// RunHooksBestEffort runs hooks and logs errors but does not return them.
func (hr *HookRunner) RunHooksBestEffort(paths []string, env HookEnv) {
	for _, p := range paths {
		if err := hr.RunHook(p, env); err != nil {
			log.Printf("hook %s: %v", filepath.Base(p), err)
		}
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/session/ -run "TestValidateProfileName|TestProjectKey|TestResolveHook|TestRunHook" -v`
Expected: ALL PASS

**Step 5: Commit**

```
git add internal/agent/session/hooks.go internal/agent/session/hooks_test.go
git commit -m "feat: add hook discovery, validation, and execution [BXN-44]"
```

---

### Task 3: Inject `stateDir` into session `Manager`

**Files:**
- Modify: `internal/agent/session/lifecycle.go:27-33,52-54`
- Modify: `internal/agent/session/lifecycle_test.go`
- Modify: `internal/agent/setup.go:49-51`

**Step 1: Update `Manager` struct and `NewManager`**

In `internal/agent/session/lifecycle.go`:

- Add `stateDir string` field to the `Manager` struct
- Add `hookRunner *HookRunner` field
- Update `NewManager` signature to accept `stateDir string`
- Initialize `hookRunner` in `NewManager`

```go
type Manager struct {
	db        *db.DB
	wt        *worktree.Manager
	stateDir  string
	hooks     *HookRunner
	mu        sync.Mutex
	sessLks   map[string]*sync.Mutex
}

func NewManager(database *db.DB, wt *worktree.Manager, stateDir string) *Manager {
	return &Manager{db: database, wt: wt, stateDir: stateDir, hooks: NewHookRunner(stateDir)}
}
```

**Step 2: Update all callers**

In `internal/agent/setup.go:51`, pass `stateDir`:

```go
mgr := session.NewManager(database, wtMgr, stateDir)
```

In `internal/agent/session/lifecycle_test.go`, update all `NewManager` calls to pass a `stateDir`:

```go
mgr := NewManager(database, wt, stateDir)
```

**Step 3: Add `Profile` field to `CreateOptions`**

In `internal/agent/session/lifecycle.go`:

```go
type CreateOptions struct {
	Name            string  `json:"name"`
	AgentType       string  `json:"agent_type"`
	Source          string  `json:"source"`
	Model           *string `json:"model,omitempty"`
	SystemPrompt    *string `json:"system_prompt,omitempty"`
	AutoApprove     bool    `json:"auto_approve"`
	ResumeSessionID string  `json:"resume_session_id,omitempty"`
	Profile         *string `json:"profile,omitempty"`
}
```

**Step 4: Run all tests to verify nothing broke**

Run: `go test ./internal/agent/... -v`
Expected: ALL PASS

**Step 5: Commit**

```
git add internal/agent/session/lifecycle.go internal/agent/session/lifecycle_test.go internal/agent/setup.go
git commit -m "refactor: inject stateDir into session Manager [BXN-44]"
```

---

### Task 4: Extend init scripts to source `post_create` hooks

**Files:**
- Modify: `internal/agent/session/initscript.go`

**Step 1: Write the failing test**

Add to a new `internal/agent/session/initscript_test.go`:

```go
package session

import (
	"strings"
	"testing"
)

func TestGenerateInitScriptWithHooks(t *testing.T) {
	hooks := []string{"/tmp/profiles/work/hooks/post_create.sh", "/tmp/projects/repo/hooks/post_create.sh"}
	script := GenerateInitScript("claude --resume abc", hooks)

	// Should contain guarded source commands
	if !strings.Contains(script, `source "/tmp/profiles/work/hooks/post_create.sh"`) {
		t.Error("expected profile hook source command")
	}
	if !strings.Contains(script, `source "/tmp/projects/repo/hooks/post_create.sh"`) {
		t.Error("expected project hook source command")
	}
	if !strings.Contains(script, "set +e") {
		t.Error("expected set +e guard")
	}
	if !strings.Contains(script, "|| true") {
		t.Error("expected || true guard")
	}
	// Agent command should still be exec'd
	if !strings.Contains(script, "exec claude --resume abc") {
		t.Error("expected exec agent command")
	}
}

func TestGenerateInitScriptWithoutHooks(t *testing.T) {
	script := GenerateInitScript("claude", nil)
	// Should not contain hook sourcing section
	if strings.Contains(script, "set +e") {
		t.Error("no hooks means no set +e block")
	}
	if !strings.Contains(script, "exec claude") {
		t.Error("expected exec agent command")
	}
}

func TestGenerateShellInitScript(t *testing.T) {
	hooks := []string{"/tmp/profiles/default/hooks/post_create.sh"}
	script := GenerateShellInitScript(hooks)

	if !strings.Contains(script, `source "/tmp/profiles/default/hooks/post_create.sh"`) {
		t.Error("expected hook source command")
	}
	if !strings.Contains(script, "exec $SHELL -l") {
		t.Error("expected exec $SHELL -l")
	}
	// Should NOT contain the agent banner
	if strings.Contains(script, "Argus") {
		t.Error("shell init script should not have agent banner")
	}
}

func TestGenerateShellInitScriptNoHooks(t *testing.T) {
	script := GenerateShellInitScript(nil)
	if script != "" {
		t.Errorf("expected empty string when no hooks, got %q", script)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/session/ -run "TestGenerate" -v`
Expected: FAIL — wrong function signatures

**Step 3: Implement init script changes**

Modify `GenerateInitScript` in `internal/agent/session/initscript.go` to accept `hookPaths []string` as a second parameter. Insert the guarded source block before the `exec` line.

Add `GenerateShellInitScript(hookPaths []string) string` that returns `""` when `hookPaths` is empty, otherwise generates a minimal wrapper.

Add a helper `generateHookSourceBlock(hookPaths []string) string` that builds:

```bash
# Source post_create hooks (errors are non-fatal)
set +e
source "/path/to/hook1" 2>&1 || true
source "/path/to/hook2" 2>&1 || true
set -e
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/session/ -run "TestGenerate" -v`
Expected: ALL PASS

**Step 5: Commit**

```
git add internal/agent/session/initscript.go internal/agent/session/initscript_test.go
git commit -m "feat: extend init scripts to source post_create hooks [BXN-44]"
```

---

### Task 5: Wire hooks into `Create` lifecycle

**Files:**
- Modify: `internal/agent/session/lifecycle.go:68-174`

**Step 1: Implement the create lifecycle changes**

In `Manager.Create()`, after validating inputs:

1. **Validate profile name** — if `opts.Profile` is non-nil, call `ValidateProfileName`. Check profile dir exists; error if explicit and missing.
2. **Resolve profile** — if nil, resolve to `"default"` if dir exists, else `""`. Store resolved profile name.
3. **Run `pre_create` hooks** — resolve and run blocking hooks. Abort on failure.
4. After `resolveSourceToCWD` and rollback defer setup:
5. **Run `on_create_worktree` hooks** — only if worktree was newly created. Abort on failure (triggers rollback).
6. **Generate init script with hooks** — pass `post_create` hook paths to `GenerateInitScript` / `GenerateShellInitScript`.
7. **Persist profile** — set `session.Profile` in the DB record.

Key changes to `Create()`:

```go
func (m *Manager) Create(opts CreateOptions) (*db.Session, error) {
	if !provider.IsValid(provider.AgentType(opts.AgentType)) {
		return nil, fmt.Errorf("%w: invalid agent type: %s", ErrInvalidInput, opts.AgentType)
	}

	// Resolve profile
	resolvedProfile, err := m.resolveProfile(opts.Profile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	sessionID := shared.GenerateID("sess")
	tmuxName := fmt.Sprintf("%s-%s", opts.AgentType, sessionID)

	// Resolve source → working directory
	cwd, worktreeBranch, cleanup, err := m.resolveSourceToCWD(...)
	// ... existing code ...

	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	// Derive project key for hook resolution
	var gitParentDir *string
	// ... existing git parent dir resolution ...
	projectKey := ""
	if gitParentDir != nil {
		projectKey = source.ParentKeyFromPath(*gitParentDir)
	} else {
		projectKey = source.ParentKeyFromPath(cwd)
	}

	// Run pre_create hooks (blocking)
	hookEnv := HookEnv{
		SessionID: sessionID, WorkingDir: cwd,
		AgentType: opts.AgentType, Profile: resolvedProfile,
	}
	preCreatePaths := m.hooks.ResolveHookPaths("pre_create.sh", resolvedProfile, projectKey)
	for _, p := range preCreatePaths {
		if err := m.hooks.RunHook(p, hookEnv); err != nil {
			return nil, fmt.Errorf("pre_create hook: %w", err)
		}
	}

	// Run on_create_worktree hooks if worktree was newly created
	if worktreeBranch != nil {
		hookEnv.WorktreePath = cwd
		wtPaths := m.hooks.ResolveHookPaths("on_create_worktree.sh", resolvedProfile, projectKey)
		for _, p := range wtPaths {
			if err := m.hooks.RunHook(p, hookEnv); err != nil {
				return nil, fmt.Errorf("on_create_worktree hook: %w", err)
			}
		}
	}

	// Build agent command and init script with post_create hooks
	postCreatePaths := m.hooks.ResolvePostCreateHookPaths(resolvedProfile, projectKey)
	// ... use postCreatePaths in GenerateInitScript/GenerateShellInitScript ...

	// Persist profile in session record
	var profilePtr *string
	if resolvedProfile != "" {
		profilePtr = &resolvedProfile
	}
	session := &db.Session{
		// ... existing fields ...
		Profile: profilePtr,
	}

	// ... rest of create ...
}
```

Add helper `resolveProfile`:

```go
func (m *Manager) resolveProfile(profileOpt *string) (string, error) {
	if profileOpt != nil {
		name := *profileOpt
		if err := ValidateProfileName(name); err != nil {
			return "", err
		}
		dir := filepath.Join(m.stateDir, "profiles", name, "hooks")
		if _, err := os.Stat(dir); err != nil {
			return "", fmt.Errorf("profile %q not found", name)
		}
		return name, nil
	}
	// Check for default profile
	dir := filepath.Join(m.stateDir, "profiles", "default", "hooks")
	if _, err := os.Stat(dir); err == nil {
		return "default", nil
	}
	return "", nil
}
```

**Step 2: Handle shell sessions with hooks**

For shell sessions (`agentCmd == ""`), use `GenerateShellInitScript(postCreatePaths)`. If it returns `""` (no hooks), keep `tmuxCmd` empty (current behavior). Otherwise, write it and use `bash <scriptPath>`.

**Step 3: Run all tests**

Run: `go test ./internal/agent/... -v`
Expected: ALL PASS

**Step 4: Commit**

```
git add internal/agent/session/lifecycle.go
git commit -m "feat: wire pre_create, on_create_worktree, and post_create hooks into Create [BXN-44]"
```

---

### Task 6: Wire hooks into `Delete` lifecycle

**Files:**
- Modify: `internal/agent/session/lifecycle.go:251-291`

**Step 1: Reorder `Delete` and add hooks**

Rewrite `Delete` to follow the documented sequence:

```
1. Look up session
2. pre_destroy (project first, then profile — LIFO, best-effort)
3. Kill tmux
4. Remove worktree
5. Delete DB record
6. post_destroy (project first, then profile — LIFO, best-effort)
```

```go
func (m *Manager) Delete(id string, force bool) error {
	l := m.sessionLock(id)
	l.Lock()
	defer l.Unlock()

	session, err := m.db.GetSession(id)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("%w: %s", db.ErrNotFound, id)
	}

	projectKey := ProjectKeyForSession(session)
	profileName := ptrStr(session.Profile)

	hookEnv := HookEnv{
		SessionID: session.ID, WorkingDir: session.WorkingDirectory,
		AgentType: session.AgentType, Profile: profileName,
	}

	// pre_destroy: LIFO (project first, then profile)
	preDestroyPaths := m.hooks.ResolveHookPathsTeardown("pre_destroy.sh", profileName, projectKey)
	m.hooks.RunHooksBestEffort(preDestroyPaths, hookEnv)

	// Kill tmux
	if HasSession(session.TmuxName) {
		KillSession(session.TmuxName)
	}

	// Remove worktree (existing logic)
	if session.WorktreeBranch != nil && m.wt.IsManaged(session.WorkingDirectory) {
		others, err := m.db.CountSessionsByWorkingDir(id, session.WorkingDirectory)
		if err != nil {
			return fmt.Errorf("check shared worktree: %w", err)
		}
		if others == 0 {
			if _, statErr := os.Stat(session.WorkingDirectory); os.IsNotExist(statErr) {
				// already removed externally
			} else if err := m.wt.RemoveWorktree(session.WorkingDirectory, force); err != nil {
				return err
			}
		}
	}

	// Delete DB record
	if err := m.db.DeleteSession(id); err != nil {
		return err
	}

	// post_destroy: LIFO (project first, then profile)
	postDestroyPaths := m.hooks.ResolveHookPathsTeardown("post_destroy.sh", profileName, projectKey)
	m.hooks.RunHooksBestEffort(postDestroyPaths, hookEnv)

	return nil
}
```

**Step 2: Run all tests**

Run: `go test ./internal/agent/... -v`
Expected: ALL PASS

**Step 3: Commit**

```
git add internal/agent/session/lifecycle.go
git commit -m "feat: wire pre_destroy and post_destroy hooks into Delete [BXN-44]"
```

---

### Task 7: Wire hooks into `EnsureSession`

**Files:**
- Modify: `internal/agent/session/lifecycle.go:338-418`

**Step 1: Update `EnsureSession` to use profile and hooks**

When `EnsureSession` recreates a killed session, it should regenerate the init script with `post_create` hooks — but only if the session has a non-NULL profile.

Key changes:
- Read `session.Profile` from DB
- If profile is non-NULL, resolve `post_create` hooks and pass to init script generation
- For shell sessions with hooks, generate the shell init wrapper

**Step 2: Run all tests**

Run: `go test ./internal/agent/... -v`
Expected: ALL PASS

**Step 3: Commit**

```
git add internal/agent/session/lifecycle.go
git commit -m "feat: regenerate init script with hooks on EnsureSession recreate [BXN-44]"
```

---

### Task 8: Add `--profile` flag to CLI

**Files:**
- Modify: `cmd/argus/cli/session_create.go`

**Step 1: Add the flag and pass to API**

In `newCreateCmd()`:

```go
var (
	provider string
	src      string
	yolo     bool
	profile  string
)
```

Add to `reqBody`:

```go
if profile != "" {
	reqBody["profile"] = profile
}
```

Add flag:

```go
cmd.Flags().StringVar(&profile, "profile", "", "Profile name for lifecycle hooks")
```

**Step 2: Run existing CLI tests**

Run: `go test ./cmd/argus/cli/ -v`
Expected: ALL PASS

**Step 3: Commit**

```
git add cmd/argus/cli/session_create.go
git commit -m "feat: add --profile flag to argus new [BXN-44]"
```

---

### Task 9: End-to-end integration test

**Files:**
- Create: `internal/agent/session/hooks_integration_test.go`

**Step 1: Write integration test**

Create a test that:
1. Sets up a `stateDir` with profile and project hooks (real scripts that write marker files)
2. Creates a `HookRunner` and exercises the full resolution + execution flow
3. Verifies hooks ran in correct order by checking marker file contents
4. Verifies teardown order is reversed
5. Verifies timeout behavior
6. Verifies profile validation rejects bad names

**Step 2: Run the integration test**

Run: `go test ./internal/agent/session/ -run "TestHookIntegration" -v`
Expected: ALL PASS

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: ALL PASS (ignore `embed.go` pattern error for `web/dist` — that's a build artifact)

**Step 4: Commit**

```
git add internal/agent/session/hooks_integration_test.go
git commit -m "test: add hook lifecycle integration tests [BXN-44]"
```
