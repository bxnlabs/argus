package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testMaxSize is the size limit used in StreamWrite tests.
const testMaxSize int64 = 1 << 20 // 1 MB

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

func TestFileMeta(t *testing.T) {
	dir := t.TempDir()

	t.Run("text file", func(t *testing.T) {
		writeTestFile(t, dir, "hello.txt", "hello world")
		meta, err := FileMeta(filepath.Join(dir, "hello.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if meta.Size != 11 {
			t.Errorf("size = %d, want 11", meta.Size)
		}
		if meta.IsBinary {
			t.Error("expected IsBinary=false for text file")
		}
		if meta.ContentType == "" {
			t.Error("expected non-empty ContentType")
		}
	})

	t.Run("binary file", func(t *testing.T) {
		p := filepath.Join(dir, "binary.bin")
		os.WriteFile(p, []byte{0xFF, 0x00, 0xFE, 0xAB}, 0644)
		meta, err := FileMeta(p)
		if err != nil {
			t.Fatal(err)
		}
		if !meta.IsBinary {
			t.Error("expected IsBinary=true for binary file")
		}
		if meta.Size != 4 {
			t.Errorf("size = %d, want 4", meta.Size)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		writeTestFile(t, dir, "empty.txt", "")
		meta, err := FileMeta(filepath.Join(dir, "empty.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if meta.IsBinary {
			t.Error("expected IsBinary=false for empty file")
		}
		if meta.Size != 0 {
			t.Errorf("size = %d, want 0", meta.Size)
		}
	})

	t.Run("directory returns error", func(t *testing.T) {
		_, err := FileMeta(dir)
		if err == nil {
			t.Error("expected error for directory")
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := FileMeta(filepath.Join(dir, "nope.txt"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("content type detection", func(t *testing.T) {
		writeTestFile(t, dir, "page.html", "<html></html>")
		meta, err := FileMeta(filepath.Join(dir, "page.html"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(meta.ContentType, "html") {
			t.Errorf("expected html content type, got %q", meta.ContentType)
		}
	})
}

func TestIsBinary(t *testing.T) {
	t.Run("text bytes", func(t *testing.T) {
		if IsBinary([]byte("hello world\nline two")) {
			t.Error("expected false for text")
		}
	})

	t.Run("binary bytes", func(t *testing.T) {
		if !IsBinary([]byte{0xFF, 0x00, 0xFE}) {
			t.Error("expected true for binary")
		}
	})

	t.Run("empty bytes", func(t *testing.T) {
		if IsBinary([]byte{}) {
			t.Error("expected false for empty")
		}
	})

	t.Run("UTF-8 with high bytes", func(t *testing.T) {
		if IsBinary([]byte("caf\xc3\xa9")) {
			t.Error("expected false for valid UTF-8")
		}
	})
}

func TestStreamWrite(t *testing.T) {
	t.Run("write from reader", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "new.txt")
		n, err := StreamWrite(p, strings.NewReader("hello world"), testMaxSize)
		if err != nil {
			t.Fatal(err)
		}
		if n != 11 {
			t.Errorf("written = %d, want 11", n)
		}
		data, _ := os.ReadFile(p)
		if string(data) != "hello world" {
			t.Errorf("content = %q, want %q", data, "hello world")
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "a", "b", "c", "deep.txt")
		_, err := StreamWrite(p, strings.NewReader("deep"), testMaxSize)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(p)
		if string(data) != "deep" {
			t.Errorf("content = %q, want %q", data, "deep")
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "over.txt")
		StreamWrite(p, strings.NewReader("original"), testMaxSize)
		StreamWrite(p, strings.NewReader("updated"), testMaxSize)
		data, _ := os.ReadFile(p)
		if string(data) != "updated" {
			t.Errorf("content = %q, want %q", data, "updated")
		}
	})

	t.Run("exceeds max size", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "big.txt")
		_, err := StreamWrite(p, strings.NewReader(strings.Repeat("x", int(testMaxSize)+1)), testMaxSize)
		if err == nil {
			t.Error("expected error for oversized write")
		}
		// File should not exist after failed write
		if _, statErr := os.Stat(p); statErr == nil {
			t.Error("expected file to not exist after failed write")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "empty.txt")
		n, err := StreamWrite(p, strings.NewReader(""), testMaxSize)
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("written = %d, want 0", n)
		}
	})

	t.Run("atomic write", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "atomic.txt")
		n, err := StreamWrite(p, strings.NewReader("complete content"), testMaxSize)
		if err != nil {
			t.Fatal(err)
		}
		if n != 16 {
			t.Errorf("written = %d, want 16", n)
		}
		data, _ := os.ReadFile(p)
		if string(data) != "complete content" {
			t.Errorf("content = %q, want %q", data, "complete content")
		}
	})

	t.Run("preserves file permissions on overwrite", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "script.sh")
		// Create executable file
		os.WriteFile(p, []byte("#!/bin/sh\necho old"), 0755)
		// Overwrite it
		StreamWrite(p, strings.NewReader("#!/bin/sh\necho new"), testMaxSize)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0755 {
			t.Errorf("permissions = %o, want 0755", info.Mode().Perm())
		}
	})

	t.Run("new file gets 0644 permissions", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "new-perms.txt")
		StreamWrite(p, strings.NewReader("hello"), testMaxSize)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0644 {
			t.Errorf("permissions = %o, want 0644", info.Mode().Perm())
		}
	})

	t.Run("returns FileSizeError for oversized write", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "big.txt")
		_, err := StreamWrite(p, strings.NewReader(strings.Repeat("x", int(testMaxSize)+1)), testMaxSize)
		if err == nil {
			t.Fatal("expected error")
		}
		var sizeErr *FileSizeError
		if !errors.As(err, &sizeErr) {
			t.Errorf("expected *FileSizeError, got %T: %v", err, err)
		}
	})
}

func TestListDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create test structure:
	//   dir/
	//     adir/
	//       nested.txt
	//     bfile.txt
	//     .git/          (excluded)
	//     node_modules/  (excluded)
	//     image.png
	//     debug.log      (excluded by *.log glob)
	os.MkdirAll(filepath.Join(dir, "adir"), 0755)
	writeTestFile(t, dir, "adir/nested.txt", "nested")
	writeTestFile(t, dir, "bfile.txt", "content")
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)
	os.MkdirAll(filepath.Join(dir, "node_modules"), 0755)
	writeTestFile(t, dir, "image.png", "fake png")
	writeTestFile(t, dir, "debug.log", "log data")

	t.Run("non-recursive listing", func(t *testing.T) {
		nodes, err := ListDirectory(dir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		// Should have: adir, bfile.txt, image.png (excluded: .git, node_modules, debug.log)
		if len(nodes) != 3 {
			t.Fatalf("expected 3 entries, got %d: %v", len(nodes), nodeNames(nodes))
		}
		// Directories first, then alphabetical
		if nodes[0].Name != "adir" || nodes[0].Type != "directory" {
			t.Errorf("first entry should be adir directory, got %+v", nodes[0])
		}
		// No children in non-recursive mode
		if nodes[0].Children != nil {
			t.Error("expected nil children in non-recursive mode")
		}
	})

	t.Run("recursive listing", func(t *testing.T) {
		nodes, err := ListDirectory(dir, true, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(nodes) != 3 {
			t.Fatalf("expected 3 top-level entries, got %d", len(nodes))
		}
		// adir should have children
		if nodes[0].Name != "adir" {
			t.Fatalf("expected adir first, got %s", nodes[0].Name)
		}
		if len(nodes[0].Children) != 1 {
			t.Fatalf("expected 1 child in adir, got %d", len(nodes[0].Children))
		}
		if nodes[0].Children[0].Name != "nested.txt" {
			t.Errorf("child name = %q, want %q", nodes[0].Children[0].Name, "nested.txt")
		}
	})

	t.Run("file extension without dot", func(t *testing.T) {
		nodes, err := ListDirectory(dir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range nodes {
			if n.Name == "image.png" {
				if n.Extension != "png" {
					t.Errorf("extension = %q, want %q", n.Extension, "png")
				}
				if n.Size == 0 {
					t.Error("expected non-zero size for file")
				}
				return
			}
		}
		t.Error("image.png not found in listing")
	})

	t.Run("directories sorted first", func(t *testing.T) {
		nodes, err := ListDirectory(dir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		if nodes[0].Type != "directory" {
			t.Error("expected directory first")
		}
		for i := 1; i < len(nodes); i++ {
			if nodes[i].Type == "directory" {
				t.Error("expected all directories before files")
			}
		}
	})

	t.Run("excluded patterns filtered", func(t *testing.T) {
		nodes, err := ListDirectory(dir, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range nodes {
			if shouldExclude(n.Name) {
				t.Errorf("excluded entry %q should not appear", n.Name)
			}
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		empty := t.TempDir()
		nodes, err := ListDirectory(empty, false, 1)
		if err != nil {
			t.Fatal(err)
		}
		if nodes == nil {
			t.Error("expected non-nil empty slice")
		}
		if len(nodes) != 0 {
			t.Errorf("expected 0 entries, got %d", len(nodes))
		}
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		_, err := ListDirectory(filepath.Join(dir, "nonexistent"), false, 1)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("depth limit respected", func(t *testing.T) {
		// Create 4-level deep structure
		deep := t.TempDir()
		os.MkdirAll(filepath.Join(deep, "a", "b", "c", "d"), 0755)
		writeTestFile(t, deep, "a/b/c/d/deep.txt", "deep")

		// maxDepth=2: should see a/, a/b/, a/b/c/ but NOT a/b/c/d/
		nodes, err := ListDirectory(deep, true, 2)
		if err != nil {
			t.Fatal(err)
		}
		// Navigate to a/b/c — it should have no children (depth exceeded)
		cNode := nodes[0].Children[0].Children[0] // a → b → c
		if cNode.Name != "c" {
			t.Fatalf("expected 'c', got %q", cNode.Name)
		}
		if cNode.Children != nil {
			t.Error("expected nil children at max depth")
		}
	})
}

// nodeNames extracts node names for error messages.
func nodeNames(nodes []FileNode) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Name
	}
	return out
}
