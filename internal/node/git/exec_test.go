package git

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// initTestRepo creates a git repo with one commit for testing.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	commands := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s: %s", args, err, out)
		}
	}
	return dir
}

// commitFile creates a file and commits it in the test repo.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	if err := writeTestFile(dir, name, content); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", name},
		{"git", "commit", "-m", message},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s: %s", args, err, out)
		}
	}
}

func writeTestFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	return writeFile(path, content)
}

func TestParseStatus(t *testing.T) {
	tests := []struct {
		char byte
		want FileStatus
	}{
		{'M', StatusModified},
		{'A', StatusAdded},
		{'D', StatusDeleted},
		{'R', StatusRenamed},
		{'C', StatusCopied},
		{'U', StatusUnmerged},
		{'?', StatusModified}, // fallback
	}
	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			if got := parseStatus(tt.char); got != tt.want {
				t.Errorf("parseStatus(%q) = %q, want %q", tt.char, got, tt.want)
			}
		})
	}
}

func TestValidateHash(t *testing.T) {
	valid := []string{
		"abcdef1",                                 // 7 chars
		"abcdef1234567890abcdef1234567890abcdef12", // 40 chars
		"1234567890abcdef1234",                     // 20 chars
	}
	for _, h := range valid {
		if err := validateHash(h); err != nil {
			t.Errorf("validateHash(%q) = %v, want nil", h, err)
		}
	}

	invalid := []string{
		"abc",               // too short
		"ABCDEF1",           // uppercase
		"abcdefg",           // non-hex
		"abc123; rm -rf /",  // injection
		"$(whoami)",         // injection
		"abc|grep",          // injection
		"",                  // empty
	}
	for _, h := range invalid {
		if err := validateHash(h); err == nil {
			t.Errorf("validateHash(%q) = nil, want error", h)
		}
	}

	t.Run("invalid hash wraps ErrInvalidInput", func(t *testing.T) {
		err := validateHash("xyz")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("valid hash does not produce error", func(t *testing.T) {
		if err := validateHash("abcdef1"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestRelativeTime(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name string
		ts   int64
		want string
	}{
		{"just now", now, "just now"},
		{"5 minutes", now - 300, "5m ago"},
		{"2 hours", now - 7200, "2h ago"},
		{"3 days", now - 259200, "3d ago"},
		{"2 weeks", now - 1209600, "2w ago"},
		{"3 months", now - 7776000, "3mo ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := relativeTime(tt.ts); got != tt.want {
				t.Errorf("relativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRunGit(t *testing.T) {
	dir := initTestRepo(t)

	t.Run("successful command", func(t *testing.T) {
		ctx := context.Background()
		out, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--git-dir")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out == "" {
			t.Error("expected output, got empty")
		}
	})

	t.Run("non-git directory", func(t *testing.T) {
		ctx := context.Background()
		_, err := runGit(ctx, t.TempDir(), defaultMaxBuffer, "status")
		if err == nil {
			t.Error("expected error for non-git directory")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond) // ensure deadline passed
		_, err := runGit(ctx, dir, defaultMaxBuffer, "status")
		if err == nil {
			t.Error("expected timeout error")
		}
	})
}
