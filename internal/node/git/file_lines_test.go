package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetFileLines_WorkingTree(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "dummy.txt", "init", "initial")

	// Create a test file with known content
	lines := []string{"line1", "line2", "line3", "line4", "line5", "line6", "line7", "line8", "line9", "line10"}
	content := strings.Join(lines, "\n") + "\n"
	writeTestFile(dir, "test.txt", content)

	t.Run("full range", func(t *testing.T) {
		result, err := GetFileLines(dir, "test.txt", 1, 10, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Lines) != 10 {
			t.Errorf("expected 10 lines, got %d", len(result.Lines))
		}
		if result.Start != 1 {
			t.Errorf("expected start=1, got %d", result.Start)
		}
		if result.End != 10 {
			t.Errorf("expected end=10, got %d", result.End)
		}
		if result.TotalLines != 10 {
			t.Errorf("expected totalLines=10, got %d", result.TotalLines)
		}
		if result.Lines[0] != "line1" {
			t.Errorf("expected first line 'line1', got %q", result.Lines[0])
		}
		if result.Lines[9] != "line10" {
			t.Errorf("expected last line 'line10', got %q", result.Lines[9])
		}
	})

	t.Run("partial range", func(t *testing.T) {
		result, err := GetFileLines(dir, "test.txt", 3, 7, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Lines) != 5 {
			t.Errorf("expected 5 lines, got %d", len(result.Lines))
		}
		if result.Lines[0] != "line3" {
			t.Errorf("expected 'line3', got %q", result.Lines[0])
		}
		if result.Lines[4] != "line7" {
			t.Errorf("expected 'line7', got %q", result.Lines[4])
		}
		if result.TotalLines != 10 {
			t.Errorf("expected totalLines=10, got %d", result.TotalLines)
		}
	})

	t.Run("clamp end beyond file", func(t *testing.T) {
		result, err := GetFileLines(dir, "test.txt", 8, 15, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Lines) != 3 {
			t.Errorf("expected 3 lines (8,9,10), got %d", len(result.Lines))
		}
		if result.End != 10 {
			t.Errorf("expected end=10, got %d", result.End)
		}
	})

	t.Run("start beyond file", func(t *testing.T) {
		result, err := GetFileLines(dir, "test.txt", 20, 25, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Lines) != 0 {
			t.Errorf("expected 0 lines, got %d", len(result.Lines))
		}
		if result.Start != 20 {
			t.Errorf("expected start=20, got %d", result.Start)
		}
		if result.End != 19 {
			t.Errorf("expected end=19 (start-1) for empty result, got %d", result.End)
		}
		if result.TotalLines != 10 {
			t.Errorf("expected totalLines=10, got %d", result.TotalLines)
		}
	})

	t.Run("invalid range start > end", func(t *testing.T) {
		_, err := GetFileLines(dir, "test.txt", 5, 3, "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("invalid range start < 1", func(t *testing.T) {
		_, err := GetFileLines(dir, "test.txt", 0, 5, "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("span exceeds max", func(t *testing.T) {
		_, err := GetFileLines(dir, "test.txt", 1, 600, "")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := GetFileLines(dir, "nonexistent.txt", 1, 5, "")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("oversize file", func(t *testing.T) {
		bigFile := filepath.Join(dir, "big.txt")
		f, err := os.Create(bigFile)
		if err != nil {
			t.Fatal(err)
		}
		// Write just over 2MB
		data := make([]byte, maxFileSizeBytes+1)
		for i := range data {
			data[i] = 'x'
		}
		f.Write(data)
		f.Close()

		_, err = GetFileLines(dir, "big.txt", 1, 5, "")
		if !errors.Is(err, ErrFileTooLarge) {
			t.Errorf("expected ErrFileTooLarge, got: %v", err)
		}
	})

	t.Run("binary file", func(t *testing.T) {
		binFile := filepath.Join(dir, "binary.dat")
		// Write content with null bytes
		os.WriteFile(binFile, []byte("hello\x00world\n"), 0644)

		_, err := GetFileLines(dir, "binary.dat", 1, 5, "")
		if !errors.Is(err, ErrBinaryFile) {
			t.Errorf("expected ErrBinaryFile, got: %v", err)
		}
	})

	t.Run("file without trailing newline", func(t *testing.T) {
		writeTestFile(dir, "noeol.txt", "line1\nline2\nline3")
		result, err := GetFileLines(dir, "noeol.txt", 1, 5, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.TotalLines != 3 {
			t.Errorf("expected totalLines=3, got %d", result.TotalLines)
		}
		if len(result.Lines) != 3 {
			t.Errorf("expected 3 lines, got %d", len(result.Lines))
		}
	})
}

func TestGetFileLines_RefBased(t *testing.T) {
	dir := initTestRepo(t)

	// Create and commit a file
	lines := []string{"alpha", "beta", "gamma", "delta", "epsilon"}
	content := strings.Join(lines, "\n") + "\n"
	commitFile(t, dir, "ref-test.txt", content, "add ref-test.txt")

	// Get the commit hash
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get HEAD: %v", err)
	}
	commitHash := strings.TrimSpace(string(out))

	t.Run("read from ref", func(t *testing.T) {
		result, err := GetFileLines(dir, "ref-test.txt", 2, 4, commitHash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Lines) != 3 {
			t.Errorf("expected 3 lines, got %d", len(result.Lines))
		}
		if result.Lines[0] != "beta" {
			t.Errorf("expected 'beta', got %q", result.Lines[0])
		}
		if result.Lines[2] != "delta" {
			t.Errorf("expected 'delta', got %q", result.Lines[2])
		}
		if result.TotalLines != 5 {
			t.Errorf("expected totalLines=5, got %d", result.TotalLines)
		}
	})

	t.Run("start beyond file at ref", func(t *testing.T) {
		result, err := GetFileLines(dir, "ref-test.txt", 100, 105, commitHash)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Lines) != 0 {
			t.Errorf("expected 0 lines, got %d", len(result.Lines))
		}
		if result.Start != 100 {
			t.Errorf("expected start=100, got %d", result.Start)
		}
		if result.End != 99 {
			t.Errorf("expected end=99 (start-1) for empty result, got %d", result.End)
		}
		if result.TotalLines != 5 {
			t.Errorf("expected totalLines=5, got %d", result.TotalLines)
		}
	})

	t.Run("invalid ref format", func(t *testing.T) {
		_, err := GetFileLines(dir, "ref-test.txt", 1, 5, "HEAD")
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for symbolic ref, got: %v", err)
		}
	})

	t.Run("short hash rejected", func(t *testing.T) {
		_, err := GetFileLines(dir, "ref-test.txt", 1, 5, commitHash[:7])
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput for short hash, got: %v", err)
		}
	})

	t.Run("file not found at ref", func(t *testing.T) {
		_, err := GetFileLines(dir, "nonexistent.txt", 1, 5, commitHash)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})

	t.Run("nonexistent ref", func(t *testing.T) {
		fakeHash := "0000000000000000000000000000000000000000"
		_, err := GetFileLines(dir, "ref-test.txt", 1, 5, fakeHash)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestCountFileLines(t *testing.T) {
	dir := t.TempDir()

	t.Run("with trailing newline", func(t *testing.T) {
		path := filepath.Join(dir, "with-eol.txt")
		os.WriteFile(path, []byte("a\nb\nc\n"), 0644)
		count, err := countFileLines(path)
		if err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("without trailing newline", func(t *testing.T) {
		path := filepath.Join(dir, "no-eol.txt")
		os.WriteFile(path, []byte("a\nb\nc"), 0644)
		count, err := countFileLines(path)
		if err != nil {
			t.Fatal(err)
		}
		if count != 3 {
			t.Errorf("expected 3, got %d", count)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		os.WriteFile(path, []byte(""), 0644)
		count, err := countFileLines(path)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("expected 0, got %d", count)
		}
	})

	t.Run("single line no newline", func(t *testing.T) {
		path := filepath.Join(dir, "single.txt")
		os.WriteFile(path, []byte("hello"), 0644)
		count, err := countFileLines(path)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("expected 1, got %d", count)
		}
	})
}

func TestGetWorkingDiff_TotalLinesAndFingerprint(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "file.txt", "line1\nline2\nline3\n", "initial")
	writeTestFile(dir, "file.txt", "line1\nmodified\nline3\n")

	result, err := GetWorkingDiff(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalLines == nil {
		t.Fatal("expected TotalLines to be non-nil")
	}
	if count, ok := result.TotalLines["file.txt"]; !ok {
		t.Error("expected file.txt in TotalLines")
	} else if count != 3 {
		t.Errorf("expected totalLines=3 for file.txt, got %d", count)
	}

	if result.Fingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if len(result.Fingerprint) != 64 {
		t.Errorf("expected 64-char hex SHA-256, got %d chars", len(result.Fingerprint))
	}

	// Same diff should produce same fingerprint
	result2, err := GetWorkingDiff(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Fingerprint != result2.Fingerprint {
		t.Error("expected identical fingerprints for unchanged diff")
	}
}
