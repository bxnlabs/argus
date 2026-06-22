# Dockerized Profiles Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a profile run its session's agent inside a Docker container defined by a `docker-compose` file, so each client's environment (tailnet, toolchain, sandbox) lives in a compose stack and "switch client" becomes "pick that profile."

**Architecture:** A profile is "dockerized" when its directory holds a `docker-compose.yml`. The agent runs via `docker compose exec` into a long-lived, shared-per-profile stack (lazy up, manual down) from the host tmux pane — so tmux, terminal streaming, and provider-session-ID capture are untouched. The host home and state dir are mounted at identical paths and the agent runs as the host UID/GID, so host path == container path everywhere. Only `post_create` (the sourced hook) runs in-container; the executed hooks stay on the host.

**Tech Stack:** Go (backend, `internal/node/...`), Cobra (CLI), Docker Compose CLI, React + TanStack Query + Vitest (web).

---

## Background for the implementer

Read these before starting — they are the touch points:

- `internal/node/session/lifecycle.go` — `Manager`, `Create`, `respawnTmux`, `ChangeProfile`, `resolveProfile`, `ListProfiles`. This is where sessions are built and where the tmux command string is assembled (the block at `Create` ~line 180 and `respawnTmux` ~line 760 is nearly identical — we extract and extend it).
- `internal/node/session/hooks.go` — hook resolution. `post_create` is *sourced*; the others are *executed*. A profile is valid only if `<stateDir>/profiles/<name>/hooks` exists (`resolveProfile`, `ListProfiles`). **A dockerized profile therefore needs both a `hooks/` dir (may be empty) and a `docker-compose.yml`.**
- `internal/node/session/initscript.go` — generates the bash init script tmux runs. `generateHookSourceBlock`, `shellQuote`, `GenerateInitScript`, `WriteInitScript` are reused.
- `internal/node/provider/provider.go` — `BuildCommand` returns `""` for the `shell` provider; `GetSessionIDPattern` returns the capture regex (empty for `shell`).
- `internal/shared/paths.go` — `StateDir()`. Home is `os.UserHomeDir()`.
- `internal/node/api/router.go`, `internal/node/api/sessions.go`, `internal/node/api/helpers.go` — HTTP routes and `respondJSON` / `respondError` / `respondInternalError`.
- `cmd/argus/cli/cli.go`, `cmd/argus/main.go`, `cmd/argus/cli/client.go`, `cmd/argus/cli/session_list.go` — CLI command registration, the `apiClient`, and the `tabwriter` table pattern.
- `web/src/data/sessions/queries.ts`, `web/src/components/{ChangeProfileDialog,NewSessionDialog}/index.tsx` — the `/profiles` consumer and the two profile selectors.

**Conventions:** TDD (failing test first). Conventional-commit messages. Go tests: `go test ./path/...`. Web tests: `pnpm -C web test`. Run `go build ./...` after backend tasks.

---

## File structure

**New files:**
- `internal/node/docker/docker.go` — pure helpers: compose-file detection, env assembly, project name, path visibility.
- `internal/node/docker/docker_test.go`
- `internal/node/docker/exec.go` — pure `docker compose exec` command-string builder.
- `internal/node/docker/exec_test.go`
- `internal/node/docker/compose.go` — `CLICompose`, the real Compose CLI wrapper.
- `internal/node/docker/compose_integration_test.go` — opt-in, docker-gated.
- `internal/node/session/docker.go` — `Manager` docker plumbing: `composeRunner` interface, `compose` field accessors, `profileLock`, `ensureStackUp`, `buildDockerTmuxCmd`, `ProfileInfo`, `ProfileUp`/`ProfileDown`/`ListProfilesDetailed`.
- `internal/node/session/docker_test.go`
- `cmd/argus/cli/profile.go` — top-level `argus profile` command group.

**Modified files:**
- `internal/node/session/lifecycle.go` — add `compose`/`profileLks` fields; extract `buildTmuxCmd`; call it from `Create` and `respawnTmux`.
- `internal/node/session/initscript.go` — add container inner-script generators + writers.
- `internal/node/session/initscript_test.go` — tests for the new generators.
- `internal/node/api/sessions.go` — `listProfiles` returns detailed info; add `profileUp`/`profileDown` handlers.
- `internal/node/api/router.go` — register `POST /profiles/{name}/up` and `/down`.
- `cmd/argus/main.go` — register `cli.NewProfileCmd()`.
- `web/src/data/sessions/queries.ts` — `ProfilesResponse` shape.
- `web/src/components/ChangeProfileDialog/index.tsx`, `web/src/components/NewSessionDialog/index.tsx` — map objects, render a dockerized badge.

---

## Task 1: docker package — detection, env, project name, path visibility

**Files:**
- Create: `internal/node/docker/docker.go`
- Test: `internal/node/docker/docker_test.go`

- [ ] **Step 1: Write the failing test**

```go
package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileComposeFile(t *testing.T) {
	state := t.TempDir()
	// Non-docker profile: hooks dir only.
	mkdir(t, filepath.Join(state, "profiles", "plain", "hooks"))
	if _, ok := ProfileComposeFile(state, "plain"); ok {
		t.Error("plain profile should not be dockerized")
	}
	// Docker profile: docker-compose.yml present.
	mkdir(t, filepath.Join(state, "profiles", "work", "hooks"))
	writeFile(t, filepath.Join(state, "profiles", "work", "docker-compose.yml"), "services: {}")
	file, ok := ProfileComposeFile(state, "work")
	if !ok {
		t.Fatal("work profile should be dockerized")
	}
	if file != filepath.Join(state, "profiles", "work", "docker-compose.yml") {
		t.Errorf("unexpected compose file: %s", file)
	}
	// compose.yaml is also recognized.
	mkdir(t, filepath.Join(state, "profiles", "alt", "hooks"))
	writeFile(t, filepath.Join(state, "profiles", "alt", "compose.yaml"), "services: {}")
	if _, ok := ProfileComposeFile(state, "alt"); !ok {
		t.Error("compose.yaml should be recognized")
	}
}

func TestProjectName(t *testing.T) {
	if got := ProjectName("work"); got != "argus-work" {
		t.Errorf("ProjectName = %q, want argus-work", got)
	}
}

func TestEnv(t *testing.T) {
	env := Env("/home/jeev", "/home/jeev/.argus")
	want := map[string]bool{
		"ARGUS_HOST_HOME=/home/jeev":        true,
		"ARGUS_STATE_DIR=/home/jeev/.argus": true,
	}
	for _, e := range env {
		delete(want, e)
	}
	if len(want) != 0 {
		t.Errorf("missing env entries: %v (got %v)", want, env)
	}
	hasUID, hasGID := false, false
	for _, e := range env {
		if len(e) > 9 && e[:9] == "ARGUS_UID" {
			hasUID = true
		}
		if len(e) > 9 && e[:9] == "ARGUS_GID" {
			hasGID = true
		}
	}
	if !hasUID || !hasGID {
		t.Errorf("expected ARGUS_UID and ARGUS_GID in env: %v", env)
	}
}

func TestPathVisible(t *testing.T) {
	home := "/home/jeev"
	state := "/data/argus"
	cases := []struct {
		path string
		want bool
	}{
		{"/home/jeev/repo/wt", true},
		{"/home/jeev", true},
		{"/data/argus/worktrees/x", true},
		{"/var/tmp/elsewhere", false},
		{"/home/jeevil/sneaky", false}, // prefix-but-not-subdir
	}
	for _, c := range cases {
		if got := PathVisible(c.path, home, state); got != c.want {
			t.Errorf("PathVisible(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/node/docker/`
