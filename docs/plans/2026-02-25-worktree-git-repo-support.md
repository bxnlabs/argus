# Worktree & Git Repo Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Sessions launched from git repos (local or remote) automatically run in isolated git worktrees; remote repos can be specified as `org/repo`, a full HTTPS URL, or an SSH URL and are cloned on demand.

**Architecture:** A new `internal/source` package resolves `--src` to either a local path or a normalized remote git URL. A new `internal/worktree` package handles cloning, worktree creation, and branch naming. The session lifecycle (`internal/agent/session/lifecycle.go`) calls these packages before spawning tmux. A new `internal/config` package reads `~/.argus/config.toml` for user preferences (e.g. `branch_prefix`).

**Tech Stack:** Go stdlib (`os/exec` for git), `github.com/BurntSushi/toml` for config parsing, React + TypeScript for UI changes, existing Cobra CLI and SQLite patterns.

---

### Task 1: Config package + TOML dependency

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Add the TOML dependency**

```bash
go get github.com/BurntSushi/toml@latest
```

Expected: `go.mod` and `go.sum` updated with `github.com/BurntSushi/toml`.

**Step 2: Write the failing tests**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
)

func TestLoadMissingFile(t *testing.T) {
	cfg, err := config.LoadFrom("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg.BranchPrefix != "" {
		t.Errorf("expected empty BranchPrefix, got %q", cfg.BranchPrefix)
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`branch_prefix = "jeev"`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BranchPrefix != "jeev" {
		t.Errorf("expected BranchPrefix %q, got %q", "jeev", cfg.BranchPrefix)
	}
}

func TestLoadMalformedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`not valid toml [[[[`), 0644); err != nil {
		t.Fatal(err)
	}
	// Malformed config should return defaults, not error
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("expected no error for malformed config, got %v", err)
	}
	if cfg.BranchPrefix != "" {
		t.Errorf("expected empty BranchPrefix for malformed config, got %q", cfg.BranchPrefix)
	}
}
```

**Step 3: Run tests to verify they fail**

```bash
go test ./internal/config/... -v
```

Expected: compile error — package does not exist yet.

**Step 4: Implement the config package**

Create `internal/config/config.go`:

```go
package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds user preferences loaded from ~/.argus/config.toml.
type Config struct {
	BranchPrefix string `toml:"branch_prefix"`
}

// Load loads the config from the default location (~/.argus/config.toml).
// Missing or malformed files are silently ignored; defaults are returned.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultConfig(), nil
	}
	return LoadFrom(filepath.Join(home, ".argus", "config.toml"))
}

// LoadFrom loads the config from the given path.
// Missing or malformed files are silently ignored; defaults are returned.
func LoadFrom(path string) (*Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, nil
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		// Malformed config: warn to stderr, use defaults.
		return cfg, nil
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{}
}
```

**Step 5: Run tests to verify they pass**

```bash
go test ./internal/config/... -v
```

Expected: all 3 tests PASS.

**Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: add config package for ~/.argus/config.toml"
```

---

### Task 2: Source resolver

**Files:**
- Create: `internal/source/source.go`
- Create: `internal/source/source_test.go`

**Step 1: Write the failing tests**

Create `internal/source/source_test.go`:

```go
package source_test

import (
	"os"
	"testing"

	"github.com/bxnlabs/argus/internal/source"
)

func TestResolveLocalPath(t *testing.T) {
	dir := t.TempDir()
	src, err := source.Resolve(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.IsRemote() {
		t.Fatal("expected local source")
	}
	if src.LocalPath != dir {
		t.Errorf("expected LocalPath %q, got %q", dir, src.LocalPath)
	}
}

func TestResolveNonexistentPathTreatedAsRemote(t *testing.T) {
	// A path that doesn't exist and isn't a valid git URL → error
	_, err := source.Resolve("/definitely/does/not/exist/on/this/system")
	if err == nil {
		t.Fatal("expected error for nonexistent path that is not a git URL")
	}
}

func TestResolveOrgRepoShorthand(t *testing.T) {
	src, err := source.Resolve("bxnlabs/argus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsRemote() {
		t.Fatal("expected remote source")
	}
	if src.Host != "github.com" {
		t.Errorf("expected Host %q, got %q", "github.com", src.Host)
	}
	if src.Org != "bxnlabs" {
		t.Errorf("expected Org %q, got %q", "bxnlabs", src.Org)
	}
	if src.Repo != "argus" {
		t.Errorf("expected Repo %q, got %q", "argus", src.Repo)
	}
	if src.RemoteURL != "https://github.com/bxnlabs/argus.git" {
		t.Errorf("unexpected RemoteURL %q", src.RemoteURL)
	}
}

func TestResolveHTTPSURL(t *testing.T) {
	src, err := source.Resolve("https://github.com/bxnlabs/argus.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsRemote() {
		t.Fatal("expected remote source")
	}
	if src.Host != "github.com" || src.Org != "bxnlabs" || src.Repo != "argus" {
		t.Errorf("unexpected parsed fields: host=%q org=%q repo=%q", src.Host, src.Org, src.Repo)
	}
}

func TestResolveHTTPSURLWithoutDotGit(t *testing.T) {
	src, err := source.Resolve("https://github.com/bxnlabs/argus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src.RemoteURL != "https://github.com/bxnlabs/argus.git" {
		t.Errorf("expected .git suffix appended, got %q", src.RemoteURL)
	}
}

func TestResolveSSHURL(t *testing.T) {
	src, err := source.Resolve("git@github.com:bxnlabs/argus.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !src.IsRemote() {
		t.Fatal("expected remote source")
	}
	if src.Host != "github.com" || src.Org != "bxnlabs" || src.Repo != "argus" {
		t.Errorf("unexpected parsed fields: host=%q org=%q repo=%q", src.Host, src.Org, src.Repo)
	}
	if src.RemoteURL != "https://github.com/bxnlabs/argus.git" {
		t.Errorf("unexpected RemoteURL %q", src.RemoteURL)
	}
}

func TestParentKeyLocal(t *testing.T) {
	src := &source.Source{LocalPath: "/Users/jeevb/repos/argus"}
	got := src.ParentKey()
	want := "--Users--jeevb--repos--argus"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestParentKeyRemote(t *testing.T) {
	src := &source.Source{
		RemoteURL: "https://github.com/bxnlabs/argus.git",
		Host:      "github.com",
		Org:       "bxnlabs",
		Repo:      "argus",
	}
	got := src.ParentKey()
	want := "github.com--bxnlabs--argus"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestResolveInvalidInput(t *testing.T) {
	cases := []string{
		"not-a-path-or-url",
		"git@missing-colon",
		"https://",
	}
	for _, tc := range cases {
		_, err := source.Resolve(tc)
		if err == nil {
			t.Errorf("expected error for input %q", tc)
		}
	}
}

// Ensure home dir is accessible (sanity check for tilde expansion).
func TestResolveTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	src, err := source.Resolve("~")
	if err != nil {
		t.Fatalf("unexpected error resolving ~: %v", err)
	}
	if src.LocalPath != home {
		t.Errorf("expected LocalPath %q, got %q", home, src.LocalPath)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/source/... -v
```

