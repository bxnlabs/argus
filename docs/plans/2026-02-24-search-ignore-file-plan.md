# Search Ignore File Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace hardcoded fd exclude patterns with a user-editable `~/.argus/search-ignore` file passed to fd via `--ignore-file`.

**Architecture:** New `ensureIgnoreFile()` function creates the default ignore file on first use. `buildFdArgs()` drops `--exclude` flags and adds `--ignore-file <path>` instead. `Search()` resolves $HOME and passes the ignore file path through.

**Tech Stack:** Go, fd (CLI), gitignore glob syntax

---

### Task 1: Add `ensureIgnoreFile` function and tests

**Files:**
- Modify: `internal/agent/filesearch/operations.go`
- Modify: `internal/agent/filesearch/operations_test.go`

**Step 1: Write the failing test for `ensureIgnoreFile`**

Add to the bottom of `operations_test.go`:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureIgnoreFile(t *testing.T) {
	t.Run("creates default file when missing", func(t *testing.T) {
		home := t.TempDir()
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".argus", "search-ignore")
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		if !strings.Contains(content, ".git/") {
			t.Error("default file should contain .git/")
		}
		if !strings.Contains(content, "Library/") {
			t.Error("default file should contain Library/")
		}
		if !strings.Contains(content, "node_modules/") {
			t.Error("default file should contain node_modules/")
		}
	})

	t.Run("preserves existing file", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".argus")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		custom := "custom-pattern/\n"
		if err := os.WriteFile(filepath.Join(dir, "search-ignore"), []byte(custom), 0o600); err != nil {
			t.Fatal(err)
		}
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != custom {
			t.Errorf("existing file was overwritten: got %q, want %q", string(data), custom)
		}
		_ = path
	})
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/filesearch/ -run TestEnsureIgnoreFile -v`
Expected: FAIL — `ensureIgnoreFile` is undefined.

**Step 3: Write `ensureIgnoreFile` in `operations.go`**

Add below the existing imports (add `"os"` to the import block):

```go
const ignoreFileName = "search-ignore"

// defaultIgnoreContents is written to ~/.argus/search-ignore on first use.
var defaultIgnoreContents = strings.TrimSpace(`
# Argus search ignore patterns (gitignore syntax)
# Edit this file to control which directories fd skips during search.
# See: https://git-scm.com/docs/gitignore#_pattern_format

# Version control
.git/

# Package managers & toolchains
node_modules/
.npm/
.nvm/
.cargo/
.rustup/

# IDE & editor state
.vscode/
.cursor/
.claude/

# Caches & local data
.cache/
.local/
.config/

# macOS
Library/
`) + "\n"

