# Tmux Status Bar Cleanup Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the tmux status bar's clock and opaque tmux session name with the Argus session ID, git branch, and working directory.

**Architecture:** Move `compressPath` from `cli` to `shared` package for reuse. Expand `ConfigureSession` to accept session ID, dir, and branch. Use `compressPath` for directory display and `truncateRight` for branch. Update both lifecycle.go callers.

**Tech Stack:** Go, tmux set-option

**Layout (git session):**
```
 Argus |          sess_m2abc12_xyz789 | main | ~/Workspace/.../bxnlabs/argus
```

**Layout (non-git session):**
```
 Argus |                    sess_m2abc12_xyz789 | ~/Workspace/.../bxnlabs/argus
```

---

### Task 1: Move `compressPath` from cli to shared package

**Files:**
- Create: `internal/shared/pathutil.go`
- Create: `internal/shared/pathutil_test.go`
- Modify: `cmd/argus/cli/pathutil.go`
- Modify: `cmd/argus/cli/pathutil_test.go`

**Step 1: Create `internal/shared/pathutil.go`**

Move the `compressPath` function from `cmd/argus/cli/pathutil.go` into
the `shared` package, exporting it as `CompressPath`. The logic is identical.

```go
package shared

import "strings"

// CompressPath shortens a path for display:
// 1. Replace home prefix with ~
// 2. If longer than threshold, keep first + last 2 segments: ~/first/.../last2
// 3. If still too long, drop first segment: ~/.../last2
func CompressPath(path, home string, threshold int) string {
	// Step 1: tilde-shorten
	display := path
	if home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			display = "~" + path[len(home):]
		}
	}

	// Step 2: compress if over threshold
	if len(display) <= threshold {
		return display
	}

	// Split into prefix (~ or empty) and segments
	var prefix string
	rest := display
	if strings.HasPrefix(display, "~/") {
		prefix = "~"
		rest = display[1:] // "/Workspace/repos/..."
	}

	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) <= 3 {
		return display
	}

	// Keep first segment + last 2 segments
	first := segments[0]
	tail := segments[len(segments)-2:]
	result := prefix + "/" + first + "/.../" + tail[0] + "/" + tail[1]

	// Step 3: if still too long, drop the first segment
	if len(result) > threshold {
		result = prefix + "/.../" + tail[0] + "/" + tail[1]
	}

	return result
}
```

**Step 2: Create `internal/shared/pathutil_test.go`**

Move the tests from `cmd/argus/cli/pathutil_test.go`, updating the package
and function name. The test cases are identical.

```go
package shared

import "testing"

func TestCompressPath(t *testing.T) {
	home := "/Users/jeevb"
	tests := []struct {
		name      string
		path      string
		home      string
		threshold int
		want      string
	}{
		{
			name:      "short path unchanged",
			path:      "/tmp/project",
			home:      home,
			threshold: 40,
			want:      "/tmp/project",
		},
		{
			name:      "tilde replaces home prefix",
			path:      "/Users/jeevb/project",
			home:      home,
			threshold: 40,
			want:      "~/project",
		},
		{
			name:      "home dir itself",
			path:      "/Users/jeevb",
			home:      home,
			threshold: 40,
			want:      "~",
		},
		{
			name:      "long path compressed",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			home:      home,
			threshold: 30,
			want:      "~/Workspace/.../bxnlabs/argus",
		},
		{
			name:      "non-home long path compressed",
			path:      "/opt/data/very/deep/nested/project",
			home:      home,
			threshold: 25,
			want:      "/opt/.../nested/project",
		},
		{
			name:      "second stage drops first segment",
			path:      "/opt/data/very/deep/nested/project",
			home:      home,
			threshold: 20,
			want:      "/.../nested/project",
		},
		{
			name:      "second stage with tilde prefix",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/very-long-project-name",
			home:      home,
			threshold: 20,
			want:      "~/.../bxnlabs/very-long-project-name",
		},
		{
			name:      "three segments no compression needed",
			path:      "/Users/jeevb/project",
			home:      home,
			threshold: 10,
			want:      "~/project",
		},
		{
			name:      "exactly at threshold no compression",
			path:      "/Users/jeevb/short",
			home:      home,
			threshold: 7,
			want:      "~/short",
		},
		{
			name:      "empty home falls back",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			home:      "",
			threshold: 30,
			want:      "/Users/.../bxnlabs/argus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompressPath(tt.path, tt.home, tt.threshold)
			if got != tt.want {
				t.Errorf("CompressPath(%q, %q, %d) = %q, want %q",
					tt.path, tt.home, tt.threshold, got, tt.want)
			}
		})
	}
}
```