Expected: compile error — package does not exist yet.

**Step 3: Implement the source package**

Create `internal/source/source.go`:

```go
package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source represents a resolved session source: either a local directory
// that exists on disk, or a remote git repository.
type Source struct {
	// Exactly one of LocalPath or RemoteURL is non-empty.
	LocalPath string // absolute local dir path (exists on disk)
	RemoteURL string // normalized HTTPS git URL

	// Set only when RemoteURL is non-empty.
	Host string // e.g. "github.com"
	Org  string // e.g. "bxnlabs"
	Repo string // e.g. "argus"
}

// IsRemote reports whether this source is a remote git repo.
func (s *Source) IsRemote() bool { return s.RemoteURL != "" }

// ParentKey returns the directory name used under ~/.argus/projects/.
//
//   - Local:  "--" + abspath with "/" replaced by "--"
//     e.g. /Users/jeevb/repos/argus → --Users--jeevb--repos--argus
//   - Remote: "host--org--repo"
//     e.g. github.com/bxnlabs/argus → github.com--bxnlabs--argus
func (s *Source) ParentKey() string {
	if s.IsRemote() {
		return s.Host + "--" + s.Org + "--" + s.Repo
	}
	return "--" + strings.ReplaceAll(s.LocalPath, "/", "--")
}

// Resolve resolves input into a Source. It first checks whether input is an
// existing local directory; otherwise it attempts to parse it as a git URL
// or "org/repo" GitHub shorthand. Returns an error if neither interpretation
// is valid.
func Resolve(input string) (*Source, error) {
	expanded, err := expandTilde(input)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return nil, err
	}

	// Prefer local path if it exists as a directory.
	if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
		return &Source{LocalPath: abs}, nil
	}

	// Otherwise try as remote.
	return parseRemote(input)
}

func parseRemote(input string) (*Source, error) {
	// SSH: git@host:org/repo[.git]
	if strings.HasPrefix(input, "git@") {
		rest := strings.TrimPrefix(input, "git@")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		host := parts[0]
		orgRepo := strings.TrimSuffix(parts[1], ".git")
		pr := strings.SplitN(orgRepo, "/", 2)
		if len(pr) != 2 || pr[0] == "" || pr[1] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		return &Source{
			RemoteURL: "https://" + host + "/" + pr[0] + "/" + pr[1] + ".git",
			Host:      host,
			Org:       pr[0],
			Repo:      pr[1],
		}, nil
	}

	// HTTPS: https://host/org/repo[.git]
	if strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(input, "https://"), "http://")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.SplitN(trimmed, "/", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		return &Source{
			RemoteURL: "https://" + parts[0] + "/" + parts[1] + "/" + parts[2] + ".git",
			Host:      parts[0],
			Org:       parts[1],
			Repo:      parts[2],
		}, nil
	}

	// Shorthand: org/repo (implies github.com)
	parts := strings.SplitN(input, "/", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(parts[0], ".") {
		repo := strings.TrimSuffix(parts[1], ".git")
		return &Source{
			RemoteURL: "https://github.com/" + parts[0] + "/" + repo + ".git",
			Host:      "github.com",
			Org:       parts[0],
			Repo:      repo,
		}, nil
	}

	return nil, fmt.Errorf("not a valid path or git URL: %s", input)
}

func expandTilde(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[1:]), nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/source/... -v
```

Expected: all tests PASS.

**Step 5: Commit**

```bash
git add internal/source/
git commit -m "feat: add source resolver package (local path vs remote git URL)"
```

---

### Task 3: DB migration — add `worktree_branch` column

**Files:**
- Modify: `internal/agent/db/schema.go`
- Modify: `internal/agent/db/models.go`
- Modify: `internal/agent/db/sessions.go`
- Modify: `internal/agent/db/migrations.go`

**Step 1: Write the failing test**

Add to `internal/agent/db/` a new file `worktree_test.go`:

