# Shell Sessions & Worktree Reuse Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Shell sessions skip worktree creation, existing worktrees are reused instead of duplicated, and agent types use a typed enum.

**Architecture:** Three changes woven through the provider, worktree, and session packages. The `AgentType` enum is introduced first (pure refactor, no behavior change). Then `FindWorktree` and `EnsureClone` are added to `worktree.Manager` (new capabilities, no callers yet). Finally `resolveSourceToCWD` is updated to wire everything together (behavioral changes).

**Tech Stack:** Go, git CLI, SQLite (schema unchanged)

---

### Task 1: Introduce AgentType enum

**Files:**
- Modify: `internal/agent/provider/provider.go`

**Step 1: Write tests for AgentType constants and updated signatures**

Modify `internal/agent/provider/provider_test.go`. Update the existing tests to use `AgentType` constants instead of string literals. All existing tests stay, just change the argument types:

```go
// In TestAllProviders, change the check loop:
for _, want := range []AgentType{AgentClaude, AgentCodex, AgentGemini, AgentShell} {
    if !ids[want] {
        t.Errorf("missing provider: %s", want)
    }
}

// Update ids map type:
ids := map[AgentType]bool{}
for _, p := range all {
    ids[p.ID] = true
}

// In TestIsValid:
if !IsValid(AgentClaude) {
    t.Error("claude should be valid")
}
if IsValid("opencode") {
    t.Error("opencode should not be valid")
}

// In each TestBuildCommand* test, replace string IDs:
// "claude" → AgentClaude
// "codex" → AgentCodex
// "gemini" → AgentGemini
// "shell" → AgentShell
// "unknown" stays as string (cast to AgentType("unknown"))
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/provider/ -v`
Expected: compilation errors — `AgentType` type doesn't exist yet.

**Step 3: Implement AgentType enum and update provider.go**

In `internal/agent/provider/provider.go`:

```go
// AgentType identifies a supported agent provider.
type AgentType string

const (
	AgentClaude AgentType = "claude"
	AgentCodex  AgentType = "codex"
	AgentGemini AgentType = "gemini"
	AgentShell  AgentType = "shell"
)
```

Update the `Provider` struct and all functions:

```go
type Provider struct {
	ID              AgentType
	Name            string
	CLI             string
	AutoApproveFlag string
	SupportsResume  bool
	ResumeArg       string
	ModelFlag       string
}

var providers = map[AgentType]*Provider{}

func register(p *Provider) {
	providers[p.ID] = p
}

func Get(id AgentType) (*Provider, error) {
	p, ok := providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", id)
	}
	return p, nil
}

func All() []*Provider {
	out := make([]*Provider, 0, len(providers))
	for _, id := range []AgentType{AgentClaude, AgentCodex, AgentGemini, AgentShell} {
		if p, ok := providers[id]; ok {
			out = append(out, p)
		}
	}
	return out
}

func IsValid(id AgentType) bool {
	_, ok := providers[id]
	return ok
}

func BuildCommand(id AgentType, opts BuildCommandOptions) (string, error) {
	// ... body unchanged
}
```

Update each provider registration file to use the constant:

`internal/agent/provider/claude.go`:
```go
register(&Provider{
    ID:              AgentClaude,
    // ... rest unchanged
})
```

`internal/agent/provider/codex.go`:
```go
register(&Provider{
    ID:              AgentCodex,
    // ... rest unchanged
})
```

`internal/agent/provider/gemini.go`:
```go
register(&Provider{
    ID:              AgentGemini,
    // ... rest unchanged
})
```

`internal/agent/provider/shell.go`:
```go
register(&Provider{
    ID:   AgentShell,
    Name: "Terminal",
    CLI:  "",
})
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/provider/ -v`
Expected: all PASS.

**Step 5: Update callers to use AgentType**

These files reference provider IDs as strings and must be updated:

`internal/agent/session/lifecycle.go`:
- `CreateOptions.AgentType` → `provider.AgentType`
- Line 68: `provider.IsValid(opts.AgentType)` — works as-is after IsValid signature change
- Line 74: `fmt.Sprintf("%s-%s", opts.AgentType, sessionID)` — works (AgentType is a string type)
- Line 93: `provider.BuildCommand(opts.AgentType, ...)` — works after signature change
- Line 129: `AgentType: opts.AgentType` → needs `string(opts.AgentType)` since `db.Session.AgentType` stays `string`
- Line 332 in EnsureSession: `provider.BuildCommand(session.AgentType, ...)` → needs `provider.AgentType(session.AgentType)` cast since DB field is string

`internal/agent/api/sessions.go`:
- Line 37-38: `if opts.AgentType == "" { opts.AgentType = "claude" }` → `if opts.AgentType == "" { opts.AgentType = provider.AgentClaude }`
- Import `provider` package (add to imports)