**Step 3: Update CLI to use shared.CompressPath**

Replace `cmd/argus/cli/pathutil.go` contents — delegate to shared:

```go
package cli

import "github.com/bxnlabs/argus/internal/shared"

// compressPath delegates to the shared implementation.
func compressPath(path, home string, threshold int) string {
	return shared.CompressPath(path, home, threshold)
}
```

Replace `cmd/argus/cli/pathutil_test.go` — keep a smoke test that confirms delegation:

```go
package cli

import "testing"

func TestCompressPath(t *testing.T) {
	// Full test coverage in internal/shared/pathutil_test.go.
	// Smoke test to verify delegation works.
	got := compressPath("/Users/jeevb/Workspace/repos/bxnlabs/argus", "/Users/jeevb", 30)
	want := "~/Workspace/.../bxnlabs/argus"
	if got != want {
		t.Errorf("compressPath() = %q, want %q", got, want)
	}
}
```

**Step 4: Run tests to verify**

Run: `go test ./internal/shared/ ./cmd/argus/cli/ -run TestCompressPath -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/shared/pathutil.go internal/shared/pathutil_test.go \
       cmd/argus/cli/pathutil.go cmd/argus/cli/pathutil_test.go
git commit -m "refactor: move compressPath to shared package for reuse"
```

---

### Task 2: Add `truncateRight` helper and tests

**Files:**
- Modify: `internal/shared/pathutil.go`
- Modify: `internal/shared/pathutil_test.go`

**Step 1: Write failing test for `truncateRight`**

Append to `internal/shared/pathutil_test.go`:

```go
func TestTruncateRight(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"within limit", "main", 20, "main"},
		{"at limit", "abcde", 5, "abcde"},
		{"over limit", "abcdefghij", 5, "abcd…"},
		{"max 1", "abcde", 1, "…"},
		{"empty", "", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateRight(tt.s, tt.max); got != tt.want {
				t.Errorf("TruncateRight(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/shared/ -run TestTruncateRight -v`
Expected: FAIL — function not defined

**Step 3: Implement `TruncateRight`**

Append to `internal/shared/pathutil.go`:

```go
// TruncateRight right-truncates s to max runes, suffixing with "…" if truncated.
func TruncateRight(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/shared/ -run TestTruncateRight -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/shared/pathutil.go internal/shared/pathutil_test.go
git commit -m "feat: add TruncateRight helper to shared package"
```

---

### Task 3: Add `buildStatusRight` test

**Files:**
- Modify: `internal/agent/session/tmux_test.go`

**Step 1: Write failing test for `buildStatusRight`**

This function composes the tmux status-right string from session ID, dir, and branch.

```go
func TestBuildStatusRight(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		dir        string
		branch     string
		home       string
		wantParts  []string // substrings that must all appear
		wantAbsent []string // substrings that must NOT appear
	}{
		{
			name:      "git session with all fields",
			sessionID: "sess_m2abc12_xyz789",
			dir:       "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			branch:    "main",
			home:      "/Users/jeevb",
			wantParts: []string{"sess_m2abc12_xyz789", "main", "bxnlabs/argus"},
		},
		{
			name:       "non-git session omits branch segment",
			sessionID:  "sess_m2abc12_xyz789",
			dir:        "/Users/jeevb/projects/myapp",
			branch:     "",
			home:       "/Users/jeevb",
			wantParts:  []string{"sess_m2abc12_xyz789", "~/projects/myapp"},
			wantAbsent: []string{"#[fg=#cba6f7]"}, // branch color absent
		},
		{
			name:      "long branch is truncated",
			sessionID: "sess_abc",
			dir:       "/tmp",
			branch:    "feat/some-really-long-branch-name",
			home:      "/Users/jeevb",
			wantParts: []string{"feat/some-really-lon…"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildStatusRight(tt.sessionID, tt.dir, tt.branch, tt.home)
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("buildStatusRight() = %q, want it to contain %q", got, part)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("buildStatusRight() = %q, should NOT contain %q", got, absent)
				}
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/session/ -run TestBuildStatusRight -v`
Expected: FAIL — function not defined

**Step 3: Commit**

```bash
git add internal/agent/session/tmux_test.go
git commit -m "test: add buildStatusRight test for tmux status bar"
```

