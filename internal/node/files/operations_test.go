package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestReadForViewer(t *testing.T) {
	dir := t.TempDir()

	t.Run("text file", func(t *testing.T) {
		writeTestFile(t, dir, "hello.txt", "hello world")
		view, err := ReadForViewer(filepath.Join(dir, "hello.txt"), ViewerMaxBytes, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.Content != "hello world" {
			t.Errorf("content = %q, want %q", view.Content, "hello world")
		}
		if view.Size != 11 {
			t.Errorf("size = %d, want 11", view.Size)
		}
		if view.IsBinary || view.IsLarge {
			t.Errorf("IsBinary = %v, IsLarge = %v, want both false", view.IsBinary, view.IsLarge)
		}
	})

	t.Run("binary file carries no content", func(t *testing.T) {
		p := filepath.Join(dir, "binary.bin")
		os.WriteFile(p, []byte{0xFF, 0x00, 0xFE, 0xAB}, 0644)
		view, err := ReadForViewer(p, ViewerMaxBytes, "")
		if err != nil {
			t.Fatal(err)
		}
		if !view.IsBinary {
			t.Error("expected IsBinary=true for binary file")
		}
		if view.Content != "" {
			t.Errorf("content = %q, want empty for a binary file", view.Content)
		}
		if view.Size != 4 {
			t.Errorf("size = %d, want 4", view.Size)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		writeTestFile(t, dir, "empty.txt", "")
		view, err := ReadForViewer(filepath.Join(dir, "empty.txt"), ViewerMaxBytes, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.IsBinary {
			t.Error("expected IsBinary=false for empty file")
		}
		if view.Size != 0 {
			t.Errorf("size = %d, want 0", view.Size)
		}
	})

	// The ceiling is enforced on bytes actually read, not on the stat, so a
	// file that grows between the two cannot smuggle itself past it.
	t.Run("past the ceiling carries no content", func(t *testing.T) {
		writeTestFile(t, dir, "big.txt", strings.Repeat("a", 33))
		view, err := ReadForViewer(filepath.Join(dir, "big.txt"), 32, "")
		if err != nil {
			t.Fatal(err)
		}
		if !view.IsLarge {
			t.Error("expected IsLarge=true past the ceiling")
		}
		if view.Content != "" {
			t.Errorf("content = %q, want empty past the ceiling", view.Content)
		}
	})

	t.Run("exactly at the ceiling still carries content", func(t *testing.T) {
		writeTestFile(t, dir, "exact.txt", strings.Repeat("a", 32))
		view, err := ReadForViewer(filepath.Join(dir, "exact.txt"), 32, "")
		if err != nil {
			t.Fatal(err)
		}
		if view.IsLarge {
			t.Error("expected IsLarge=false for a file exactly at the ceiling")
		}
		if len(view.Content) != 32 {
			t.Errorf("content is %d bytes, want 32", len(view.Content))
		}
	})

	t.Run("directory returns ErrNotRegular", func(t *testing.T) {
		_, err := ReadForViewer(dir, ViewerMaxBytes, "")
		if !errors.Is(err, ErrNotRegular) {
			t.Errorf("err = %v, want ErrNotRegular", err)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := ReadForViewer(filepath.Join(dir, "nope.txt"), ViewerMaxBytes, "")
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("err = %v, want os.ErrNotExist", err)
		}
	})
}

// An open pane re-reads itself every 30s and is nearly always looking at a
// file nobody touched, so the unchanged answer is the common one.
func TestReadForViewerUnchanged(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "poll.txt")
	writeTestFile(t, dir, "poll.txt", "first")

	first, err := ReadForViewer(p, ViewerMaxBytes, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.ETag == "" {
		t.Fatal("expected an etag on a full read")
	}

	t.Run("a matching etag sends nothing back", func(t *testing.T) {
		again, err := ReadForViewer(p, ViewerMaxBytes, first.ETag)
		if err != nil {
			t.Fatal(err)
		}
		if !again.Unchanged {
			t.Error("Unchanged = false, want true for a file that did not move")
		}
		if again.Content != "" {
			t.Errorf("content = %q, want empty when unchanged", again.Content)
		}
		if again.ETag != first.ETag {
			t.Errorf("etag = %q, want it stable at %q", again.ETag, first.ETag)
		}
	})

	t.Run("a rewrite invalidates it", func(t *testing.T) {
		// Rewrite to a different length so the etag moves on size alone; the
		// test then does not rest on the filesystem's timestamp resolution.
		if err := os.WriteFile(p, []byte("second, and longer"), 0644); err != nil {
			t.Fatal(err)
		}
		after, err := ReadForViewer(p, ViewerMaxBytes, first.ETag)
		if err != nil {
			t.Fatal(err)
		}
		if after.Unchanged {
			t.Fatal("Unchanged = true, want false after a rewrite")
		}
		if after.Content != "second, and longer" {
			t.Errorf("content = %q, want the new bytes", after.Content)
		}
		if after.ETag == first.ETag {
			t.Error("etag did not move with the file")
		}
	})

	t.Run("a touch alone invalidates it", func(t *testing.T) {
		// Same bytes, new mtime: the validator is deliberately not a content
		// hash, so this costs one redundant transfer rather than going stale.
		writeTestFile(t, dir, "touched.txt", "same")
		tp := filepath.Join(dir, "touched.txt")
		before, err := ReadForViewer(tp, ViewerMaxBytes, "")
		if err != nil {
			t.Fatal(err)
		}
		future := time.Now().Add(2 * time.Second)
		if err := os.Chtimes(tp, future, future); err != nil {
			t.Fatal(err)
		}
		after, err := ReadForViewer(tp, ViewerMaxBytes, before.ETag)
		if err != nil {
			t.Fatal(err)
		}
		if after.Unchanged {
			t.Error("Unchanged = true, want false once the mtime moved")
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
