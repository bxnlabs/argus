package git

import (
	"context"
	"errors"
	"os/exec"
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
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("nonexistent hash wraps ErrNotFound", func(t *testing.T) {
		_, err := GetCommitDetail(dir, "abcdef1234567890abcdef1234567890abcdef12")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

func TestGetCommitFullDiff(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa\n", "add a")
	commitFile(t, dir, "b.txt", "bbb\n", "add b")

	// Make a commit that changes both files
	writeTestFile(dir, "a.txt", "aaa modified\n")
	writeTestFile(dir, "b.txt", "bbb modified\n")
	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = dir
	cmd.CombinedOutput()
	cmd = exec.Command("git", "commit", "-m", "modify both files")
	cmd.Dir = dir
	cmd.CombinedOutput()

	commits, _ := GetHistory(dir, 1)
	hash := commits[0].Hash

	t.Run("returns combined diff", func(t *testing.T) {
		result, err := GetCommitFullDiff(dir, hash)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Diff, "a.txt") {
			t.Error("expected diff to contain a.txt")
		}
		if !strings.Contains(result.Diff, "b.txt") {
			t.Error("expected diff to contain b.txt")
		}
		// Should contain multiple diff --git sections
		if strings.Count(result.Diff, "diff --git") < 2 {
			t.Errorf("expected multiple diff sections, got %d", strings.Count(result.Diff, "diff --git"))
		}
	})

	t.Run("invalid hash", func(t *testing.T) {
		_, err := GetCommitFullDiff(dir, "not-a-hash!")
		if err == nil {
			t.Error("expected error for invalid hash")
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("expected ErrInvalidInput, got: %v", err)
		}
	})

	t.Run("nonexistent hash wraps ErrNotFound", func(t *testing.T) {
		_, err := GetCommitFullDiff(dir, "abcdef1234567890abcdef1234567890abcdef12")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got: %v", err)
		}
	})
}

// The file list feeds totalLines and nothing else, so no failure it can
// produce may reach the caller: the diff is already assembled by the time it
// runs, and a blown deadline there would answer 504 for a request that has a
// complete answer in hand. The signature is the guarantee — there is no error
// to propagate — and this pins the operational case the type alone can't show.
func TestCommitDiffFilesDegradesRatherThanFailing(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "a.txt", "aaa\n", "add a")
	commits, _ := GetHistory(dir, 1)
	hash := commits[0].Hash

	if files := commitDiffFiles(context.Background(), dir, hash); len(files) != 1 || files[0].Path != "a.txt" {
		t.Fatalf("premise: expected the commit's one file, got %+v", files)
	}

	ctx, cancel := expiredContext()
	defer cancel()
	if files := commitDiffFiles(ctx, dir, hash); len(files) != 0 {
		t.Errorf("expected a blown deadline to yield no files, got %+v", files)
	}
	// The empty list must still produce the empty map the response promises,
	// not a nil one that serialises as JSON null.
	if got := computeTotalLines(ctx, dir, hash, nil); got == nil {
		t.Error("expected an empty totalLines map rather than nil")
	}
}
