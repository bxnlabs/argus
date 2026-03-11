package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetStatus(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "initial.txt", "hello", "initial commit")

	t.Run("clean repo", func(t *testing.T) {
		status, err := GetStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Branch == "" {
			t.Error("expected branch name")
		}
		if len(status.Staged) != 0 || len(status.Unstaged) != 0 || len(status.Untracked) != 0 {
			t.Error("expected clean status")
		}
	})

	t.Run("untracked file", func(t *testing.T) {
		writeTestFile(dir, "untracked.txt", "new file")
		status, err := GetStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(status.Untracked) != 1 {
			t.Fatalf("expected 1 untracked, got %d", len(status.Untracked))
		}
		if status.Untracked[0].Path != "untracked.txt" {
			t.Errorf("path = %q, want %q", status.Untracked[0].Path, "untracked.txt")
		}
		if status.Untracked[0].Status != StatusUntracked {
			t.Errorf("status = %q, want %q", status.Untracked[0].Status, StatusUntracked)
		}
	})

	t.Run("staged file", func(t *testing.T) {
		writeTestFile(dir, "staged.txt", "staged content")
		cmd := exec.Command("git", "add", "staged.txt")
		cmd.Dir = dir
		cmd.CombinedOutput()

		status, err := GetStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		hasStagedFile := false
		for _, f := range status.Staged {
			if f.Path == "staged.txt" && f.Status == StatusAdded && f.Staged {
				hasStagedFile = true
			}
		}
		if !hasStagedFile {
			t.Errorf("expected staged.txt in staged list, got %+v", status.Staged)
		}
	})

	t.Run("modified file", func(t *testing.T) {
		writeTestFile(dir, "initial.txt", "modified content")
		status, err := GetStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		hasUnstagedFile := false
		for _, f := range status.Unstaged {
			if f.Path == "initial.txt" && f.Status == StatusModified {
				hasUnstagedFile = true
			}
		}
		if !hasUnstagedFile {
			t.Errorf("expected initial.txt in unstaged list, got %+v", status.Unstaged)
		}
	})

	t.Run("untracked directory shows individual files", func(t *testing.T) {
		dir := initTestRepo(t)
		commitFile(t, dir, "existing.txt", "hello", "initial commit")

		if err := os.MkdirAll(filepath.Join(dir, "newdir", "sub"), 0755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		writeTestFile(dir, "newdir/file1.txt", "content1")
		writeTestFile(dir, "newdir/file2.txt", "content2")
		writeTestFile(dir, "newdir/sub/deep.txt", "deep content")

		status, err := GetStatus(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		paths := make(map[string]bool)
		for _, f := range status.Untracked {
			paths[f.Path] = true
		}
		for _, want := range []string{"newdir/file1.txt", "newdir/file2.txt", "newdir/sub/deep.txt"} {
			if !paths[want] {
				t.Errorf("expected untracked file %q, got paths: %v", want, paths)
			}
		}
		if paths["newdir/"] {
			t.Error("should not have bare directory entry newdir/")
		}
	})

	t.Run("non-git directory", func(t *testing.T) {
		_, err := GetStatus(t.TempDir())
		if err == nil {
			t.Error("expected error for non-git directory")
		}
	})
}

func TestGetFileDiff(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "file.txt", "original", "initial")

	t.Run("unstaged diff", func(t *testing.T) {
		writeTestFile(dir, "file.txt", "modified")
		diff, err := GetFileDiff(dir, "file.txt", false, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(diff, "-original") || !strings.Contains(diff, "+modified") {
			t.Errorf("unexpected diff: %s", diff)
		}
	})

	t.Run("untracked diff", func(t *testing.T) {
		writeTestFile(dir, "brand-new.txt", "new content here")
		diff, err := GetFileDiff(dir, "brand-new.txt", false, true)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(diff, "+new content here") {
			t.Errorf("expected untracked diff to contain file content, got: %s", diff)
		}
	})

	t.Run("staged diff", func(t *testing.T) {
		cmd := exec.Command("git", "add", "file.txt")
		cmd.Dir = dir
		cmd.CombinedOutput()

		diff, err := GetFileDiff(dir, "file.txt", true, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(diff, "-original") || !strings.Contains(diff, "+modified") {
			t.Errorf("unexpected staged diff: %s", diff)
		}
	})
}

func TestGetFileContent(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "existing.txt", "committed content", "add file")

	t.Run("existing file", func(t *testing.T) {
		content, isNew, err := GetFileContent(dir, "existing.txt")
		if err != nil {
			t.Fatal(err)
		}
		if isNew {
			t.Error("expected isNew=false")
		}
		if !strings.Contains(content, "committed content") {
			t.Errorf("unexpected content: %q", content)
		}
	})

	t.Run("new file", func(t *testing.T) {
		_, isNew, err := GetFileContent(dir, "nonexistent.txt")
		if err != nil {
			t.Fatal(err)
		}
		if !isNew {
			t.Error("expected isNew=true")
		}
	})
}

func TestGetWorkingDiff(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "tracked.txt", "original", "initial commit")

	t.Run("mixed changes", func(t *testing.T) {
		writeTestFile(dir, "tracked.txt", "modified content")
		writeTestFile(dir, "brand-new.txt", "new file content")

		result, err := GetWorkingDiff(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Diff == "" {
			t.Error("expected non-empty diff")
		}
		if len(result.Files) < 2 {
			t.Errorf("expected at least 2 files, got %d", len(result.Files))
		}
		if result.TotalAdditions == 0 {
			t.Error("expected non-zero additions")
		}
		for _, f := range result.Files {
			if f.Path == "brand-new.txt" {
				if f.Additions == 0 {
					t.Error("expected brand-new.txt to have Additions > 0")
				}
			}
		}
		paths := make(map[string]bool)
		for _, f := range result.Files {
			paths[f.Path] = true
		}
		if !paths["tracked.txt"] {
			t.Error("expected tracked.txt in files")
		}
		if !paths["brand-new.txt"] {
			t.Error("expected brand-new.txt in files")
		}
	})

	t.Run("staged changes included", func(t *testing.T) {
		cmd := exec.Command("git", "add", "tracked.txt")
		cmd.Dir = dir
		cmd.CombinedOutput()

		result, err := GetWorkingDiff(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Diff == "" {
			t.Error("expected non-empty diff")
		}
		hasStagedFile := false
		for _, f := range result.Files {
			if f.Path == "tracked.txt" {
				hasStagedFile = true
			}
		}
		if !hasStagedFile {
			t.Error("expected tracked.txt in file list")
		}
	})

	t.Run("clean repo", func(t *testing.T) {
		cleanDir := initTestRepo(t)
		commitFile(t, cleanDir, "f.txt", "x", "init")
		result, err := GetWorkingDiff(cleanDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Files) != 0 {
			t.Errorf("expected 0 files, got %d", len(result.Files))
		}
	})

	t.Run("no commits with staged file", func(t *testing.T) {
		emptyDir := initTestRepo(t)
		writeTestFile(emptyDir, "first.txt", "hello")
		cmd := exec.Command("git", "add", "first.txt")
		cmd.Dir = emptyDir
		cmd.CombinedOutput()

		result, err := GetWorkingDiff(emptyDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Diff == "" {
			t.Error("expected non-empty diff for staged file in HEAD-less repo")
		}
		found := false
		for _, f := range result.Files {
			if f.Path == "first.txt" && f.Status == StatusAdded {
				found = true
			}
		}
		if !found {
			t.Error("expected first.txt with StatusAdded in Files")
		}
		if result.TotalAdditions == 0 {
			t.Error("expected non-zero TotalAdditions")
		}
	})

	t.Run("deleted file", func(t *testing.T) {
		delDir := initTestRepo(t)
		commitFile(t, delDir, "remove-me.txt", "bye", "add file to delete")
		os.Remove(filepath.Join(delDir, "remove-me.txt"))

		result, err := GetWorkingDiff(delDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		found := false
		for _, f := range result.Files {
			if f.Path == "remove-me.txt" && f.Status == StatusDeleted {
				found = true
			}
		}
		if !found {
			t.Error("expected remove-me.txt with deleted status")
		}
	})

	t.Run("staged and unstaged changes to same file", func(t *testing.T) {
		bothDir := initTestRepo(t)
		commitFile(t, bothDir, "dual.txt", "original", "init")
		writeTestFile(bothDir, "dual.txt", "staged version")
		cmd := exec.Command("git", "add", "dual.txt")
		cmd.Dir = bothDir
		cmd.CombinedOutput()
		writeTestFile(bothDir, "dual.txt", "unstaged version")

		result, err := GetWorkingDiff(bothDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		count := 0
		for _, f := range result.Files {
			if f.Path == "dual.txt" {
				count++
			}
		}
		if count != 1 {
			t.Errorf("expected dual.txt exactly once, got %d", count)
		}
	})
}

func TestCheck(t *testing.T) {
	t.Run("git repo", func(t *testing.T) {
		dir := initTestRepo(t)
		commitFile(t, dir, "f.txt", "x", "init")

		isRepo, err := Check(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !isRepo {
			t.Error("expected true")
		}
	})

	t.Run("non-git directory", func(t *testing.T) {
		isRepo, err := Check(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if isRepo {
			t.Error("expected false")
		}
	})
}