```go
package db_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/agent/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	if err := d.RunMigrations(); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return d
}

func TestWorktreeBranchColumn(t *testing.T) {
	d := openTestDB(t)

	branch := "jeev/fix-auth"
	s := &db.Session{
		ID:               "sess_test_1",
		Name:             "fix auth",
		TmuxName:         "claude-sess_test_1",
		WorkingDirectory: "/tmp/wt",
		AgentType:        "claude",
		WorktreeBranch:   &branch,
	}

	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := d.GetSession("sess_test_1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.WorktreeBranch == nil {
		t.Fatal("expected WorktreeBranch to be set")
	}
	if *got.WorktreeBranch != branch {
		t.Errorf("expected WorktreeBranch %q, got %q", branch, *got.WorktreeBranch)
	}
}

func TestWorktreeBranchNullable(t *testing.T) {
	d := openTestDB(t)

	s := &db.Session{
		ID:               "sess_test_2",
		Name:             "plain session",
		TmuxName:         "claude-sess_test_2",
		WorkingDirectory: "/tmp/plain",
		AgentType:        "claude",
	}

	if err := d.CreateSession(s); err != nil {
		t.Fatalf("create session: %v", err)
	}

	got, err := d.GetSession("sess_test_2")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.WorktreeBranch != nil {
		t.Errorf("expected nil WorktreeBranch, got %q", *got.WorktreeBranch)
	}
}

func TestMigrationsIdempotent(t *testing.T) {
	d := openTestDB(t)
	// Running migrations a second time should not error
	if err := d.RunMigrations(); err != nil {
		t.Fatalf("second RunMigrations failed: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/agent/db/... -v -run TestWorktree
```

Expected: compile error — `WorktreeBranch` field does not exist.

**Step 3: Update the schema**

In `internal/agent/db/schema.go`, add `worktree_branch TEXT` to the sessions table (after `auto_approve`):

```go
const schema = `
-- Sessions table
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  tmux_name TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  working_directory TEXT NOT NULL DEFAULT '~',
  provider_session_id TEXT,
  model TEXT DEFAULT 'sonnet',
  system_prompt TEXT,
  agent_type TEXT NOT NULL DEFAULT 'claude',
  auto_approve INTEGER NOT NULL DEFAULT 0,
  worktree_branch TEXT
);

-- Migrations tracking
CREATE TABLE IF NOT EXISTS _migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`
```

**Step 4: Update the Session model**

In `internal/agent/db/models.go`, add `WorktreeBranch` after `AutoApprove`:

```go
type Session struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	TmuxName          string  `json:"tmux_name"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
	WorkingDirectory  string  `json:"working_directory"`
	ProviderSessionID *string `json:"provider_session_id"`
	Model             *string `json:"model"`
	SystemPrompt      *string `json:"system_prompt"`
	AgentType         string  `json:"agent_type"`
	AutoApprove       bool    `json:"auto_approve"`
	WorktreeBranch    *string `json:"worktree_branch"`
}
```

**Step 5: Update sessions.go**

In `internal/agent/db/sessions.go`, update the three places that reference columns:

Replace `sessionColumns`:
```go
const sessionColumns = `id, name, tmux_name, created_at, updated_at,
	working_directory, provider_session_id, model, system_prompt,
	agent_type, auto_approve, worktree_branch`