`cmd/argus/cli/session_create.go`:
- Line 81: Flag default `"claude"` stays as a string (cobra flags are strings). The `provider` constant is only used server-side.
- No change needed here — the string goes over HTTP to the API, which assigns it to `CreateOptions.AgentType`.

Since `CreateOptions.AgentType` becomes `provider.AgentType` but JSON unmarshaling produces a string, we need to keep `CreateOptions.AgentType` as `string` in the struct tag for JSON compatibility. Instead, add a helper:

Actually, the cleanest approach: keep `CreateOptions.AgentType` as `string` (it's a JSON-deserialized struct) and cast to `provider.AgentType` at the call sites. This avoids JSON unmarshaling issues.

Revised approach for `lifecycle.go`:
- `CreateOptions.AgentType` stays `string`
- Line 68: `provider.IsValid(provider.AgentType(opts.AgentType))`
- Line 93: `provider.BuildCommand(provider.AgentType(opts.AgentType), ...)`
- Line 129: `AgentType: opts.AgentType` — unchanged (both string)
- Line 332: `provider.BuildCommand(provider.AgentType(session.AgentType), ...)`

`internal/agent/api/sessions.go`:
- Line 38: `opts.AgentType = string(provider.AgentClaude)`

**Step 6: Run full test suite**

Run: `go test ./internal/... ./cmd/... -v`
Expected: all PASS.

**Step 7: Commit**

```bash
git add internal/agent/provider/ internal/agent/session/lifecycle.go internal/agent/api/sessions.go
git commit -m "refactor: introduce AgentType enum in provider package"
```

---

### Task 2: Add FindWorktree to worktree.Manager

**Files:**
- Modify: `internal/worktree/manager.go`
- Modify: `internal/worktree/manager_test.go`

**Step 1: Write tests for FindWorktree**

Add to `internal/worktree/manager_test.go`:

```go
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
	if found != wtPath {
		t.Errorf("FindWorktree = %q, want %q", found, wtPath)
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
	if found != wtPath2 {
		t.Errorf("FindWorktree = %q, want %q", found, wtPath2)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/worktree/ -run TestFindWorktree -v`
Expected: compilation error — `FindWorktree` doesn't exist.

**Step 3: Implement FindWorktree**

Add to `internal/worktree/manager.go`:

```go
// FindWorktree checks whether a git worktree already exists for the given
// branch. repoDir can be the main repo root or any existing worktree
// directory (git worktree list works from either).
// Returns the worktree path if found, empty string if not.
func (m *Manager) FindWorktree(repoDir, branch string) (string, error) {
	out, err := gitOutput(repoDir, "worktree", "list")
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}

	target := "[" + branch + "]"
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == target {
			return fields[0], nil
		}
	}
	return "", nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/worktree/ -run TestFindWorktree -v`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/worktree/manager.go internal/worktree/manager_test.go
git commit -m "feat: add FindWorktree to detect existing worktrees by branch"
```

---

### Task 3: Add EnsureClone to worktree.Manager

**Files:**
- Modify: `internal/worktree/manager.go`
- Modify: `internal/worktree/manager_test.go`

**Step 1: Write test for EnsureClone**

Add to `internal/worktree/manager_test.go`:

```go
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/worktree/ -run TestEnsureClone -v`
Expected: compilation error — `EnsureClone` doesn't exist.

**Step 3: Implement EnsureClone and refactor CreateForRemoteRepo**

Extract the clone-or-fetch logic from `CreateForRemoteRepo` into `EnsureClone`:

```go
// EnsureClone clones the remote repo if not already cloned, or fetches
// updates if it is. Returns the clone directory path.
func (m *Manager) EnsureClone(src *source.Source) (string, error) {
	cloneDir := filepath.Join(m.stateDir, "projects", src.ParentKey(), "gitrepo")

	_, statErr := os.Stat(cloneDir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return "", fmt.Errorf("stat clone dir: %w", statErr)
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0755); err != nil {
			return "", fmt.Errorf("create project dir: %w", err)
		}
		if err := runGit("", "clone", src.RemoteURL, cloneDir); err != nil {
			os.RemoveAll(cloneDir)
			return "", fmt.Errorf("clone repo: %w", err)
		}
	} else {
		defaultBranch, err := getDefaultBranch(cloneDir)
		if err != nil {
			return "", err
		}
		if err := runGit(cloneDir, "fetch", "origin"); err != nil {
			return "", fmt.Errorf("fetch: %w", err)
		}
		if err := runGit(cloneDir, "checkout", defaultBranch); err != nil {
			return "", fmt.Errorf("checkout default branch: %w", err)
		}
		if err := runGit(cloneDir, "reset", "--hard", "origin/"+defaultBranch); err != nil {
			return "", fmt.Errorf("reset to origin: %w", err)
		}
	}
	return cloneDir, nil
}
```

Refactor `CreateForRemoteRepo` to call `EnsureClone`:

```go
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string) (worktreePath, branch string, err error) {
	cloneDir, err := m.EnsureClone(src)
	if err != nil {
		return "", "", err
	}
	return m.createWorktree(cloneDir, src.ParentKey(), sessionName)
}
```

**Step 4: Run all worktree tests**

Run: `go test ./internal/worktree/ -v`
Expected: all PASS (including existing tests, verifying refactor didn't break anything).

**Step 5: Commit**

```bash
git add internal/worktree/manager.go internal/worktree/manager_test.go
git commit -m "feat: extract EnsureClone from CreateForRemoteRepo"
```

---

### Task 4: Worktree reuse in createWorktree

**Files:**
- Modify: `internal/worktree/manager.go`
- Modify: `internal/worktree/manager_test.go`

**Step 1: Write test for worktree reuse**

Add to `internal/worktree/manager_test.go`:

```go
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
	if wtPath2 != wtPath1 {
		t.Errorf("expected reused path %q, got %q", wtPath1, wtPath2)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/worktree/ -run TestCreateForLocalRepoReusesExistingWorktree -v`
Expected: FAIL — currently creates `jeev/my-feature-2` instead of reusing.

**Step 3: Update createWorktree to check for existing worktrees**

Modify `createWorktree` in `internal/worktree/manager.go`:

```go
func (m *Manager) createWorktree(repoDir, parentKey, sessionName string) (worktreePath, branch string, err error) {
	slug := slugify(sessionName)
	baseBranch := m.branchName(slug)

	// Check if a worktree already exists for this branch.
	existing, err := m.FindWorktree(repoDir, baseBranch)
	if err != nil {
		return "", "", err
	}
	if existing != "" {
		return existing, baseBranch, nil
	}

	branch, err = m.uniqueBranch(repoDir, baseBranch)
	if err != nil {
		return "", "", err
	}

	defaultBranch, err := getDefaultBranch(repoDir)
	if err != nil {
		return "", "", err
	}

	worktreePath = filepath.Join(m.stateDir, "projects", parentKey, "worktrees", worktreeDirName(branch))

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", "", fmt.Errorf("create worktrees dir: %w", err)
	}

	if err := runGit(repoDir, "worktree", "add", worktreePath, "-b", branch, defaultBranch); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w", err)
	}

	return worktreePath, branch, nil
}
```

**Step 4: Run all worktree tests**

Run: `go test ./internal/worktree/ -v`
Expected: all PASS. Note: `TestCreateForLocalRepoBranchConflict` will now REUSE the first worktree instead of creating a second one. Update it:

```go
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
	if wtPath2 != wtPath1 {
		t.Errorf("expected reused path %q, got %q", wtPath1, wtPath2)
	}
}
```

**Step 5: Run all worktree tests again**

Run: `go test ./internal/worktree/ -v`
Expected: all PASS.

**Step 6: Commit**

```bash
git add internal/worktree/manager.go internal/worktree/manager_test.go
git commit -m "feat: reuse existing worktrees instead of creating duplicates"
```

---

### Task 5: Shell sessions skip worktree creation

**Files:**
- Modify: `internal/agent/session/lifecycle.go`

**Step 1: Write test for shell session skipping worktree**

There are no existing unit tests for `resolveSourceToCWD` (it's tested indirectly through integration). Since it calls `findGitRoot` and `worktree.Manager` which shell out to git, a unit test would need the same `initGitRepo` helper. For now, verify via the full test suite and manual testing.

However, we can add a targeted test. Create `internal/agent/session/lifecycle_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/session/ -run TestResolveSourceToCWD -v`
Expected: compilation error — `resolveSourceToCWD` doesn't accept `agentType` parameter yet.

**Step 3: Update resolveSourceToCWD to accept agentType and skip worktree for shell**

Modify `internal/agent/session/lifecycle.go`:

Update the signature:
```go
func (m *Manager) resolveSourceToCWD(src, sessionName string, agentType provider.AgentType) (cwd string, worktreeBranch *string, cleanup func(), err error) {
```

Update the body — after source resolution, add the shell-specific paths:

```go
func (m *Manager) resolveSourceToCWD(src, sessionName string, agentType provider.AgentType) (cwd string, worktreeBranch *string, cleanup func(), err error) {
	noop := func() {}

	if src == "" {
		home, err := shared.ExpandPath("~")
		if err != nil {
			return "", nil, noop, fmt.Errorf("expand home directory: %w", err)
		}
		return home, nil, noop, nil
	}

	resolved, err := source.Resolve(src)
	if err != nil {
		return "", nil, noop, err
	}

	if resolved.IsRemote() {
		if agentType == provider.AgentShell {
			// Shell sessions clone but don't create a worktree.
			cloneDir, err := m.wt.EnsureClone(resolved)
			if err != nil {
				return "", nil, noop, err
			}
			return cloneDir, nil, noop, nil
		}
		wtPath, branch, err := m.wt.CreateForRemoteRepo(resolved, sessionName)
		if err != nil {
			return "", nil, noop, err
		}
		return wtPath, &branch, func() { m.wt.Cleanup(wtPath) }, nil
	}

	// Local path: check if it's inside a git repo.
	gitRoot, err := findGitRoot(resolved.LocalPath)
	if err != nil {
		// Not a git repo — use the path directly.
		return resolved.LocalPath, nil, noop, nil
	}

	if agentType == provider.AgentShell {
		// Shell sessions use the local path directly, no worktree.
		return resolved.LocalPath, nil, noop, nil
	}

	wtPath, branch, err := m.wt.CreateForLocalRepo(gitRoot, sessionName)
	if err != nil {
		return "", nil, noop, err
	}
	return wtPath, &branch, func() { m.wt.Cleanup(wtPath) }, nil
}
```

Update the call site in `Create` (line 79):
```go
cwd, worktreeBranch, cleanup, err := m.resolveSourceToCWD(opts.Source, opts.Name, provider.AgentType(opts.AgentType))
```

**Step 4: Run tests**

Run: `go test ./internal/agent/session/ -run TestResolveSourceToCWD -v`
Expected: all PASS.

**Step 5: Run full test suite**

Run: `go test ./internal/... ./cmd/... -v`
Expected: all PASS.

**Step 6: Commit**

```bash
git add internal/agent/session/lifecycle.go internal/agent/session/lifecycle_test.go
git commit -m "feat: shell sessions skip worktree creation"
```

---

### Task 6: Detect when source path is already a worktree

**Files:**
- Modify: `internal/agent/session/lifecycle.go`
- Modify: `internal/agent/session/lifecycle_test.go`

**Step 1: Write test for source path that is a worktree**

Add to `internal/agent/session/lifecycle_test.go`:

```go
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
	wtPath, branch, err := wt.CreateForLocalRepo(gitRoot, "existing work")
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/session/ -run TestResolveSourceToCWD_SourceIsExistingWorktree -v`
Expected: FAIL — currently creates a new worktree instead of reusing.

**Step 3: Add worktree detection to resolveSourceToCWD**

Add a helper to detect the branch of a worktree path and update the local-path handling. We need a new method on `worktree.Manager`:

Add to `internal/worktree/manager.go`:

```go
// FindWorktreeByPath checks if the given path is a known git worktree and
// returns its branch name. Returns empty string if the path is not a worktree
// or is the main working tree.
func (m *Manager) FindWorktreeByPath(dir string) (branch string, err error) {
	out, err := gitOutput(dir, "worktree", "list")
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == dir {
			// Extract branch name from "[branch]" format
			raw := fields[2]
			if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
				return raw[1 : len(raw)-1], nil
			}
		}
	}
	return "", nil
}
```

Add a test for `FindWorktreeByPath` in `internal/worktree/manager_test.go`:

```go
func TestFindWorktreeByPath(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	wtPath, branch, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
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
```

Now update `resolveSourceToCWD` in `lifecycle.go`. In the local-path + agent section, before calling `CreateForLocalRepo`, check if the resolved path IS a worktree:

```go
	if agentType == provider.AgentShell {
		return resolved.LocalPath, nil, noop, nil
	}

	// Check if the resolved path is already a worktree — reuse it.
	existingBranch, err := m.wt.FindWorktreeByPath(resolved.LocalPath)
	if err == nil && existingBranch != "" {
		return resolved.LocalPath, &existingBranch, noop, nil
	}

	wtPath, branch, err := m.wt.CreateForLocalRepo(gitRoot, sessionName)
```

Note: when reusing an existing worktree, cleanup is `noop` since we didn't create it.

**Step 4: Run tests**

Run: `go test ./internal/worktree/ -run TestFindWorktreeByPath -v`
Expected: all PASS.

Run: `go test ./internal/agent/session/ -run TestResolveSourceToCWD -v`
Expected: all PASS.

**Step 5: Run full test suite**

Run: `go test ./internal/... ./cmd/... -v`
Expected: all PASS.

**Step 6: Commit**

```bash
git add internal/worktree/manager.go internal/worktree/manager_test.go internal/agent/session/lifecycle.go internal/agent/session/lifecycle_test.go
git commit -m "feat: detect and reuse existing worktree when source path is a worktree"
```

---

### Task 7: Final verification

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: all PASS.

**Step 2: Build the binary**

Run: `go build ./cmd/argus/`
Expected: compiles with no errors.

**Step 3: Commit any remaining changes**

If there are any uncommitted files, commit them:

```bash
git status
```
