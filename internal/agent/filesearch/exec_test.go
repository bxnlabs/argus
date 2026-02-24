package filesearch

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestFile creates a file with content inside a test directory.
func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestIsAvailable(t *testing.T) {
	if !IsAvailable() {
		t.Skip("fd not installed, skipping")
	}
}

func TestLimitedWriter(t *testing.T) {
	t.Run("within limit", func(t *testing.T) {
		w := &limitedWriter{limit: 20}
		n, err := w.Write([]byte("hello"))
		if err != nil {
			t.Fatal(err)
		}
		if n != 5 {
			t.Errorf("wrote %d, want 5", n)
		}
		if w.buf.String() != "hello" {
			t.Errorf("buf = %q, want %q", w.buf.String(), "hello")
		}
	})

	t.Run("exceeds limit", func(t *testing.T) {
		w := &limitedWriter{limit: 5}
		_, err := w.Write([]byte("too long"))
		if err == nil {
			t.Error("expected error for exceeding limit")
		}
	})

	t.Run("cumulative overflow", func(t *testing.T) {
		w := &limitedWriter{limit: 10}
		_, err := w.Write([]byte("hello"))
		if err != nil {
			t.Fatal("first write should succeed")
		}
		_, err = w.Write([]byte("world!"))
		if err == nil {
			t.Fatal("second write should fail (exceeds 10 bytes)")
		}
	})
}

func TestRunFd(t *testing.T) {
	if !IsAvailable() {
		t.Skip("fd not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "package main\n")
	writeTestFile(t, dir, "sub/util.go", "package sub\n")

	t.Run("success", func(t *testing.T) {
		ctx := context.Background()
		out, err := runFd(ctx, dir, maxOutputBuffer, "--type", "f")
		if err != nil {
			t.Fatal(err)
		}
		if out == "" {
			t.Error("expected output")
		}
	})

	t.Run("no matches returns empty not error", func(t *testing.T) {
		ctx := context.Background()
		out, err := runFd(ctx, dir, maxOutputBuffer, "xyznonexistent")
		if err != nil {
			t.Fatalf("exit code 1 should not error: %v", err)
		}
		if out != "" {
			t.Errorf("expected empty output for no matches, got %q", out)
		}
	})

	t.Run("timeout returns partial output not error", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond)
		_, err := runFd(ctx, dir, maxOutputBuffer, "--version")
		if err != nil {
			t.Errorf("timeout should return partial output, not error: %v", err)
		}
	})
}