// ensureIgnoreFile returns the path to ~/.argus/search-ignore, creating it
// with sensible defaults if it doesn't already exist.
func ensureIgnoreFile(home string) (string, error) {
	dir := filepath.Join(home, ".argus")
	path := filepath.Join(dir, ignoreFileName)

	if _, err := os.Stat(path); err == nil {
		return path, nil // already exists
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(defaultIgnoreContents), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return path, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/filesearch/ -run TestEnsureIgnoreFile -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/agent/filesearch/operations.go internal/agent/filesearch/operations_test.go
git commit -m "feat(filesearch): add ensureIgnoreFile for ~/.argus/search-ignore"
```

---

### Task 2: Wire ignore file into `buildFdArgs` and `Search`

**Files:**
- Modify: `internal/agent/filesearch/operations.go`
- Modify: `internal/agent/filesearch/operations_test.go`

**Step 1: Write failing tests for new `buildFdArgs` signature**

Replace `TestBuildFdArgs_Hidden` in `operations_test.go` with:

```go
func TestBuildFdArgs_IgnoreFile(t *testing.T) {
	args := buildFdArgs("test", "", 20, "/tmp/fake-ignore")

	hasIgnoreFile := false
	for i, a := range args {
		if a == "--ignore-file" && i+1 < len(args) && args[i+1] == "/tmp/fake-ignore" {
			hasIgnoreFile = true
		}
	}
	if !hasIgnoreFile {
		t.Error("expected --ignore-file /tmp/fake-ignore")
	}

	// Verify no --exclude flags remain
	for _, a := range args {
		if a == "--exclude" {
			t.Error("should not have --exclude flags when using --ignore-file")
		}
	}
}
```

Update the other `TestBuildFdArgs_*` test calls to pass the new 4th argument:

```go
func TestBuildFdArgs_Directory(t *testing.T) {
	args := buildFdArgs("test", "directory", 20, "/tmp/ignore")
	// ... rest unchanged
}

func TestBuildFdArgs_File(t *testing.T) {
	args := buildFdArgs("test", "file", 20, "/tmp/ignore")
	// ... rest unchanged
}

func TestBuildFdArgs_Both(t *testing.T) {
	args := buildFdArgs("test", "", 20, "/tmp/ignore")
	// ... rest unchanged
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/filesearch/ -run TestBuildFdArgs -v`
Expected: FAIL — `buildFdArgs` takes 3 args, not 4.

**Step 3: Update `buildFdArgs` and `Search` in `operations.go`**

Remove the `hiddenExcludes` var entirely.

Update `buildFdArgs` signature to accept `ignoreFile string`:

```go
func buildFdArgs(query, searchType string, limit int, ignoreFile string) []string {
	args := []string{
		"-i",
		"--hidden",
		"--max-depth", fmt.Sprintf("%d", maxDepth),
		"--max-results", fmt.Sprintf("%d", limit*overFetchFactor),
		"--absolute-path",
		"--ignore-file", ignoreFile,
	}

	switch searchType {
	case "directory":
		args = append(args, "-t", "d")
	case "file":
		args = append(args, "-t", "f")
	}

	args = append(args, query)
	return args
}
```

Update `Search` to resolve home and call `ensureIgnoreFile`:

```go
func Search(searchDir, query, searchType string, limit int) (*FileSearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	ignoreFile, err := ensureIgnoreFile(home)
	if err != nil {
		return nil, fmt.Errorf("ignore file: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), fdTimeout)
	defer cancel()

	args := buildFdArgs(query, searchType, limit, ignoreFile)
	output, err := runFd(ctx, searchDir, maxOutputBuffer, args...)
	if err != nil {
		return nil, err
	}

	results := parseOutput(output, query, searchType, limit)

	return &FileSearchResponse{
		Results: results,
		Query:   query,
		Count:   len(results),
	}, nil
}
```

**Step 4: Run all tests to verify they pass**

Run: `go test ./internal/agent/filesearch/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/agent/filesearch/operations.go internal/agent/filesearch/operations_test.go
git commit -m "feat(filesearch): replace hardcoded excludes with --ignore-file

Remove hiddenExcludes and the --exclude loop from buildFdArgs.
Instead, pass --ignore-file pointing to ~/.argus/search-ignore.
Search() now calls ensureIgnoreFile() to create the default file
on first use."
```

---

### Task 3: Manual verification

**Step 1: Delete any existing ignore file to test auto-creation**

Run: `rm -f ~/.argus/search-ignore`

**Step 2: Build and run the server**

Run: `make build && ./bin/argus`

**Step 3: Hit the search endpoint**

Run: `curl -s 'http://localhost:3000/agent/api/files/search?q=argus&type=directory' | jq .`

Expected:
- HTTP 200 with results
- `~/.argus/search-ignore` now exists with default contents

**Step 4: Verify the ignore file was created**

Run: `cat ~/.argus/search-ignore`

Expected: Default contents matching the design doc.

**Step 5: Verify customization works**

Add a custom pattern to `~/.argus/search-ignore` (e.g., `Workspace/`) and re-run the search to confirm the custom exclude takes effect.

**Step 6: Commit (nothing to commit — verification only)**