Expected: FAIL — `undefined: ProfileComposeFile` (package doesn't compile).

- [ ] **Step 3: Write the implementation**

```go
// Package docker runs profile agent sessions inside docker-compose stacks.
// A profile is "dockerized" when its directory contains a compose file; the
// agent then runs via `docker compose exec` into a shared, per-profile stack.
package docker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// composeFileNames are the compose file names recognized in a profile
// directory, in priority order.
var composeFileNames = []string{"docker-compose.yml", "compose.yaml"}

// ProfileComposeFile returns the path to a profile's compose file and true when
// the profile is dockerized. It returns "" and false otherwise.
func ProfileComposeFile(stateDir, profile string) (string, bool) {
	for _, name := range composeFileNames {
		p := filepath.Join(stateDir, "profiles", profile, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}

// ProjectName returns the stable docker-compose project name for a profile.
// All sessions of a profile share this project (one stack per profile).
func ProjectName(profile string) string {
	return "argus-" + profile
}

// Env returns the environment passed to compose invocations so the compose
// file can mount the host home and state dir at identical paths and run the
// agent as the host user.
func Env(home, stateDir string) []string {
	return []string{
		"ARGUS_HOST_HOME=" + home,
		"ARGUS_STATE_DIR=" + stateDir,
		"ARGUS_UID=" + strconv.Itoa(os.Getuid()),
		"ARGUS_GID=" + strconv.Itoa(os.Getgid()),
	}
}

// PathVisible reports whether path is under the host home or the state dir,
// the two roots mounted into the container at identical paths. A session whose
// working directory is outside both cannot be seen inside the container.
func PathVisible(path, home, stateDir string) bool {
	return underRoot(path, home) || underRoot(path, stateDir)
}

func underRoot(path, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}
```

`fmt` is intentionally not imported here — it is first used in `compose.go` (Task 3).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/node/docker/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/docker/docker.go internal/node/docker/docker_test.go
git commit -m "feat(docker): profile compose-file detection, env, path visibility"
```

---

## Task 2: docker package — `docker compose exec` command builder

**Files:**
- Create: `internal/node/docker/exec.go`
- Test: `internal/node/docker/exec_test.go`

- [ ] **Step 1: Write the failing test**

```go
package docker

import (
	"strings"
	"testing"
)

func TestExecCommand(t *testing.T) {
	got := ExecCommand(ExecOptions{
		Project: "argus-work",
		File:    "/home/jeev/.argus/profiles/work/docker-compose.yml",
		Workdir: "/home/jeev/repo/wt",
		UID:     "1000",
		GID:     "1000",
		Service: "agent",
		Command: "bash /home/jeev/.argus/tmp/argus-inner-sess_1.sh",
	})

	for _, want := range []string{
		"docker compose",
		"-p 'argus-work'",
		"-f '/home/jeev/.argus/profiles/work/docker-compose.yml'",
		"exec",
		"-w '/home/jeev/repo/wt'",
		"-u '1000:1000'",
		" agent ",
		"bash /home/jeev/.argus/tmp/argus-inner-sess_1.sh",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ExecCommand missing %q in:\n%s", want, got)
		}
	}
}

func TestExecCommandQuotesSingleQuotes(t *testing.T) {
	got := ExecCommand(ExecOptions{
		Project: "argus-work",
		File:    "/x/it's/compose.yaml",
		Service: "agent",
		Command: "true",
	})
	if !strings.Contains(got, `'/x/it'\''s/compose.yaml'`) {
		t.Errorf("path with single quote not escaped: %s", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/node/docker/ -run TestExecCommand`
Expected: FAIL — `undefined: ExecCommand`.

- [ ] **Step 3: Write the implementation**

```go
package docker

import "strings"

// ExecOptions describe a `docker compose exec` invocation that runs Command
// inside a profile stack's service.
type ExecOptions struct {
	Project string
	File    string
	Workdir string
	UID     string
	GID     string
	Service string
	Command string // raw shell command, e.g. "bash /path/inner.sh"
}

// ExecCommand builds the shell command string that runs Command inside the
// profile's service, for embedding in the host tmux init script. Workdir,
// File, and Project are single-quoted; Command is appended verbatim (the
// caller is responsible for quoting any paths inside it).
func ExecCommand(o ExecOptions) string {
	var b strings.Builder
	b.WriteString("docker compose")
	b.WriteString(" -p " + shellQuote(o.Project))
	b.WriteString(" -f " + shellQuote(o.File))
	b.WriteString(" exec")
	if o.Workdir != "" {
		b.WriteString(" -w " + shellQuote(o.Workdir))
	}
	if o.UID != "" {
		b.WriteString(" -u " + shellQuote(o.UID+":"+o.GID))
	}
	b.WriteString(" " + o.Service + " ")
	b.WriteString(o.Command)
	return b.String()
}

// shellQuote returns a single-quoted shell string with internal single quotes
// escaped as '\''.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/node/docker/ -run TestExecCommand`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/docker/exec.go internal/node/docker/exec_test.go
git commit -m "feat(docker): docker compose exec command builder"
```

---

## Task 3: docker package — `CLICompose` (real compose wrapper)

**Files:**
- Create: `internal/node/docker/compose.go`

This wraps the `docker compose` CLI. It is exercised end-to-end by the opt-in integration test in Task 11; here we only add the type and a compile-time check that it satisfies the interface the session package will define. No unit test (it shells out to Docker).

- [ ] **Step 1: Write the implementation**

```go
package docker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CLICompose runs the `docker compose` CLI against a profile's stack.
type CLICompose struct{}

// NewCLICompose returns a CLICompose.
func NewCLICompose() *CLICompose { return &CLICompose{} }

// Up brings the stack up in detached mode. Output (including image build/pull
// progress) is attached to the error on failure.
func (CLICompose) Up(ctx context.Context, project, file string, env []string) error {
	return run(ctx, env, "-p", project, "-f", file, "up", "-d")
}

// Down tears the stack down.
func (CLICompose) Down(ctx context.Context, project, file string, env []string) error {
	return run(ctx, env, "-p", project, "-f", file, "down")
}

// IsUp reports whether any service container of the stack is running.
func (CLICompose) IsUp(ctx context.Context, project, file string, env []string) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "-p", project, "-f", file, "ps", "--status", "running", "-q"})...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("docker compose ps: %w", withStderr(err))
	}
	return strings.TrimSpace(string(out)) != "", nil
}