---

### Task 4: Implement `buildStatusRight` and update `ConfigureSession`

**Files:**
- Modify: `internal/agent/session/tmux.go`

**Step 1: Add `buildStatusRight` and update `ConfigureSession` signature**

Add the `shared` import to tmux.go:

```go
import (
	...
	"github.com/bxnlabs/argus/internal/shared"
)
```

Add constants and `buildStatusRight`:

```go
const (
	maxDirWidth    = 30
	maxBranchWidth = 20
)

// buildStatusRight formats the right side of the tmux status bar.
// Layout with branch:    "{sessionID} | {branch} | {dir} "
// Layout without branch: "{sessionID} | {dir} "
func buildStatusRight(sessionID, dir, branch, home string) string {
	displayDir := shared.CompressPath(dir, home, maxDirWidth)
	displayID := sessionID

	if branch == "" {
		return fmt.Sprintf("#[fg=#a6adc8]%s #[fg=#6c7086]| #[fg=#89b4fa]%s ", displayID, displayDir)
	}
	displayBranch := shared.TruncateRight(branch, maxBranchWidth)
	return fmt.Sprintf("#[fg=#a6adc8]%s #[fg=#6c7086]| #[fg=#cba6f7] %s #[fg=#6c7086]| #[fg=#89b4fa]%s ", displayID, displayBranch, displayDir)
}
```

Update `ConfigureSession` — new signature adds sessionID, dir, branch, home:

```go
// ConfigureSession applies the standard Argus tmux status bar styling to a session.
func ConfigureSession(name, sessionID, dir, branch, home string) {
	statusRight := buildStatusRight(sessionID, dir, branch, home)
	options := []struct{ key, val string }{
		{"status-style", "bg=#1e1e2e,fg=#cdd6f4"},
		{"status-left", "#[fg=#cba6f7,bold] Argus #[fg=#6c7086]| "},
		{"status-left-length", "20"},
		{"status-right", statusRight},
		{"status-right-length", "80"},
		{"status-position", "bottom"},
		{"mouse", "on"},
	}
	for _, o := range options {
		if err := exec.Command("tmux", "set-option", "-t", name, o.key, o.val).Run(); err != nil {
			log.Printf("tmux set-option %s: %v", o.key, err)
		}
	}
}
```

Note: `status-right-length` increased to 80 to accommodate session ID + branch + dir + dividers.

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/agent/session/ -run TestBuildStatusRight -v`
Expected: PASS

**Step 3: Verify compilation fails (callers need updating)**

Run: `go build ./...`
Expected: FAIL — `ConfigureSession` call sites pass wrong number of arguments

**Step 4: Commit**

```bash
git add internal/agent/session/tmux.go
git commit -m "feat: implement buildStatusRight, update ConfigureSession signature"
```

---

### Task 5: Update callers in lifecycle.go

**Files:**
- Modify: `internal/agent/session/lifecycle.go`

**Step 1: Update `Create` call site (line ~128)**

The variables `cwd`, `gitParentDir`, `worktreeBranch`, and `sessionID` are
already in scope. We need to resolve `home` via `os.UserHomeDir()`.

Before:
```go
ConfigureSession(tmuxName)
```

After:
```go
configDir := cwd
if gitParentDir != nil {
	configDir = *gitParentDir
}
configBranch := ""
if worktreeBranch != nil {
	configBranch = *worktreeBranch
}
home, _ := os.UserHomeDir()
ConfigureSession(tmuxName, sessionID, configDir, configBranch, home)
```

**Step 2: Update `EnsureSession` call site (line ~391)**

Before:
```go
ConfigureSession(tmuxName)
```

After — extract from the `session` DB record:
```go
configDir := session.WorkingDirectory
if session.GitParentDir != nil {
	configDir = *session.GitParentDir
}
configBranch := ""
if session.WorktreeBranch != nil {
	configBranch = *session.WorktreeBranch
}
home, _ := os.UserHomeDir()
ConfigureSession(tmuxName, session.ID, configDir, configBranch, home)
```

**Step 3: Verify full build passes**

Run: `go build ./...`
Expected: PASS

**Step 4: Run all session tests**

Run: `go test ./internal/agent/session/ -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/session/lifecycle.go
git commit -m "feat: pass session ID, dir, branch to ConfigureSession"
```

---

### Task 6: Full verification

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: PASS

**Step 2: Build binary**

Run: `go build ./cmd/argus/`
Expected: PASS