```

Replace `scanSession` to scan the new field:
```go
func scanSession(row interface{ Scan(...any) error }) (*Session, error) {
	var s Session
	var autoApprove int
	err := row.Scan(
		&s.ID, &s.Name, &s.TmuxName, &s.CreatedAt, &s.UpdatedAt,
		&s.WorkingDirectory,
		&s.ProviderSessionID, &s.Model, &s.SystemPrompt,
		&s.AgentType, &autoApprove,
		&s.WorktreeBranch,
	)
	if err != nil {
		return nil, err
	}
	s.AutoApprove = autoApprove != 0
	return &s, nil
}
```

Replace `CreateSession` INSERT to include `worktree_branch`:
```go
func (d *DB) CreateSession(s *Session) error {
	autoApprove := 0
	if s.AutoApprove {
		autoApprove = 1
	}
	_, err := d.sql.Exec(
		`INSERT INTO sessions (id, name, tmux_name, working_directory, provider_session_id, model, system_prompt, agent_type, auto_approve, worktree_branch)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.TmuxName, s.WorkingDirectory,
		s.ProviderSessionID, s.Model, s.SystemPrompt,
		s.AgentType, autoApprove, s.WorktreeBranch,
	)
	if err != nil {
		return fmt.Errorf("create session %s: %w", s.ID, err)
	}
	return nil
}
```

**Step 6: Add migration for existing databases**

Replace `internal/agent/db/migrations.go`:

```go
package db

// RunMigrations runs any pending schema migrations.
func (d *DB) RunMigrations() error {
	return d.migrate("add_worktree_branch", func() error {
		_, err := d.sql.Exec(`ALTER TABLE sessions ADD COLUMN worktree_branch TEXT`)
		return err
	})
}

// migrate runs fn only if the named migration has not been applied.
func (d *DB) migrate(name string, fn func() error) error {
	var count int
	row := d.sql.QueryRow(`SELECT COUNT(*) FROM _migrations WHERE name = ?`, name)
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil // already applied
	}
	if err := fn(); err != nil {
		return err
	}
	_, err := d.sql.Exec(`INSERT INTO _migrations (name) VALUES (?)`, name)
	return err
}
```

**Step 7: Run tests to verify they pass**

```bash
go test ./internal/agent/db/... -v
```

Expected: all tests PASS (including new worktree tests).

**Step 8: Verify the whole project still builds**

```bash
go build ./...
```

Expected: no errors.

**Step 9: Commit**

```bash
git add internal/agent/db/
git commit -m "feat: add worktree_branch column to sessions table with migration"
```

---

### Task 4: Worktree package — slug generation and local git worktrees

**Files:**
- Create: `internal/worktree/slugify.go`
- Create: `internal/worktree/slugify_test.go`
- Create: `internal/worktree/manager.go`
- Create: `internal/worktree/manager_test.go`

**Step 1: Write the failing slug tests**

Create `internal/worktree/slugify_test.go`:

```go
package worktree

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Fix Auth Bug!", "fix-auth-bug"},
		{"  my feature  ", "my-feature"},
		{"already-valid", "already-valid"},
		{"123abc", "123abc"},
		{"UPPER CASE", "upper-case"},
		{"multiple   spaces", "multiple-spaces"},
		{"a--b", "a-b"},
		{"!!!!", "session"}, // all special → fallback
		{"", "session"},
	}
	for _, tc := range cases {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestWorktreeDirName(t *testing.T) {
	cases := []struct {
		branch string
		want   string
	}{
		{"jeev/fix-auth-bug", "jeev--fix-auth-bug"},
		{"fix-auth-bug", "fix-auth-bug"},
		{"prefix/org/feature", "prefix--org--feature"},
	}
	for _, tc := range cases {
		got := worktreeDirName(tc.branch)
		if got != tc.want {
			t.Errorf("worktreeDirName(%q) = %q, want %q", tc.branch, got, tc.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/worktree/... -v -run TestSlugify
```

Expected: compile error — package does not exist yet.

**Step 3: Implement slug utilities**

Create `internal/worktree/slugify.go`:

```go
package worktree

import (
	"regexp"
	"strings"
)

var nonAlphanumRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugify converts a session name into a valid git branch name component.
// It lowercases the name, collapses non-alphanumeric runs to "-", and
// trims leading/trailing "-". Returns "session" if the result is empty.
func slugify(name string) string {
	lower := strings.ToLower(name)
	slug := nonAlphanumRun.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "session"
	}
	return slug
}

// worktreeDirName converts a branch name to a safe directory name by
// replacing "/" with "--". e.g. "jeev/fix-auth" → "jeev--fix-auth".
func worktreeDirName(branch string) string {
	return strings.ReplaceAll(branch, "/", "--")
}
```

**Step 4: Run slug tests to verify they pass**

```bash
go test ./internal/worktree/... -v -run "TestSlugify|TestWorktreeDirName"
```

Expected: PASS.

**Step 5: Write the failing manager tests (local git)**

Create `internal/worktree/manager_test.go`:

```go
package worktree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/worktree"
)

// initGitRepo creates a temporary git repo with an initial commit.
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

	// Write a file and commit so HEAD is valid
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

	// Worktree directory must exist
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

	// Create first worktree
	_, branch1, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("first CreateForLocalRepo: %v", err)
	}
	if branch1 != "jeev/my-feature" {
		t.Errorf("expected first branch %q, got %q", "jeev/my-feature", branch1)
	}

	// Same session name → should get a -2 suffix
	_, branch2, err := mgr.CreateForLocalRepo(gitRoot, "my feature")
	if err != nil {
		t.Fatalf("second CreateForLocalRepo: %v", err)
	}
	if branch2 != "jeev/my-feature-2" {
		t.Errorf("expected second branch %q, got %q", "jeev/my-feature-2", branch2)
	}
}
```

**Step 6: Run tests to verify they fail**

```bash
go test ./internal/worktree/... -v -run TestCreateForLocal
```

Expected: compile error — `worktree.NewManager` and `CreateForLocalRepo` do not exist.

**Step 7: Implement the worktree manager (local git only)**

Create `internal/worktree/manager.go`:

```go
package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/source"
)

// Manager handles git worktree creation and remote repo cloning.
type Manager struct {
	stateDir string
	cfg      *config.Config
}

// NewManager creates a new worktree Manager.
// stateDir is the ~/.argus directory; cfg is the loaded user config.
func NewManager(stateDir string, cfg *config.Config) *Manager {
	return &Manager{stateDir: stateDir, cfg: cfg}
}

// CreateForLocalRepo creates an isolated git worktree for a local git repo.
// gitRoot must be the absolute path to the repo root (from git rev-parse --show-toplevel).
// Returns the absolute path of the created worktree and the git branch name.
func (m *Manager) CreateForLocalRepo(gitRoot, sessionName string) (worktreePath, branch string, err error) {
	src := &source.Source{LocalPath: gitRoot}
	return m.createWorktree(gitRoot, src.ParentKey(), sessionName)
}

// CreateForRemoteRepo clones (or fetches) the remote repo and creates a worktree.
// Returns the absolute path of the created worktree and the git branch name.
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string) (worktreePath, branch string, err error) {
	cloneDir := filepath.Join(m.stateDir, "projects", src.ParentKey(), "gitrepo")

	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0755); err != nil {
			return "", "", fmt.Errorf("create project dir: %w", err)
		}
		if err := runGit("", "clone", src.RemoteURL, cloneDir); err != nil {
			// Clean up partial clone on failure
			os.RemoveAll(cloneDir)
			return "", "", fmt.Errorf("clone repo: %w", err)
		}
	} else {
		// Repo already cloned: fetch + reset to default branch HEAD
		defaultBranch, err := getDefaultBranch(cloneDir)
		if err != nil {
			return "", "", err
		}
		if err := runGit(cloneDir, "fetch", "origin"); err != nil {
			return "", "", fmt.Errorf("fetch: %w", err)
		}
		if err := runGit(cloneDir, "checkout", defaultBranch); err != nil {
			return "", "", fmt.Errorf("checkout default branch: %w", err)
		}
		if err := runGit(cloneDir, "reset", "--hard", "origin/"+defaultBranch); err != nil {
			return "", "", fmt.Errorf("reset to origin: %w", err)
		}
	}

	return m.createWorktree(cloneDir, src.ParentKey(), sessionName)
}

// createWorktree is the shared implementation for local and remote worktree creation.
func (m *Manager) createWorktree(repoDir, parentKey, sessionName string) (worktreePath, branch string, err error) {
	slug := slugify(sessionName)
	baseBranch := m.branchName(slug)

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

// branchName builds the full branch name: "<prefix>/<slug>" or just "<slug>".
func (m *Manager) branchName(slug string) string {
	if m.cfg.BranchPrefix != "" {
		return m.cfg.BranchPrefix + "/" + slug
	}
	return slug
}

// uniqueBranch returns branch if it doesn't exist in the repo, or appends
// "-2", "-3", etc. until a unique name is found.
func (m *Manager) uniqueBranch(repoDir, branch string) (string, error) {
	exists, err := branchExists(repoDir, branch)
	if err != nil {
		return "", err
	}
	if !exists {
		return branch, nil
	}
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", branch, i)
		exists, err := branchExists(repoDir, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find unique branch name for %s", branch)
}

// branchExists reports whether the given branch name exists locally.
func branchExists(repoDir, branch string) (bool, error) {
	out, err := gitOutput(repoDir, "branch", "--list", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// getDefaultBranch returns the repo's default branch name.
// It tries: origin/HEAD symbolic ref → local "main" → local "master" → error.
func getDefaultBranch(repoDir string) (string, error) {
	// Try symbolic ref (works after clone)
	out, err := gitOutput(repoDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out) // refs/remotes/origin/main
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1], nil
		}
	}

	// Fall back to local branch existence
	for _, branch := range []string{"main", "master"} {
		exists, err := branchExists(repoDir, branch)
		if err == nil && exists {
			return branch, nil
		}
	}

	// Try remote branches
	for _, branch := range []string{"main", "master"} {
		out, err := gitOutput(repoDir, "branch", "-r", "--list", "origin/"+branch)
		if err == nil && strings.TrimSpace(out) != "" {
			return branch, nil
		}
	}

	return "", fmt.Errorf("cannot determine default branch for repo at %s", repoDir)
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

**Step 8: Run all worktree tests to verify they pass**

```bash
go test ./internal/worktree/... -v
```

Expected: all tests PASS.

**Step 9: Commit**

```bash
git add internal/worktree/
git commit -m "feat: add worktree package (slugify, local git worktree creation)"
```

---

### Task 5: Session lifecycle integration

**Files:**
- Modify: `internal/agent/session/lifecycle.go`
- Modify: `internal/agent/setup.go`
- Modify: `internal/agent/api/sessions.go`

**Context:** `CreateOptions` gains a `Source` field (replacing `WorkingDirectory` in the API). The `Manager` gains a `worktree.Manager`. During `Create`, the source is resolved and — if it's a git repo — a worktree is created. The `EnsureSession` path is unchanged (it uses `working_directory` from the DB, which is already the worktree path after initial creation).

**Step 1: Update `internal/agent/session/lifecycle.go`**

At the top of the file, add imports for the new packages:
```go
import (
    "fmt"
    "os/exec"
    "strings"
    "sync"

    "github.com/bxnlabs/argus/internal/agent/db"
    "github.com/bxnlabs/argus/internal/agent/provider"
    "github.com/bxnlabs/argus/internal/shared"
    "github.com/bxnlabs/argus/internal/source"
    "github.com/bxnlabs/argus/internal/worktree"
)
```

Replace `Manager` struct and `NewManager`:
```go
type Manager struct {
    db      *db.DB
    wt      *worktree.Manager
    mu      sync.Mutex
    sessLks map[string]*sync.Mutex
}

func NewManager(database *db.DB, wt *worktree.Manager) *Manager {
    return &Manager{db: database, wt: wt}
}
```

Replace `CreateOptions`:
```go
type CreateOptions struct {
    Name            string  `json:"name"`
    AgentType       string  `json:"agent_type"`
    Source          string  `json:"source"`
    Model           *string `json:"model,omitempty"`
    SystemPrompt    *string `json:"system_prompt,omitempty"`
    AutoApprove     bool    `json:"auto_approve"`
    ResumeSessionID string  `json:"resume_session_id,omitempty"`
}
```

In `Create`, replace the working directory resolution block (lines 66–76 in the original) with:

```go
    // Resolve source → working directory (and optional worktree branch)
    cwd, worktreeBranch, err := m.resolveSourceToCWD(opts.Source, opts.Name)
    if err != nil {
        return nil, fmt.Errorf("resolve source: %w", err)
    }
```

And update the session struct construction to include `WorktreeBranch`:
```go
    session := &db.Session{
        ID:                sessionID,
        Name:              opts.Name,
        TmuxName:          tmuxName,
        WorkingDirectory:  cwd,
        AgentType:         opts.AgentType,
        Model:             opts.Model,
        SystemPrompt:      opts.SystemPrompt,
        AutoApprove:       opts.AutoApprove,
        ProviderSessionID: providerSessionID,
        WorktreeBranch:    worktreeBranch,
    }
```

Add the helper method to the `Manager` type (outside `Create`, in the same file):
```go
// resolveSourceToCWD resolves a source string to a working directory path.
// If the source is or contains a git repo, it creates an isolated worktree
// and returns the worktree path and branch name.
// If source is empty, defaults to home directory.
func (m *Manager) resolveSourceToCWD(src, sessionName string) (cwd string, worktreeBranch *string, err error) {
    if src == "" {
        home, err := shared.ExpandPath("~")
        if err != nil {
            return "", nil, fmt.Errorf("expand home directory: %w", err)
        }
        return home, nil, nil
    }

    resolved, err := source.Resolve(src)
    if err != nil {
        return "", nil, err
    }

    if resolved.IsRemote() {
        wtPath, branch, err := m.wt.CreateForRemoteRepo(resolved, sessionName)
        if err != nil {
            return "", nil, err
        }
        return wtPath, &branch, nil
    }

    // Local path: check if it's inside a git repo.
    gitRoot, err := findGitRoot(resolved.LocalPath)
    if err != nil {
        // Not a git repo — use the path directly.
        return resolved.LocalPath, nil, nil
    }

    wtPath, branch, err := m.wt.CreateForLocalRepo(gitRoot, sessionName)
    if err != nil {
        return "", nil, err
    }
    return wtPath, &branch, nil
}

// findGitRoot returns the git root for the given directory, or an error if
// the directory is not inside a git repository.
func findGitRoot(dir string) (string, error) {
    cmd := exec.Command("git", "rev-parse", "--show-toplevel")
    cmd.Dir = dir
    out, err := cmd.Output()
    if err != nil {
        return "", err
    }
    return strings.TrimSpace(string(out)), nil
}
```

**Step 2: Update `internal/agent/setup.go`**

Replace the setup to wire in config and worktree manager:

```go
package agent

import (
    "fmt"
    "net/http"
    "os"
    "path/filepath"

    "github.com/bxnlabs/argus/internal/agent/api"
    "github.com/bxnlabs/argus/internal/agent/db"
    "github.com/bxnlabs/argus/internal/agent/session"
    "github.com/bxnlabs/argus/internal/agent/status"
    "github.com/bxnlabs/argus/internal/config"
    "github.com/bxnlabs/argus/internal/worktree"
)

type Config struct {
    DBPath  string
    Address string
}

func Setup(cfg Config) (http.Handler, func(), error) {
    database, err := db.Open(cfg.DBPath)
    if err != nil {
        return nil, nil, fmt.Errorf("open db: %w", err)
    }

    if err := database.RunMigrations(); err != nil {
        database.Close()
        return nil, nil, fmt.Errorf("migrations: %w", err)
    }

    // Determine state dir from DB path (parent of agent.db → ~/.argus)
    stateDir := filepath.Dir(cfg.DBPath)
    if stateDir == "." {
        home, err := os.UserHomeDir()
        if err != nil {
            database.Close()
            return nil, nil, fmt.Errorf("home dir: %w", err)
        }
        stateDir = filepath.Join(home, ".argus")
    }

    userCfg, _ := config.Load()
    wtMgr := worktree.NewManager(stateDir, userCfg)

    mgr := session.NewManager(database, wtMgr)
    detector := status.NewDetector()

    handler := api.NewRouter(api.Deps{
        SessionManager: mgr,
        StatusDetector: detector,
    })

    cleanup := func() { database.Close() }
    return handler, cleanup, nil
}
```

**Step 3: Update `internal/agent/api/sessions.go` create handler**

Replace the `working_directory` default with `source`:
```go
func (h *sessionHandler) create(w http.ResponseWriter, r *http.Request) {
    var opts agentsession.CreateOptions
    if err := parseBody(w, r, &opts); err != nil {
        respondError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    if opts.AgentType == "" {
        opts.AgentType = "claude"
    }
    if opts.Name == "" {
        opts.Name = "New Session"
    }
    // opts.Source may be empty (defaults to home dir in lifecycle)

    session, err := h.manager.Create(opts)
    if err != nil {
        respondInternalError(w, err)
        return
    }

    respondJSON(w, http.StatusCreated, map[string]any{"session": session})
}
```

**Step 4: Build and verify**

```bash
go build ./...
```

Expected: no errors.

**Step 5: Commit**

```bash
git add internal/agent/session/lifecycle.go internal/agent/setup.go internal/agent/api/sessions.go
git commit -m "feat: integrate source resolution and worktree creation into session lifecycle"
```

---

### Task 6: CLI changes — `--src`, `session pwd`, `session ls` BRANCH column

**Files:**
- Modify: `cmd/argus/cli/session_create.go`
- Modify: `cmd/argus/cli/resolve.go`
- Create: `cmd/argus/cli/session_pwd.go`
- Modify: `cmd/argus/cli/cli.go`
- Modify: `cmd/argus/cli/session_list.go`

**Step 1: Update `session_create.go` — rename `--dir` to `--src`**

Replace the entire file content:

```go
package cli

import (
    "bytes"
    "encoding/json"
    "fmt"
    "os"

    "github.com/spf13/cobra"
)

func newCreateCmd() *cobra.Command {
    var (
        provider string
        src      string
        yolo     bool
    )

    cmd := &cobra.Command{
        Use:   "new <name>",
        Short: "Create a new session and attach",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            cmd.SilenceUsage = true
            name := args[0]

            path, err := discoveryFilePath()
            if err != nil {
                return err
            }
            c, err := newClient(path)
            if err != nil {
                return err
            }

            reqBody := map[string]any{
                "name":         name,
                "agent_type":   provider,
                "auto_approve": yolo,
            }
            if src != "" {
                reqBody["source"] = src
            }

            data, err := json.Marshal(reqBody)
            if err != nil {
                return fmt.Errorf("marshal request: %w", err)
            }

            body, err := c.post("/api/sessions", bytes.NewReader(data))
            if err != nil {
                return err
            }

            var resp struct {
                Session sessionInfo `json:"session"`
            }
            if err := json.Unmarshal(body, &resp); err != nil {
                return fmt.Errorf("parse response: %w", err)
            }

            s := resp.Session
            fmt.Fprintf(os.Stderr, "Created session %q (%s)\n", s.Name, s.AgentType)

            return attachTmux(s.TmuxName)
        },
    }

    cmd.Flags().StringVar(&provider, "provider", "claude", "Agent type (claude, codex, gemini, shell)")
    cmd.Flags().StringVar(&src, "src", "", "Source: local path or git URL/shorthand (defaults to current directory)")
    cmd.Flags().BoolVar(&yolo, "yolo", false, "Enable auto-approve")

    return cmd
}
```

**Step 2: Add `WorktreeBranch` to `sessionInfo` in `resolve.go`**

Add `WorktreeBranch *string` to the `sessionInfo` struct:

```go
type sessionInfo struct {
    ID               string  `json:"id"`
    Name             string  `json:"name"`
    TmuxName         string  `json:"tmux_name"`
    CreatedAt        string  `json:"created_at"`
    UpdatedAt        string  `json:"updated_at"`
    WorkingDirectory string  `json:"working_directory"`
    AgentType        string  `json:"agent_type"`
    AutoApprove      bool    `json:"auto_approve"`
    Model            *string `json:"model"`
    WorktreeBranch   *string `json:"worktree_branch"`
}
```

**Step 3: Create `session_pwd.go`**

```go
package cli

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newPwdCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "pwd <name-or-id>",
        Short: "Print the working directory of a session",
        Long: `Print the working directory of a session to stdout.

Useful for shell integration:

  acd() { cd "$(argus session pwd "$1")"; }`,
        Args: cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            cmd.SilenceUsage = true
            path, err := discoveryFilePath()
            if err != nil {
                return err
            }
            c, err := newClient(path)
            if err != nil {
                return err
            }
            s, err := fetchAndResolve(c, args[0])
            if err != nil {
                return err
            }
            fmt.Println(s.WorkingDirectory)
            return nil
        },
    }
}
```

**Step 4: Register `pwd` in `cli.go`**

In `cli.go`, add `newPwdCmd()` to the `AddCommand` call:

```go
cmd.AddCommand(
    newListCmd(),
    newCreateCmd(),
    newAttachCmd(),
    newDeleteCmd(),
    newRenameCmd(),
    newPwdCmd(),
)
```

**Step 5: Add BRANCH column to `session_list.go`**

Replace the header and row formatting in `newListCmd`:

```go
fmt.Fprintln(w, "ID\tNAME\tSTATUS\tPROVIDER\tBRANCH\tUPDATED")
for _, s := range resp.Sessions {
    st := statuses[s.ID]
    if st == "" {
        st = "-"
    }
    branch := ""
    if s.WorktreeBranch != nil {
        branch = *s.WorktreeBranch
    }
    fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
        s.ID, s.Name, st, s.AgentType, branch, relativeTime(s.UpdatedAt))
}
```

**Step 6: Build and verify**

```bash
go build ./...
```

Expected: no errors.

**Step 7: Commit**

```bash
git add cmd/argus/cli/
git commit -m "feat: rename --dir to --src, add session pwd command, add BRANCH column to ls"
```

---

### Task 7: UI — types and NewSessionDialog Local/Remote tabs

**Files:**
- Modify: `web/src/types.ts`
- Modify: `web/src/components/NewSessionDialog/index.tsx`

**Step 1: Update `web/src/types.ts`**

In `CreateSessionParams`, rename `working_directory` to `source`:

```typescript
export interface CreateSessionParams {
  name?: string;
  agent_type: AgentType;
  source?: string;
  auto_approve?: boolean;
}
```

In `Session`, add `worktree_branch`:

```typescript
export interface Session {
  id: string;
  name: string;
  tmux_name: string;
  created_at: string;
  updated_at: string;
  working_directory: string;
  provider_session_id: string | null;
  model: string | null;
  system_prompt: string | null;
  agent_type: AgentType;
  auto_approve: boolean;
  worktree_branch: string | null;
}
```

**Step 2: Update `NewSessionDialog/index.tsx`**

Replace the entire file content:

```tsx
import { useState, useEffect, useRef } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { AgentSelector } from "./AgentSelector";
import { DirectoryPicker } from "@/components/DirectoryPicker";
import { FolderOpen } from "lucide-react";
import type { AgentType, CreateSessionParams } from "@/types";