func run(ctx context.Context, env []string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker compose %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// withStderr appends an *exec.ExitError's captured stderr to the error text.
func withStderr(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/node/docker/`
Expected: builds cleanly.

- [ ] **Step 3: Commit**

```bash
git add internal/node/docker/compose.go
git commit -m "feat(docker): CLICompose up/down/is-up wrapper"
```

---

## Task 4: Container init-script generators

**Files:**
- Modify: `internal/node/session/initscript.go`
- Test: `internal/node/session/initscript_test.go`

The host wrapper script (banner + capture) is generated by the existing `GenerateInitScript`/`WriteInitScript` with the agent command set to the `docker compose exec ...` string and no hooks. We only need the **inner** scripts that run inside the container.

- [ ] **Step 1: Write the failing tests** (append to `initscript_test.go`)

```go
func TestGenerateContainerInitScript(t *testing.T) {
	hooks := []string{"/home/jeev/.argus/profiles/work/hooks/post_create.sh"}
	script := GenerateContainerInitScript("claude --resume abc", hooks)

	if !strings.HasPrefix(script, "#!/bin/bash") {
		t.Error("expected shebang")
	}
	if !strings.Contains(script, `source '/home/jeev/.argus/profiles/work/hooks/post_create.sh'`) {
		t.Error("expected sourced post_create hook")
	}
	if !strings.Contains(script, "claude --resume abc") {
		t.Error("expected agent command")
	}
	// No banner and no capture — those live in the host wrapper.
	if strings.Contains(script, "Argus Session Init") || strings.Contains(script, "tmux capture-pane") {
		t.Error("container script must not contain banner or capture")
	}
}

func TestGenerateContainerShellInitScript(t *testing.T) {
	// Always returns a script, even with no hooks (a containerized shell must
	// run through docker compose exec).
	script := GenerateContainerShellInitScript(nil)
	if !strings.Contains(script, "exec $SHELL -l") {
		t.Error("expected exec $SHELL -l")
	}
	withHooks := GenerateContainerShellInitScript([]string{"/h/post_create.sh"})
	if !strings.Contains(withHooks, "source '/h/post_create.sh'") {
		t.Error("expected sourced hook")
	}
}

func TestWriteContainerInitScript(t *testing.T) {
	state := t.TempDir()
	path, err := WriteContainerInitScript("sess_xyz", state, "claude", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Must live under <stateDir>/tmp so it is visible in the container.
	if !strings.HasPrefix(path, filepath.Join(state, "tmp")) {
		t.Errorf("inner script not under state tmp dir: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "claude") {
		t.Error("expected agent command in written file")
	}
}
```

Add imports `os` and `path/filepath` to the test file if not already present (it currently imports only `strings` and `testing`).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/node/session/ -run TestGenerateContainerInitScript`
Expected: FAIL — `undefined: GenerateContainerInitScript`.

- [ ] **Step 3: Write the implementation** (append to `initscript.go`)

```go
// GenerateContainerInitScript returns the script run INSIDE the container to
// launch an agent session: export PATH, source post_create hooks, run the
// agent command. The banner and provider-session-ID capture stay in the host
// wrapper (GenerateInitScript). The script self-deletes on start.
func GenerateContainerInitScript(agentCommand string, hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)
	hookSection := ""
	if hookBlock != "" {
		hookSection = "\n" + hookBlock
	}
	return "#!/bin/bash\n" +
		"# Argus Container Init Script\n" +
		"# Auto-generated - do not edit manually\n" +
		"rm -f -- \"$0\"\n" +
		"export PATH=\"$HOME/.local/bin:$PATH\"\n" +
		hookSection +
		"\n" +
		agentCommand + "\n"
}

// GenerateContainerShellInitScript returns the script run INSIDE the container
// for a shell session: source post_create hooks (if any) and exec the login
// shell. Unlike GenerateShellInitScript it always returns a script, because a
// containerized shell session must run through `docker compose exec`.
func GenerateContainerShellInitScript(hookPaths []string) string {
	hookBlock := generateHookSourceBlock(hookPaths)
	hookSection := ""
	if hookBlock != "" {
		hookSection = hookBlock + "\n"
	}
	return "#!/bin/bash\n" +
		"# Argus Container Shell Init Script\n" +
		"rm -f -- \"$0\"\n" +
		hookSection +
		"exec $SHELL -l\n"
}

// containerScriptDir returns <stateDir>/tmp, creating it 0700. This directory
// is mounted into the container at the same path, so init scripts written here
// are runnable in-container by path.
func containerScriptDir(stateDir string) (string, error) {
	dir := filepath.Join(stateDir, "tmp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create container script dir: %w", err)
	}
	return dir, nil
}

// WriteContainerInitScript writes the agent inner-init script under the mounted
// state tmp dir and returns its path (identical on host and in-container).
func WriteContainerInitScript(sessionID, stateDir, agentCommand string, hookPaths []string) (string, error) {
	dir, err := containerScriptDir(stateDir)
	if err != nil {
		return "", err
	}
	content := GenerateContainerInitScript(agentCommand, hookPaths)
	path := filepath.Join(dir, fmt.Sprintf("argus-inner-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write container init script: %w", err)
	}
	return path, nil
}

// WriteContainerShellInitScript writes the shell inner-init script under the
// mounted state tmp dir and returns its path.
func WriteContainerShellInitScript(sessionID, stateDir string, hookPaths []string) (string, error) {
	dir, err := containerScriptDir(stateDir)
	if err != nil {
		return "", err
	}
	content := GenerateContainerShellInitScript(hookPaths)
	path := filepath.Join(dir, fmt.Sprintf("argus-inner-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return "", fmt.Errorf("write container shell init script: %w", err)
	}
	return path, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/node/session/ -run 'TestGenerateContainer|TestWriteContainer'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/session/initscript.go internal/node/session/initscript_test.go
git commit -m "feat(session): container inner-init script generators"
```

---

## Task 5: Manager docker plumbing + profile-stack methods

**Files:**
- Modify: `internal/node/session/lifecycle.go` (struct fields + `NewManager`)
- Create: `internal/node/session/docker.go`
- Create: `internal/node/session/docker_test.go`

- [ ] **Step 1: Add fields to `Manager` and wire `NewManager`** (`lifecycle.go`)

In the `Manager` struct (after `sessLks`), add:

```go
	compose    composeRunner
	profileLks map[string]*sync.Mutex
```

In `NewManager`, set the compose default:

```go
func NewManager(database *db.DB, wt *worktree.Manager, stateDir string) *Manager {
	return &Manager{db: database, wt: wt, stateDir: stateDir, hooks: NewHookRunner(stateDir), compose: docker.NewCLICompose()}
}
```

Add the import `"github.com/bxnlabs/argus/internal/node/docker"` to `lifecycle.go`.

- [ ] **Step 2: Write the failing test** (`docker_test.go`)

```go
package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
)

// fakeCompose records calls and tracks per-project up state.
type fakeCompose struct {
	up        map[string]bool
	upCalls   []string
	downCalls []string
	isUpErr   error
}

func newFakeCompose() *fakeCompose { return &fakeCompose{up: map[string]bool{}} }

func (f *fakeCompose) Up(_ context.Context, project, _ string, _ []string) error {
	f.upCalls = append(f.upCalls, project)
	f.up[project] = true
	return nil
}
func (f *fakeCompose) Down(_ context.Context, project, _ string, _ []string) error {
	f.downCalls = append(f.downCalls, project)
	f.up[project] = false
	return nil
}
func (f *fakeCompose) IsUp(_ context.Context, project, _ string, _ []string) (bool, error) {
	if f.isUpErr != nil {
		return false, f.isUpErr
	}
	return f.up[project], nil
}

func dockerTestManager(t *testing.T) (*Manager, *fakeCompose, string) {
	t.Helper()
	state := t.TempDir()
	database, err := db.Open(filepath.Join(state, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	wt := worktree.NewManager(state, &config.Config{})
	mgr := NewManager(database, wt, state)
	fake := newFakeCompose()
	mgr.compose = fake
	return mgr, fake, state
}

// makeProfile creates a profile dir with a hooks/ subdir, plus a compose file
// when dockerized.
func makeProfile(t *testing.T, state, name string, dockerized bool) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(state, "profiles", name, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if dockerized {
		if err := os.WriteFile(filepath.Join(state, "profiles", name, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProfileUpDown(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)

	if err := mgr.ProfileUp("work"); err != nil {
		t.Fatalf("ProfileUp: %v", err)
	}
	if !fake.up["argus-work"] {
		t.Error("expected stack up after ProfileUp")
	}
	// Idempotent: already up → no second Up call.
	if err := mgr.ProfileUp("work"); err != nil {
		t.Fatal(err)
	}
	if len(fake.upCalls) != 1 {
		t.Errorf("expected 1 up call, got %d", len(fake.upCalls))
	}

	if err := mgr.ProfileDown("work"); err != nil {
		t.Fatalf("ProfileDown: %v", err)
	}
	if fake.up["argus-work"] {
		t.Error("expected stack down after ProfileDown")
	}
}

func TestProfileUpRejectsNonDocker(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "plain", false)
	if err := mgr.ProfileUp("plain"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for non-docker profile, got %v", err)
	}
}

func TestListProfilesDetailed(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "plain", false)
	makeProfile(t, state, "work", true)
	fake.up["argus-work"] = true

	infos, err := mgr.ListProfilesDetailed()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ProfileInfo{}
	for _, i := range infos {
		got[i.Name] = i
	}
	if got["plain"].Dockerized || got["plain"].Stack != "-" {
		t.Errorf("plain: %+v", got["plain"])
	}
	if !got["work"].Dockerized || got["work"].Stack != "up" {
		t.Errorf("work: %+v", got["work"])
	}
}
```

- [ ] **Step 3: Run to verify it fails**

Run: `go test ./internal/node/session/ -run 'TestProfileUpDown|TestProfileUpRejects|TestListProfilesDetailed'`
Expected: FAIL — `undefined: ProfileInfo`, `mgr.compose undefined`, etc.

- [ ] **Step 4: Write the implementation** (`docker.go`)

```go
package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/bxnlabs/argus/internal/node/docker"
)

// composeRunner is the subset of docker.CLICompose the Manager depends on, so
// tests can substitute a fake.
type composeRunner interface {
	Up(ctx context.Context, project, file string, env []string) error
	Down(ctx context.Context, project, file string, env []string) error
	IsUp(ctx context.Context, project, file string, env []string) (bool, error)
}

// ProfileInfo describes a profile for the listing API/CLI.
type ProfileInfo struct {
	Name       string `json:"name"`
	Dockerized bool   `json:"dockerized"`
	Stack      string `json:"stack"` // "up" | "down" | "-" (host) | "?" (daemon error)
}

// profileLock returns a per-profile mutex, creating it if needed. It serializes
// stack up/down for a profile so concurrent session creates bring the shared
// stack up exactly once.
func (m *Manager) profileLock(name string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profileLks == nil {
		m.profileLks = make(map[string]*sync.Mutex)
	}
	l, ok := m.profileLks[name]
	if !ok {
		l = &sync.Mutex{}
		m.profileLks[name] = l
	}
	return l
}

// ensureStackUp brings the profile's compose stack up if it is not already
// running. Serialized per profile.
func (m *Manager) ensureStackUp(profile, composeFile string) error {
	l := m.profileLock(profile)
	l.Lock()
	defer l.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	project := docker.ProjectName(profile)
	env := docker.Env(home, m.stateDir)
	up, err := m.compose.IsUp(context.Background(), project, composeFile, env)
	if err != nil {
		return err
	}
	if up {
		return nil
	}
	return m.compose.Up(context.Background(), project, composeFile, env)
}

// ProfileUp brings a dockerized profile's stack up. Returns ErrInvalidInput if
// the profile is not dockerized.
func (m *Manager) ProfileUp(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	file, ok := docker.ProfileComposeFile(m.stateDir, name)
	if !ok {
		return fmt.Errorf("%w: profile %q is not dockerized", ErrInvalidInput, name)
	}
	return m.ensureStackUp(name, file)
}

// ProfileDown tears a dockerized profile's stack down.
func (m *Manager) ProfileDown(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	file, ok := docker.ProfileComposeFile(m.stateDir, name)
	if !ok {
		return fmt.Errorf("%w: profile %q is not dockerized", ErrInvalidInput, name)
	}
	l := m.profileLock(name)
	l.Lock()
	defer l.Unlock()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	return m.compose.Down(context.Background(), docker.ProjectName(name), file, docker.Env(home, m.stateDir))
}

// ListProfilesDetailed returns every profile annotated with its type and, for
// dockerized profiles, its stack status. Status lookups are best-effort: a
// Docker daemon error yields "?" rather than failing the listing.
func (m *Manager) ListProfilesDetailed() ([]ProfileInfo, error) {
	names, err := m.ListProfiles()
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	out := make([]ProfileInfo, 0, len(names))
	for _, name := range names {
		info := ProfileInfo{Name: name, Stack: "-"}
		if file, ok := docker.ProfileComposeFile(m.stateDir, name); ok {
			info.Dockerized = true
			info.Stack = "?"
			if up, err := m.compose.IsUp(context.Background(), docker.ProjectName(name), file, docker.Env(home, m.stateDir)); err == nil {
				if up {
					info.Stack = "up"
				} else {
					info.Stack = "down"
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// dockerIdentity returns the host UID/GID as strings for `exec --user`.
func dockerIdentity() (uid, gid string) {
	return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
}
```

`provider` is intentionally not imported yet — it is first used by `buildTmuxCmd` in Task 6.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/node/session/ -run 'TestProfileUpDown|TestProfileUpRejects|TestListProfilesDetailed'`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/node/session/lifecycle.go internal/node/session/docker.go internal/node/session/docker_test.go
git commit -m "feat(session): profile stack up/down/list with compose runner"
```

---

## Task 6: Extract `buildTmuxCmd` (non-docker refactor)

**Files:**
- Modify: `internal/node/session/lifecycle.go`
- Test: `internal/node/session/docker_test.go`

This is a pure refactor: pull the duplicated tmux-command construction out of `Create` and `respawnTmux` into one method. Behavior is unchanged for host (non-docker) profiles.

- [ ] **Step 1: Write the failing test** (append to `docker_test.go`)

```go
func TestBuildTmuxCmd_HostAgent(t *testing.T) {
	mgr, _, _ := dockerTestManager(t)
	cmd, err := mgr.buildTmuxCmd("sess_1", "claude", "", t.TempDir(), "claude --resume x", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cmd, "bash ") {
		t.Errorf("expected 'bash <script>', got %q", cmd)
	}
}

func TestBuildTmuxCmd_HostShellNoHooks(t *testing.T) {
	mgr, _, _ := dockerTestManager(t)
	// Shell provider (empty agent command) with no hooks → no init script.
	cmd, err := mgr.buildTmuxCmd("sess_2", "shell", "", t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if cmd != "" {
		t.Errorf("expected empty command for hookless shell session, got %q", cmd)
	}
}
```

Add `"strings"` to the `docker_test.go` imports.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/node/session/ -run TestBuildTmuxCmd`
Expected: FAIL — `mgr.buildTmuxCmd undefined`.

- [ ] **Step 3: Add `buildTmuxCmd`** (in `docker.go`)

```go
// buildTmuxCmd constructs the command tmux runs for a session: the init script
// that launches the agent (or shell) and sources post_create hooks. For a
// dockerized profile the agent/shell runs inside the profile's container via
// `docker compose exec`; otherwise it runs directly on the host. An empty
// return means "start tmux's default shell with no init script".
func (m *Manager) buildTmuxCmd(sessionID, providerType, profile, cwd, agentCmd string, postCreatePaths []string) (string, error) {
	if composeFile, ok := docker.ProfileComposeFile(m.stateDir, profile); ok {
		return m.buildDockerTmuxCmd(sessionID, providerType, profile, composeFile, cwd, agentCmd, postCreatePaths)
	}
	// Host (non-docker) path — unchanged behavior.
	if agentCmd != "" {
		pattern := provider.GetSessionIDPattern(provider.ProviderType(providerType))
		scriptPath, err := WriteInitScript(sessionID, agentCmd, pattern, postCreatePaths)
		if err != nil {
			return "", fmt.Errorf("write init script: %w", err)
		}
		return "bash " + scriptPath, nil
	}
	if len(postCreatePaths) > 0 {
		scriptPath, err := WriteShellInitScript(sessionID, postCreatePaths)
		if err != nil {
			return "", fmt.Errorf("write shell init script: %w", err)
		}
		if scriptPath != "" {
			return "bash " + scriptPath, nil
		}
	}
	return "", nil
}
```

Add the `provider` import to `docker.go` (`"github.com/bxnlabs/argus/internal/node/provider"`) — it is first used here. `buildDockerTmuxCmd` doesn't exist yet — add a temporary stub at the bottom of `docker.go` so the package compiles (Task 7 replaces it).

```go
// buildDockerTmuxCmd is implemented in Task 7.
func (m *Manager) buildDockerTmuxCmd(sessionID, providerType, profile, composeFile, cwd, agentCmd string, postCreatePaths []string) (string, error) {
	return "", fmt.Errorf("not implemented")
}
```

- [ ] **Step 4: Replace the inline block in `Create`** (`lifecycle.go`)

Find the block in `Create` that begins with `var tmuxCmd string` and ends just before `// Spawn tmux session` (it builds `tmuxCmd` from `agentCmd`/`postCreatePaths`). Replace the whole block with:

```go
	tmuxCmd, err := m.buildTmuxCmd(sessionID, opts.ProviderType, resolvedProfile, cwd, agentCmd, postCreatePaths)
	if err != nil {
		return nil, err
	}
```

- [ ] **Step 5: Replace the inline block in `respawnTmux`** (`lifecycle.go`)

Find the matching `var tmuxCmd string` block in `respawnTmux` (after `postCreatePaths := ...`) and replace it with:

```go
	tmuxCmd, err := m.buildTmuxCmd(session.ID, session.ProviderType, profileName, cwd, agentCmd, postCreatePaths)
	if err != nil {
		return "", err
	}
```

- [ ] **Step 6: Run the full session test suite to verify no regression**

Run: `go test ./internal/node/session/`
Expected: PASS (existing Create/respawn/clone/profile tests still green; new `TestBuildTmuxCmd_*` pass). The docker stub is never hit by host-profile tests.

- [ ] **Step 7: Commit**

```bash
git add internal/node/session/lifecycle.go internal/node/session/docker.go internal/node/session/docker_test.go
git commit -m "refactor(session): extract buildTmuxCmd from Create/respawnTmux"
```

---

## Task 7: Docker branch — `buildDockerTmuxCmd` + cwd validation

**Files:**
- Modify: `internal/node/session/docker.go`
- Test: `internal/node/session/docker_test.go`

- [ ] **Step 1: Write the failing test** (append to `docker_test.go`)

```go
func TestBuildDockerTmuxCmd_AgentSession(t *testing.T) {
	mgr, fake, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	cwd := filepath.Join(state, "wt") // under stateDir → visible in container
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd, err := mgr.buildTmuxCmd("sess_d1", "claude", "work", cwd, "claude --resume z", nil)
	if err != nil {
		t.Fatalf("buildTmuxCmd: %v", err)
	}
	if !strings.HasPrefix(cmd, "bash ") {
		t.Fatalf("expected 'bash <host-wrapper>', got %q", cmd)
	}
	// Lazy-up happened.
	if !fake.up["argus-work"] {
		t.Error("expected stack brought up")
	}
	// The inner script was written under the mounted state tmp dir.
	innerPath := filepath.Join(state, "tmp", "argus-inner-sess_d1.sh")
	if _, err := os.Stat(innerPath); err != nil {
		t.Errorf("inner script not written: %v", err)
	}
	// The host wrapper invokes docker compose exec into the agent service.
	hostScript := strings.TrimPrefix(cmd, "bash ")
	data, err := os.ReadFile(hostScript)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"docker compose", "exec", "-p 'argus-work'", "agent", innerPath} {
		if !strings.Contains(string(data), want) {
			t.Errorf("host wrapper missing %q", want)
		}
	}
	// Capture logic present (claude supports resume).
	if !strings.Contains(string(data), "tmux capture-pane") {
		t.Error("expected provider-id capture in host wrapper")
	}
}

func TestBuildDockerTmuxCmd_RejectsInvisibleCwd(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	// A cwd outside both home and the state dir.
	_, err := mgr.buildTmuxCmd("sess_d2", "claude", "work", "/nonexistent-root-xyz/wt", "claude", nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for invisible cwd, got %v", err)
	}
}

func TestBuildDockerTmuxCmd_ShellSession(t *testing.T) {
	mgr, _, state := dockerTestManager(t)
	makeProfile(t, state, "work", true)
	cwd := filepath.Join(state, "sh")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := mgr.buildTmuxCmd("sess_d3", "shell", "work", cwd, "", nil)
	if err != nil {
		t.Fatalf("buildTmuxCmd shell: %v", err)
	}
	hostScript := strings.TrimPrefix(cmd, "bash ")
	data, err := os.ReadFile(hostScript)
	if err != nil {
		t.Fatal(err)
	}
	// Shell session: docker exec present, but no provider-id capture.
	if !strings.Contains(string(data), "docker compose") {
		t.Error("expected docker compose exec for shell session")
	}
	if strings.Contains(string(data), "tmux capture-pane") {
		t.Error("shell session must not capture a provider id")
	}
	innerData, err := os.ReadFile(filepath.Join(state, "tmp", "argus-inner-sess_d3.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(innerData), "exec $SHELL -l") {
		t.Error("expected container shell inner script")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/node/session/ -run TestBuildDockerTmuxCmd`
Expected: FAIL — stub returns "not implemented".

- [ ] **Step 3: Replace the stub with the real implementation** (`docker.go`)

```go
// buildDockerTmuxCmd builds the host wrapper command for a dockerized profile.
// It validates the cwd is visible in the container, ensures the stack is up,
// writes the inner init script under the mounted state tmp dir, and wraps it in
// a `docker compose exec` invocation embedded in the standard host init script
// (which provides the banner and, for resume-capable providers, the
// provider-session-ID capture).
func (m *Manager) buildDockerTmuxCmd(sessionID, providerType, profile, composeFile, cwd, agentCmd string, postCreatePaths []string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	if !docker.PathVisible(cwd, home, m.stateDir) {
		return "", fmt.Errorf("%w: working directory %q is not under the home or state directory mounted into the %q profile container", ErrInvalidInput, cwd, profile)
	}
	if err := m.ensureStackUp(profile, composeFile); err != nil {
		return "", fmt.Errorf("start profile %q stack: %w", profile, err)
	}

	var innerPath string
	pattern := ""
	if agentCmd != "" {
		innerPath, err = WriteContainerInitScript(sessionID, m.stateDir, agentCmd, postCreatePaths)
		pattern = provider.GetSessionIDPattern(provider.ProviderType(providerType))
	} else {
		innerPath, err = WriteContainerShellInitScript(sessionID, m.stateDir, postCreatePaths)
	}
	if err != nil {
		return "", err
	}

	uid, gid := dockerIdentity()
	execCmd := docker.ExecCommand(docker.ExecOptions{
		Project: docker.ProjectName(profile),
		File:    composeFile,
		Workdir: cwd,
		UID:     uid,
		GID:     gid,
		Service: "agent",
		Command: "bash " + shellQuote(innerPath),
	})

	// The host wrapper is the standard init script with the agent command set
	// to the docker-exec string and no host-side hooks (hooks are sourced
	// inside the container by the inner script).
	hostPath, err := WriteInitScript(sessionID, execCmd, pattern, nil)
	if err != nil {
		return "", fmt.Errorf("write host wrapper script: %w", err)
	}
	return "bash " + hostPath, nil
}
```

`shellQuote` already exists in `initscript.go` (same package). Remove the temporary stub function added in Task 6.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/node/session/ -run TestBuildDockerTmuxCmd`
Expected: PASS.

- [ ] **Step 5: Run the full backend build + session suite**

Run: `go build ./... && go test ./internal/node/session/ ./internal/node/docker/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/node/session/docker.go internal/node/session/docker_test.go
git commit -m "feat(session): run agent in profile container via docker compose exec"
```

---

## Task 8: API — detailed `/profiles` + stack up/down endpoints

**Files:**
- Modify: `internal/node/api/sessions.go`
- Modify: `internal/node/api/router.go`
- Test: `internal/node/api/sessions_test.go`

- [ ] **Step 1: Write the failing test** (append to `sessions_test.go`)

```go
// newProfileTestHandler builds a sessionHandler backed by a real Manager over a
// temp state dir (mirrors the construction already used in this file).
func newProfileTestHandler(t *testing.T) (*sessionHandler, string) {
	t.Helper()
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	wt := worktree.NewManager(stateDir, &config.Config{})
	mgr := session.NewManager(database, wt, stateDir)
	return &sessionHandler{manager: mgr}, stateDir
}

func TestProfileUpHandler_NotDockerized(t *testing.T) {
	h, stateDir := newProfileTestHandler(t)
	// Create a non-docker profile so ProfileUp returns ErrInvalidInput.
	if err := os.MkdirAll(filepath.Join(stateDir, "profiles", "plain", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/profiles/plain/up", nil)
	req.SetPathValue("name", "plain")
	w := httptest.NewRecorder()
	h.profileUp(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-docker profile, got %d", w.Code)
	}
}
```

The imports `db`, `worktree`, `config`, and `session` are already used elsewhere in `sessions_test.go`; add `os`, `path/filepath`, `net/http`, and `net/http/httptest` if not already present. If a similar handler-construction helper already exists in the file, reuse it instead of adding `newProfileTestHandler`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/node/api/ -run TestProfileUpHandler`
Expected: FAIL — `h.profileUp undefined`.

- [ ] **Step 3: Update `listProfiles` and add handlers** (`sessions.go`)

Replace the body of `listProfiles` with the detailed listing:

```go
// GET /profiles
func (h *sessionHandler) listProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.manager.ListProfilesDetailed()
	if err != nil {
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

// POST /profiles/{name}/up brings a dockerized profile's stack up.
func (h *sessionHandler) profileUp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.manager.ProfileUp(name); err != nil {
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"name": name, "stack": "up"})
}

// POST /profiles/{name}/down tears a dockerized profile's stack down.
func (h *sessionHandler) profileDown(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := h.manager.ProfileDown(name); err != nil {
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"name": name, "stack": "down"})
}
```

`errors` and `session` are already imported in `sessions.go` (used by existing handlers).

- [ ] **Step 4: Register the routes** (`router.go`)

In the `// Profile routes` section, add after the existing `GET /profiles` line:

```go
	mux.HandleFunc("POST /profiles/{name}/up", sh.profileUp)
	mux.HandleFunc("POST /profiles/{name}/down", sh.profileDown)
```

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./internal/node/api/ -run TestProfile && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/node/api/sessions.go internal/node/api/router.go internal/node/api/sessions_test.go
git commit -m "feat(api): detailed /profiles listing + stack up/down endpoints"
```

---

## Task 9: CLI — top-level `argus profile` group

**Files:**
- Create: `cmd/argus/cli/profile.go`
- Modify: `cmd/argus/main.go`

The existing session-scoped `argus session profile set/rm` stays as-is (those assign a profile to a session). The new top-level group manages profile *stacks*.

- [ ] **Step 1: Write the implementation** (`profile.go`)

```go
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// profileInfo mirrors the node API's ProfileInfo payload.
type profileInfo struct {
	Name       string `json:"name"`
	Dockerized bool   `json:"dockerized"`
	Stack      string `json:"stack"`
}

// NewProfileCmd returns the top-level "profile" command group for managing
// dockerized-profile compose stacks.
func NewProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage dockerized-profile stacks",
		// Profile commands use only the discovery file (no config loading),
		// mirroring the session command group.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
	}
	cmd.AddCommand(newProfileLsCmd(), newProfileUpCmd(), newProfileDownCmd(), newProfileStatusCmd())
	return cmd
}

// profileClient builds an API client from the discovery file.
func profileClient() (*apiClient, error) {
	path, err := discoveryFilePath()
	if err != nil {
		return nil, err
	}
	return newClient(path)
}

func fetchProfiles(c *apiClient) ([]profileInfo, error) {
	body, err := c.get("/profiles")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Profiles []profileInfo `json:"profiles"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return resp.Profiles, nil
}

func newProfileLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List profiles and their stack status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			profiles, err := fetchProfiles(c)
			if err != nil {
				return err
			}
			if len(profiles) == 0 {
				fmt.Println("No profiles.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tTYPE\tSTACK")
			for _, p := range profiles {
				typ := "host"
				if p.Dockerized {
					typ = "docker"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, typ, p.Stack)
			}
			return w.Flush()
		},
	}
}

func newProfileUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up <profile>",
		Short: "Bring a dockerized profile's stack up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			if _, err := c.post("/profiles/"+args[0]+"/up", nil); err != nil {
				return err
			}
			fmt.Printf("Profile %q stack is up\n", args[0])
			return nil
		},
	}
}

func newProfileDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down <profile>",
		Short: "Tear a dockerized profile's stack down",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			if _, err := c.post("/profiles/"+args[0]+"/down", nil); err != nil {
				return err
			}
			fmt.Printf("Profile %q stack is down\n", args[0])
			return nil
		},
	}
}

func newProfileStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <profile>",
		Short: "Show a profile's stack status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			c, err := profileClient()
			if err != nil {
				return err
			}
			profiles, err := fetchProfiles(c)
			if err != nil {
				return err
			}
			for _, p := range profiles {
				if p.Name == args[0] {
					typ := "host"
					if p.Dockerized {
						typ = "docker"
					}
					fmt.Printf("%s (%s): %s\n", p.Name, typ, p.Stack)
					return nil
				}
			}
			return fmt.Errorf("profile %q not found", args[0])
		},
	}
}
```

- [ ] **Step 2: Register the command** (`main.go`)

In `newRootCmd`'s `rootCmd.AddCommand(...)`, add `cli.NewProfileCmd(),` alongside `cli.NewSessionCmd(),`.

- [ ] **Step 3: Verify it builds and the command is wired**

Run: `go build ./... && go run ./cmd/argus profile --help`
Expected: builds; help lists `ls`, `up`, `down`, `status`.

- [ ] **Step 4: Commit**

```bash
git add cmd/argus/cli/profile.go cmd/argus/main.go
git commit -m "feat(cli): top-level 'argus profile' stack commands"
```

---

## Task 10: Web — `/profiles` shape + consumers + dockerized badge

**Files:**
- Modify: `web/src/data/sessions/queries.ts`
- Modify: `web/src/components/ChangeProfileDialog/index.tsx`
- Modify: `web/src/components/NewSessionDialog/index.tsx`

The API now returns objects, not strings, so the consumers must be updated or the build breaks.

- [ ] **Step 1: Update the response type** (`queries.ts`)

Replace:

```ts
interface ProfilesResponse {
  profiles: string[];
}
```

with:

```ts
export interface ProfileInfo {
  name: string;
  dockerized: boolean;
  stack: string; // "up" | "down" | "-" | "?"
}

interface ProfilesResponse {
  profiles: ProfileInfo[];
}
```

- [ ] **Step 2: Update `ChangeProfileDialog`** (`index.tsx`)

The `profiles` array is now `ProfileInfo[]`. Replace the `.map` over `profiles` (currently `key={p} value={p}>{p}`) with:

```tsx
                {profiles.map((p) => (
                  <SelectItem key={p.name} value={p.name}>
                    {p.name}
                    {p.dockerized && (
                      <span className="text-muted-foreground ml-2 text-xs">🐳</span>
                    )}
                  </SelectItem>
                ))}
```

The `const profiles = profilesData?.profiles ?? [];` line is unchanged (it's now `ProfileInfo[]`). No other usages of `profiles` as strings exist in this file.

- [ ] **Step 3: Update `NewSessionDialog`** (`index.tsx`)

Replace its `profiles.map` block (currently `key={p} value={p}>{p}`) with:

```tsx
                    {profiles.map((p) => (
                      <SelectItem key={p.name} value={p.name}>
                        {p.name}
                        {p.dockerized && (
                          <span className="text-muted-foreground ml-2 text-xs">🐳</span>
                        )}
                      </SelectItem>
                    ))}
```

`profiles.length > 0` and `const profiles = profilesData?.profiles ?? [];` remain valid (array length / array). Confirm no other code in the file treats a profile entry as a string (it doesn't — `profile` state is a separate string holding the selected name).

- [ ] **Step 4: Typecheck + run web tests**

Run: `pnpm -C web exec tsc -b && pnpm -C web test`
Expected: typecheck passes; existing tests (`SessionList`, `SessionInfoDialog`, etc.) pass.

> If typecheck flags any other `profilesData.profiles` usage as `string[]`, update it to read `.name`. Search: `grep -rn "profilesData" web/src`.

- [ ] **Step 5: Commit**

```bash
git add web/src/data/sessions/queries.ts web/src/components/ChangeProfileDialog/index.tsx web/src/components/NewSessionDialog/index.tsx
git commit -m "feat(web): dockerized-profile badge and detailed /profiles shape"
```

---

## Task 11: Opt-in Docker integration test

**Files:**
- Create: `internal/node/docker/compose_integration_test.go`

Guarded so it never runs in normal CI: skips unless `ARGUS_DOCKER_IT=1` and the `docker` CLI is present. Verifies the real `CLICompose` up → is-up → down round trip against a trivial stack.

- [ ] **Step 1: Write the test**

```go
package docker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCLICompose_RoundTrip(t *testing.T) {
	if os.Getenv("ARGUS_DOCKER_IT") != "1" {
		t.Skip("set ARGUS_DOCKER_IT=1 to run docker integration test")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}

	dir := t.TempDir()
	compose := filepath.Join(dir, "docker-compose.yml")
	// A minimal keep-alive service named like the agent service.
	content := "services:\n  agent:\n    image: busybox\n    command: sleep 300\n"
	if err := os.WriteFile(compose, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewCLICompose()
	project := "argus-it-roundtrip"
	env := Env("/tmp", dir)
	ctx := context.Background()

	t.Cleanup(func() { _ = c.Down(ctx, project, compose, env) })

	if err := c.Up(ctx, project, compose, env); err != nil {
		t.Fatalf("Up: %v", err)
	}
	up, err := c.IsUp(ctx, project, compose, env)
	if err != nil {
		t.Fatalf("IsUp: %v", err)
	}
	if !up {
		t.Fatal("expected stack to be up")
	}
	if err := c.Down(ctx, project, compose, env); err != nil {
		t.Fatalf("Down: %v", err)
	}
	down, err := c.IsUp(ctx, project, compose, env)
	if err != nil {
		t.Fatalf("IsUp after down: %v", err)
	}
	if down {
		t.Fatal("expected stack to be down")
	}
}
```

- [ ] **Step 2: Verify it skips by default**

Run: `go test ./internal/node/docker/ -run TestCLICompose_RoundTrip -v`
Expected: `--- SKIP` (no `ARGUS_DOCKER_IT`).

- [ ] **Step 3: (Optional, if Docker available) run it for real**

Run: `ARGUS_DOCKER_IT=1 go test ./internal/node/docker/ -run TestCLICompose_RoundTrip -v`
Expected: PASS (pulls `busybox`, brings the stack up/down).

- [ ] **Step 4: Commit**

```bash
git add internal/node/docker/compose_integration_test.go
git commit -m "test(docker): opt-in compose up/is-up/down integration test"
```

---

## Final verification

- [ ] **Run the full backend suite**

Run: `go build ./... && go test ./...`
Expected: PASS (the docker integration test skips).

- [ ] **Run the web suite + typecheck**

Run: `pnpm -C web exec tsc -b && pnpm -C web test`
Expected: PASS.

- [ ] **Manual smoke (optional, requires Docker + a real node)**

1. Create a profile dir: `~/.argus/profiles/demo/` with an empty `hooks/` and a `docker-compose.yml` defining an `agent` service (image with `claude` installed, `command: sleep infinity`, mounting `${ARGUS_HOST_HOME}` and `${ARGUS_STATE_DIR}` at identical paths, `user: "${ARGUS_UID}:${ARGUS_GID}"`).
2. `argus profile ls` → shows `demo  docker  down`.
3. Create a session with `--profile demo`; confirm via `docker ps` that the stack came up and the agent runs inside the container; confirm files written in the worktree are owned by your host user.
4. `argus profile ls` → `demo  docker  up`; `argus profile down demo` → stack torn down.

---

## Notes / decisions baked into this plan

- **Docker profile dir shape:** a dockerized profile needs both an (empty-OK) `hooks/` dir and a `docker-compose.yml`/`compose.yaml`. The `hooks/` requirement is inherited from existing profile validity (`resolveProfile`, `ListProfiles`); relaxing it is out of scope.
- **Agent service name** is the convention `agent` (no per-profile override in v1).
- **Hooks:** `post_create` is sourced inside the container (via the inner init script); `pre_create`/`on_create_worktree`/`pre_destroy` run on the host unchanged (untouched in `Create`/`Delete`/`ChangeProfile`).
- **`post_create` env** does not cross into the container beyond what the inner script sources; agent runtime env (API keys, model, PATH) should live in the image/compose `environment:`/`env_file:`.
- **Lazy up / manual down:** `ensureStackUp` is called from `buildTmuxCmd`'s docker branch, so `Create`, `EnsureSession`→`respawnTmux`, and `ChangeProfile`→`respawnTmux` all trigger lazy-up. Nothing tears a stack down automatically.
- **`<stateDir>/tmp`** is created lazily (0700) by `containerScriptDir`. If you prefer the repo's bootstrap-at-startup convention, add an `EnsureSecureDir(filepath.Join(stateDir, "tmp"))` call in `node.Setup` and drop the `MkdirAll` — both are acceptable.
