package git

import (
	"strings"
	"testing"
)

func TestGetHistory(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa", "first commit")
	commitFile(t, dir, "b.txt", "bbb", "second commit")
	commitFile(t, dir, "c.txt", "ccc", "third commit")

	t.Run("returns commits", func(t *testing.T) {
		commits, err := GetHistory(dir, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(commits) != 3 {
			t.Fatalf("expected 3 commits, got %d", len(commits))
		}
		// Most recent first
		if commits[0].Subject != "third commit" {
			t.Errorf("first commit subject = %q, want %q", commits[0].Subject, "third commit")
		}
		if commits[0].Hash == "" || commits[0].ShortHash == "" {
			t.Error("expected hash and shortHash")
		}
		if commits[0].Timestamp == "" {
			t.Error("expected timestamp")
		}
		if commits[0].RelativeTime == "" {
			t.Error("expected relativeTime")
		}
		if commits[0].Author != "Test" {
			t.Errorf("author = %q, want %q", commits[0].Author, "Test")
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		commits, err := GetHistory(dir, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(commits) != 2 {
			t.Fatalf("expected 2 commits, got %d", len(commits))
		}
	})

	t.Run("shortstat parsed", func(t *testing.T) {
		commits, err := GetHistory(dir, 1)
		if err != nil {
			t.Fatal(err)
		}
		if commits[0].FilesChanged == 0 {
			t.Error("expected filesChanged > 0")
		}
	})

	t.Run("empty repo", func(t *testing.T) {
		emptyDir := initTestRepo(t)
		commits, err := GetHistory(emptyDir, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(commits) != 0 {
			t.Errorf("expected 0 commits for empty repo, got %d", len(commits))
		}
	})
}

func TestGetCommitDetail(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "detail.txt", "content", "detail commit")

	// Get the hash from history
	commits, _ := GetHistory(dir, 1)
	if len(commits) == 0 {
		t.Fatal("no commits")
	}
	hash := commits[0].Hash

	t.Run("returns detail", func(t *testing.T) {
		detail, err := GetCommitDetail(dir, hash)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Hash != hash {
			t.Errorf("hash = %q, want %q", detail.Hash, hash)
		}
		if detail.Subject != "detail commit" {
			t.Errorf("subject = %q, want %q", detail.Subject, "detail commit")
		}
		if len(detail.Files) == 0 {
			t.Error("expected files")
		}
		if detail.Files[0].Path != "detail.txt" {
			t.Errorf("file path = %q, want %q", detail.Files[0].Path, "detail.txt")
		}
	})

	t.Run("invalid hash", func(t *testing.T) {
		_, err := GetCommitDetail(dir, "not-a-hash!")
		if err == nil {
			t.Error("expected error for invalid hash")
		}
	})
}

func TestGetCommitFileDiff(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "diff.txt", "hello\nworld\n", "add diff.txt")

	commits, _ := GetHistory(dir, 1)
	hash := commits[0].Hash

	diff, err := GetCommitFileDiff(dir, hash, "diff.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "+hello") {
		t.Errorf("expected diff to contain +hello, got: %s", diff)
	}
}