type SourceTab = "local" | "remote";

interface NewSessionDialogProps {
  open: boolean;
  onClose: () => void;
  onCreateSession: (params: CreateSessionParams) => void;
}

export function NewSessionDialog({
  open,
  onClose,
  onCreateSession,
}: NewSessionDialogProps) {
  const [name, setName] = useState("");
  const [agentType, setAgentType] = useState<AgentType>("claude");
  const [sourceTab, setSourceTab] = useState<SourceTab>("local");
  const [localDir, setLocalDir] = useState("");
  const [remoteRepo, setRemoteRepo] = useState("");
  const [autoApprove, setAutoApprove] = useState(true);
  const [showDirectoryPicker, setShowDirectoryPicker] = useState(false);
  const directoryPickerClosingRef = useRef(false);

  useEffect(() => {
    if (open) {
      setName("");
      setAgentType("claude");
      setSourceTab("local");
      setLocalDir("");
      setRemoteRepo("");
      setAutoApprove(true);
      setShowDirectoryPicker(false);
    }
  }, [open]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const params: CreateSessionParams = {
      agent_type: agentType,
    };

    if (name.trim()) {
      params.name = name.trim();
    }

    const sourceValue =
      sourceTab === "local" ? localDir.trim() : remoteRepo.trim();
    if (sourceValue) {
      params.source = sourceValue;
    }

    if (autoApprove) {
      params.auto_approve = true;
    }

    onCreateSession(params);
    onClose();
  };

  return (
    <>
      <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
        <DialogContent
          className="top-[env(safe-area-inset-top)] translate-y-0 max-h-[85vh] overflow-y-auto sm:top-[50%] sm:translate-y-[-50%]"
          onPointerDownOutside={(e) => {
            if (directoryPickerClosingRef.current) e.preventDefault();
          }}
          onFocusOutside={(e) => {
            if (directoryPickerClosingRef.current) {
              e.preventDefault();
              directoryPickerClosingRef.current = false;
            }
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && e.shiftKey) {
              e.preventDefault();
              handleSubmit(e as unknown as React.FormEvent);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>New Session</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <AgentSelector value={agentType} onChange={setAgentType} />

            <div className="space-y-2">
              <label className="text-sm font-medium">
                Name{" "}
                <span className="text-muted-foreground font-normal">
                  (optional)
                </span>
              </label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Auto-generated if empty"
                autoFocus
              />
            </div>

            <div className="space-y-2">
              {/* Tab switcher */}
              <div className="flex gap-1 rounded-md bg-muted p-1 w-fit">
                <button
                  type="button"
                  onClick={() => setSourceTab("local")}
                  className={`px-3 py-1 text-sm rounded-sm transition-colors ${
                    sourceTab === "local"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  Local
                </button>
                <button
                  type="button"
                  onClick={() => setSourceTab("remote")}
                  className={`px-3 py-1 text-sm rounded-sm transition-colors ${
                    sourceTab === "remote"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  Remote
                </button>
              </div>

              {sourceTab === "local" ? (
                <div>
                  <label className="text-sm font-medium">Directory</label>
                  <div className="flex gap-2 mt-1">
                    <Input
                      value={localDir}
                      onChange={(e) => setLocalDir(e.target.value)}
                      placeholder="~/projects/my-app"
                      className="flex-1"
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      onClick={() => setShowDirectoryPicker(true)}
                      aria-label="Browse folders"
                    >
                      <FolderOpen className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              ) : (
                <div>
                  <label className="text-sm font-medium">Repository</label>
                  <Input
                    value={remoteRepo}
                    onChange={(e) => setRemoteRepo(e.target.value)}
                    placeholder="org/repo  or  https://github.com/org/repo.git"
                    className="mt-1"
                  />
                </div>
              )}
            </div>

            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <label className="text-sm font-medium">Auto-approve</label>
                <p className="text-muted-foreground text-xs">
                  Skip permission prompts for tool calls
                </p>
              </div>
              <Switch
                checked={autoApprove}
                onCheckedChange={setAutoApprove}
              />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit">Create</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <DirectoryPicker
        open={showDirectoryPicker}
        onOpenChange={(o) => {
          if (!o) directoryPickerClosingRef.current = true;
          setShowDirectoryPicker(o);
        }}
        onSelect={(path) => setLocalDir(path)}
        initialPath={localDir}
      />
    </>
  );
}
```

**Step 3: Check for any other usages of `working_directory` in `CreateSessionParams`**

```bash
grep -r "working_directory" web/src/
```

If any hits outside of `types.ts` and `NewSessionDialog`, update them to use `source`.

**Step 4: Build the frontend**

```bash
cd web && npm run build
```

Expected: no TypeScript errors.

**Step 5: Commit**

```bash
git add web/src/types.ts web/src/components/NewSessionDialog/index.tsx
git commit -m "feat: update UI with Local/Remote tab switcher and source field"
```

---

### Task 8: End-to-end build and smoke test

**Step 1: Build everything**

```bash
go build ./... && cd web && npm run build && cd ..
```

Expected: no errors.

**Step 2: Run all Go tests**

```bash
go test ./...
```

Expected: all tests PASS.

**Step 3: Manual smoke test — local git session**

```bash
# Start the agent
./bin/argus &

# Create a session from the current repo (which is a git repo)
argus session new "test worktree" --src .

# List sessions — BRANCH column should show the created branch
argus session ls

# Verify worktree was created
ls ~/.argus/projects/

# Print working directory
argus session pwd "test worktree"
```

Expected: session is created in an isolated worktree branch; `session ls` shows the branch; `session pwd` prints the worktree path.

**Step 4: Manual smoke test — plain directory session**

```bash
# Create a session in a non-git directory
argus session new "plain session" --src /tmp

# BRANCH column should be empty
argus session ls
```

Expected: plain directory session works as before; no worktree created; BRANCH column is empty.

**Step 5: Final commit**

```bash
git add -A
git commit -m "chore: final build verification for worktree and git repo support"
```
