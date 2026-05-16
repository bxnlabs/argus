package compareview

import (
	"testing"

	"github.com/bxnlabs/argus/internal/node/git"
)

func intp(n int) *int { return &n }

func TestParseUnifiedDiff_SingleFileModify(t *testing.T) {
	in := `diff --git a/src/a.go b/src/a.go
index 1111111..2222222 100644
--- a/src/a.go
+++ b/src/a.go
@@ -1,3 +1,3 @@
 line1
-line2
+line2-new
 line3
`
	got, err := ParseUnifiedDiff(in)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 file, got %d", len(got))
	}
	f := got[0]
	if f.Path != "src/a.go" {
		t.Errorf("Path = %q, want %q", f.Path, "src/a.go")
	}
	if f.Status != git.StatusModified {
		t.Errorf("Status = %q, want %q", f.Status, git.StatusModified)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(f.Hunks))
	}
	h := f.Hunks[0]
	if h.Kind != HunkKindDiff {
		t.Errorf("Kind = %q, want %q", h.Kind, HunkKindDiff)
	}
	if h.OldStart != 1 || h.OldCount != 3 || h.NewStart != 1 || h.NewCount != 3 {
		t.Errorf("range mismatch: %+v", h)
	}
	if len(h.Lines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(h.Lines))
	}
	want := []HunkLine{
		{Type: "context", Content: "line1", OldLineNumber: intp(1), NewLineNumber: intp(1)},
		{Type: "deletion", Content: "line2", OldLineNumber: intp(2), NewLineNumber: nil},
		{Type: "addition", Content: "line2-new", OldLineNumber: nil, NewLineNumber: intp(2)},
		{Type: "context", Content: "line3", OldLineNumber: intp(3), NewLineNumber: intp(3)},
	}
	for i, w := range want {
		got := h.Lines[i]
		if got.Type != w.Type || got.Content != w.Content {
			t.Errorf("line %d: got %+v, want %+v", i, got, w)
		}
		if !intpEq(got.OldLineNumber, w.OldLineNumber) || !intpEq(got.NewLineNumber, w.NewLineNumber) {
			t.Errorf("line %d numbers: got %v/%v, want %v/%v", i, got.OldLineNumber, got.NewLineNumber, w.OldLineNumber, w.NewLineNumber)
		}
	}
}

func intpEq(a, b *int) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	return *a == *b
}

func TestParseUnifiedDiff_MultiFile(t *testing.T) {
	in := `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old
+new
diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-x
+y
`
	got, err := ParseUnifiedDiff(in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 files, got %d", len(got))
	}
	if got[0].Path != "a.txt" || got[1].Path != "b.txt" {
		t.Errorf("paths: %q, %q", got[0].Path, got[1].Path)
	}
}

func TestParseUnifiedDiff_NewFile(t *testing.T) {
	in := `diff --git a/new.txt b/new.txt
new file mode 100644
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+hello
+world
`
	got, _ := ParseUnifiedDiff(in)
	if len(got) != 1 {
		t.Fatalf("got %d files", len(got))
	}
	if got[0].Status != git.StatusAdded || got[0].Path != "new.txt" {
		t.Errorf("got %+v", got[0])
	}
}

func TestParseUnifiedDiff_DeletedFile(t *testing.T) {
	in := `diff --git a/gone.txt b/gone.txt
deleted file mode 100644
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
`
	got, _ := ParseUnifiedDiff(in)
	if len(got) != 1 {
		t.Fatalf("got %d files", len(got))
	}
	if got[0].Status != git.StatusDeleted || got[0].Path != "gone.txt" {
		t.Errorf("got %+v", got[0])
	}
}

func TestParseUnifiedDiff_Rename(t *testing.T) {
	in := `diff --git a/old.txt b/new.txt
similarity index 90%
rename from old.txt
rename to new.txt
--- a/old.txt
+++ b/new.txt
@@ -1 +1 @@
-a
+b
`
	got, _ := ParseUnifiedDiff(in)
	if len(got) != 1 {
		t.Fatalf("got %d files", len(got))
	}
	if got[0].Status != git.StatusRenamed || got[0].Path != "new.txt" || got[0].OldPath != "old.txt" {
		t.Errorf("got %+v", got[0])
	}
}

func TestParseUnifiedDiff_Empty(t *testing.T) {
	got, err := ParseUnifiedDiff("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil && len(got) != 0 {
		t.Errorf("expected nil/empty, got %v", got)
	}
}

func TestParseUnifiedDiff_BinaryFile(t *testing.T) {
	in := `diff --git a/img.bin b/img.bin
Binary files a/img.bin and b/img.bin differ
`
	got, _ := ParseUnifiedDiff(in)
	if len(got) != 1 {
		t.Fatalf("got %d files", len(got))
	}
	if !got[0].IsBinary || got[0].Path != "img.bin" {
		t.Errorf("got %+v", got[0])
	}
}
