package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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

	// Same SIGPIPE masking as runGit: an untracked file whose diff exceeds the
	// buffer must report the limit, not "signal: broken pipe".
	t.Run("untracked diff over limit reports the limit", func(t *testing.T) {
		writeTestFile(dir, "huge.txt", strings.Repeat("x=1\n", 300_000))
		_, err := runGitNoIndex(context.Background(), dir, `git diff of "huge.txt"`, 1024,
			"diff", "-U20", "--no-index", "/dev/null", "huge.txt")
		if err == nil {
			t.Fatal("expected error for oversized untracked diff")
		}
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge, got: %v", err)
		}
		if strings.Contains(err.Error(), "broken pipe") {
			t.Errorf("oversized output masked as a signal error: %v", err)
		}
		if !strings.Contains(err.Error(), "1024 bytes") {
			t.Errorf("expected error to name the limit it was given, got: %v", err)
		}
	})

	// git reports "could not access" as exit 1 with empty stdout, which is the
	// same status "files differ" uses. Accepting the status alone would return
	// a vanished path as an answered request with no changes.
	t.Run("inaccessible path is not an empty diff", func(t *testing.T) {
		out, err := runGitNoIndex(context.Background(), dir, `git diff of "gone.txt"`, diffMaxBuffer,
			"diff", "-U20", "--no-index", "/dev/null", "gone.txt")
		if err == nil {
			t.Fatalf("expected an error for a path that cannot be accessed, got diff %q", out)
		}
		if !strings.Contains(err.Error(), "gone.txt") {
			t.Errorf("expected the error to name the path, got: %v", err)
		}
		// Non-operational, so appendUntrackedDiffs warns and continues rather
		// than failing the whole request — a file listed by status can vanish.
		if isOperationalError(err) {
			t.Errorf("a missing path is not an operational failure, got: %v", err)
		}
	})

	// A configured diff.external replaces git's patch with the driver's output,
	// and a driver that exits 0 silently makes git report "files differ" with
	// nothing on stdout. Without --no-ext-diff that is indistinguishable from
	// the access failure above, so a real diff would be rejected as an error.
	t.Run("external diff driver does not suppress the patch", func(t *testing.T) {
		driver := filepath.Join(t.TempDir(), "silent.sh")
		if err := os.WriteFile(driver, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatal(err)
		}
		gitInDir(t, dir, "config", "diff.external", driver)
		defer gitInDir(t, dir, "config", "--unset", "diff.external")

		writeTestFile(dir, "ext-untracked.txt", "content here")
		diff, err := GetFileDiff(dir, "ext-untracked.txt", false, true)
		if err != nil {
			t.Fatalf("a real diff must survive a silent external driver: %v", err)
		}
		if !strings.Contains(diff, "+content here") {
			t.Errorf("expected the native patch, got: %q", diff)
		}

		// The tracked path is served by runGit, which has no exit-1 guard, but
		// the same driver would otherwise blank the working diff entirely.
		tracked, err := GetFileDiff(dir, "file.txt", false, false)
		if err != nil {
			t.Fatalf("tracked diff under an external driver: %v", err)
		}
		if tracked == "" {
			t.Error("expected a native tracked patch, got an empty diff")
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

	// GetFileContent maps any runGit failure to "not in HEAD = new file". For a
	// size failure that is not merely a lost error, it is wrong data: a file
	// that exists in HEAD gets rendered as a brand-new empty one, with nothing
	// logged and no error to explain it.
	t.Run("oversized committed file is an error, not a new file", func(t *testing.T) {
		repo := initTestRepo(t)
		commitFile(t, repo, "huge.txt", oversizedContent(), "add huge")

		content, isNew, err := GetFileContent(repo, "huge.txt")
		if err == nil {
			t.Fatalf("expected error, got isNew=%v content=%d bytes", isNew, len(content))
		}
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge, got: %v", err)
		}
		if isNew {
			t.Error("oversized committed file must not be reported as new")
		}
	})
}

// oversizedContent returns text guaranteed to exceed diffMaxBuffer, so tests
// exercise the real production limit rather than an injected one.
func oversizedContent() string {
	return strings.Repeat("x=1\n", int(diffMaxBuffer/4)+1024)
}

// sizedContent returns approximately n bytes of diffable text.
func sizedContent(n int) string {
	return strings.Repeat("x=1\n", n/4)
}

// GetWorkingDiff's untracked loop must run on the caller's deadline. When each
// file got its own longTimeout, the loop could outlast the enclosing context
// and the next call after it failed on an expired context instead — far from
// the loop that actually spent the time.
func TestGetFileDiffHonorsCallerContext(t *testing.T) {
	dir := initTestRepo(t)
	commitFile(t, dir, "file.txt", "original", "initial")
	writeTestFile(dir, "file.txt", "modified")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := getFileDiff(ctx, dir, "file.txt", false, false); err == nil {
		t.Error("expected error from a cancelled context")
	}

	writeTestFile(dir, "untracked.txt", "new")
	if _, err := getFileDiff(ctx, dir, "untracked.txt", false, true); err == nil {
		t.Error("expected error from a cancelled context on the untracked path")
	}
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

	// The tracked diff already hard-fails when it exceeds the buffer, so an
	// untracked file hitting the same limit must fail the same way. Logging and
	// continuing returns 200 with the file present in Files but absent from
	// Diff — and a fingerprint computed over the incomplete diff.
	t.Run("oversized untracked file fails instead of silently dropping", func(t *testing.T) {
		repo := initTestRepo(t)
		commitFile(t, repo, "tracked.txt", "hello\n", "initial commit")
		if err := writeTestFile(repo, "huge.txt", oversizedContent()); err != nil {
			t.Fatal(err)
		}

		result, err := GetWorkingDiff(repo)
		if err == nil {
			t.Fatalf("expected error, got diff of %d bytes", len(result.Diff))
		}
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge, got: %v", err)
		}
	})

	// Each git command is bounded on its own, so two individually-legal diffs
	// used to sum past the limit unchecked. Neither file here trips the
	// per-command cap; only their combination does.
	t.Run("individually legal untracked diffs are bounded in aggregate", func(t *testing.T) {
		repo := initTestRepo(t)
		commitFile(t, repo, "tracked.txt", "hello\n", "initial commit")
		half := int(diffMaxBuffer / 2)
		for _, name := range []string{"a-half.txt", "b-half.txt"} {
			if err := writeTestFile(repo, name, sizedContent(half)); err != nil {
				t.Fatal(err)
			}
			// Guard the premise: if either file alone tripped the per-command
			// limit, this would pass without the aggregate bound existing.
			if _, err := GetFileDiff(repo, name, false, true); err != nil {
				t.Fatalf("%s should be under the per-command limit: %v", name, err)
			}
		}

		_, err := GetWorkingDiff(repo)
		if err == nil {
			t.Fatal("expected error for combined diff over the limit")
		}
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge, got: %v", err)
		}
		if !strings.Contains(err.Error(), "gitignored") {
			t.Errorf("expected an actionable message naming a likely cause, got: %v", err)
		}
	})

	// The count cap must fire before any per-file work: the point is to avoid
	// spawning two git processes per path, not to fail after doing so.
	t.Run("untracked file count over cap fails before diffing", func(t *testing.T) {
		repo := initTestRepo(t)
		commitFile(t, repo, "tracked.txt", "hello\n", "initial commit")
		for i := 0; i <= maxUntrackedFiles; i++ {
			if err := writeTestFile(repo, fmt.Sprintf("f%05d.txt", i), "x\n"); err != nil {
				t.Fatal(err)
			}
		}

		_, err := GetWorkingDiff(repo)
		if err == nil {
			t.Fatal("expected error for untracked file count over the cap")
		}
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge, got: %v", err)
		}
		if !strings.Contains(err.Error(), strconv.Itoa(maxUntrackedFiles)) {
			t.Errorf("expected the cap in the message, got: %v", err)
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

	// The HEAD-less path used to concatenate the cached and unstaged diffs by
	// hand before appending. They are now separate parts joined by the shared
	// separator logic. That is only safe if the bytes are identical — the
	// fingerprint is a SHA-256 of this string and drives frontend staleness.
	t.Run("no commits: combined diff is byte-identical to cached + unstaged", func(t *testing.T) {
		repo := initTestRepo(t)
		writeTestFile(repo, "staged.txt", "v1\n")
		gitInDir(t, repo, "add", "staged.txt")
		writeTestFile(repo, "staged.txt", "v2\n")

		gitOutput := func(args ...string) string {
			cmd := exec.Command("git", args...)
			cmd.Dir = repo
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("git %v: %v", args, err)
			}
			return string(out)
		}
		want := gitOutput("diff", "-U3", "--cached") + "\n" + gitOutput("diff", "-U3")

		result, err := GetWorkingDiff(repo)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Diff != want {
			t.Errorf("combined diff changed shape.\ngot:  %q\nwant: %q", result.Diff, want)
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

// A blown deadline must stop the untracked loop rather than being logged per
// file. Every remaining iteration would fail identically, so continuing buys
// nothing and costs one warning per path — up to maxUntrackedFiles of them —
// before the timeout finally surfaces from whichever command runs next.
func TestAppendUntrackedDiffsStopsOnExpiredContext(t *testing.T) {
	repo := initTestRepo(t)
	paths := []string{"a.txt", "b.txt", "c.txt"}
	for _, p := range paths {
		if err := writeTestFile(repo, p, "x\n"); err != nil {
			t.Fatal(err)
		}
	}

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	ctx, cancel := expiredContext()
	defer cancel()

	err := appendUntrackedDiffs(ctx, repo, paths, func(string) error { return nil })
	if err == nil {
		t.Fatal("expected an error from an expired context")
	}
	// runGitDiffNoIndex wraps the cause with %w, which is what lets the loop
	// classify it from the chain instead of sampling ctx.Err() afterwards.
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the context error to reach the caller, got: %v", err)
	}
	if n := strings.Count(logs.String(), "warning: diff for untracked"); n != 0 {
		t.Errorf("expected the loop to abort without per-file warnings, got %d", n)
	}
}

// expiredContext returns a context whose deadline has already passed, so
// ctx.Err() is DeadlineExceeded deterministically rather than Canceled.
func expiredContext() (context.Context, context.CancelFunc) {
	return context.WithDeadline(context.Background(), time.Now().Add(-time.Hour))
}

func TestAsTimeoutError(t *testing.T) {
	boom := errors.New("boom")

	t.Run("deadline in the chain types the failure", func(t *testing.T) {
		err := asTimeoutError(fmt.Errorf("git diff: %w", context.DeadlineExceeded))
		if !errors.Is(err, ErrTimeout) {
			t.Fatalf("expected ErrTimeout, got: %v", err)
		}
		if !strings.Contains(err.Error(), gitignoreHint) {
			t.Errorf("expected an actionable message, got: %v", err)
		}
	})

	// The classifier must read the error chain, not sample ctx.Err() after the
	// fact: a real git failure landing just before the deadline would otherwise
	// be discarded and reported as a timeout.
	t.Run("unrelated error passes through", func(t *testing.T) {
		if err := asTimeoutError(boom); err != boom {
			t.Errorf("expected the original error, got: %v", err)
		}
	})

	t.Run("nil error stays nil", func(t *testing.T) {
		if err := asTimeoutError(nil); err != nil {
			t.Errorf("expected nil, got: %v", err)
		}
	})

	// A size failure is the more specific diagnosis and already carries its own
	// actionable message; a deadline blown alongside it must not mask it.
	t.Run("size failure outranks a deadline", func(t *testing.T) {
		size := fmt.Errorf("%w: too big: %w", ErrOutputTooLarge, context.DeadlineExceeded)
		err := asTimeoutError(size)
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge to survive, got: %v", err)
		}
		if errors.Is(err, ErrTimeout) {
			t.Errorf("size failure should not be retyped as a timeout: %v", err)
		}
	})
}

// runGit types timeouts centrally so every exported operation gets a 504 with
// a message rather than a bare 500, and wraps both sentinels: ErrTimeout for
// the API mapping, context.DeadlineExceeded for the internal callers that test
// the chain. Folding the cause into the message left both unreachable.
func TestRunGitTypesTimeouts(t *testing.T) {
	repo := initTestRepo(t)
	ctx, cancel := expiredContext()
	defer cancel()

	_, err := runGit(ctx, repo, defaultMaxBuffer, "status", "--porcelain=v1")
	if err == nil {
		t.Fatal("expected an error from an expired context")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout so respondGitError maps it, got: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the deadline to survive as a wrapped cause, got: %v", err)
	}
}

// Operations other than GetWorkingDiff and GetFileContent return runGit errors
// straight to the API. Before timeouts were typed in runGit they arrived as a
// generic 500 while those two returned 504 — an inconsistency this exercises
// through an exported entry point rather than a hand-built error.
func TestGetFileDiffTypesTimeouts(t *testing.T) {
	repo := initTestRepo(t)
	commitFile(t, repo, "file.txt", "hello\n", "initial commit")
	ctx, cancel := expiredContext()
	defer cancel()

	if _, err := getFileDiff(ctx, repo, "file.txt", false, false); !errors.Is(err, ErrTimeout) {
		t.Errorf("tracked diff: expected ErrTimeout, got: %v", err)
	}
	if _, err := getFileDiff(ctx, repo, "file.txt", false, true); !errors.Is(err, ErrTimeout) {
		t.Errorf("untracked diff: expected ErrTimeout, got: %v", err)
	}
}

// A real git failure must keep its own diagnostic even when the deadline
// expires in the same instant: it exits with a status and writes stderr, which
// a killed process never does.
func TestRunGitPreservesGenuineFailures(t *testing.T) {
	repo := initTestRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	_, err := runGit(ctx, repo, defaultMaxBuffer, "rev-parse", "--verify", "no-such-ref")
	if err == nil {
		t.Fatal("expected an error for a bad revision")
	}
	if errors.Is(err, ErrTimeout) {
		t.Errorf("a genuine failure must not be typed as a timeout: %v", err)
	}
	if !strings.Contains(err.Error(), "Needed a single revision") {
		t.Errorf("expected git's own stderr to survive, got: %v", err)
	}
}

// A timeout says nothing about whether the file is in HEAD, so it must not be
// reported as a brand-new empty file — that is wrong data delivered as a 200,
// not a missing result.
func TestGetFileContentDoesNotReportTimeoutAsNewFile(t *testing.T) {
	repo := initTestRepo(t)
	commitFile(t, repo, "existing.txt", "committed\n", "initial commit")
	ctx, cancel := expiredContext()
	defer cancel()

	content, isNew, err := getFileContent(ctx, repo, "existing.txt")
	if err == nil {
		t.Fatalf("expected an error, got isNew=%v content=%q", isNew, content)
	}
	if isNew {
		t.Error("a timed-out read must not be classified as a new file")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the deadline to reach the caller, got: %v", err)
	}
}

// The numstat loop's failure mode is worse than the diff loop's: it reports
// Additions: 0 and a nil error, so a blown deadline would surface as a 200
// describing changes that are not the ones on disk. Exercised against the
// loop directly — driving it through getWorkingDiffFileStats would stop at the
// tracked name-status call and never reach this code.
func TestUntrackedFileStatsStopsOnExpiredContext(t *testing.T) {
	repo := initTestRepo(t)
	paths := []string{"a.txt", "b.txt", "c.txt"}
	for _, p := range paths {
		if err := writeTestFile(repo, p, "x\n"); err != nil {
			t.Fatal(err)
		}
	}

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(prev) })

	ctx, cancel := expiredContext()
	defer cancel()

	files, adds, err := untrackedFileStats(ctx, repo, paths)
	if err == nil {
		t.Fatalf("expected an error, got %d files with %d additions", len(files), adds)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected the deadline to reach the caller, got: %v", err)
	}
	if n := strings.Count(logs.String(), "warning: numstat for untracked"); n != 0 {
		t.Errorf("expected no per-file warnings before aborting, got %d", n)
	}
}

// Counting lines past the limit GetFileLines refuses to expand describes a
// button that cannot work, and an unbounded scan is what let a large working
// tree overrun the caller's deadline.
func TestCountFileLinesRefusesOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.txt")
	if err := os.WriteFile(path, []byte(sizedContent(int(maxFileSizeBytes)+4096)), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := countFileLines(context.Background(), path); !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge, got: %v", err)
	}

	small := filepath.Join(dir, "small.txt")
	if err := os.WriteFile(small, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	count, err := countFileLines(context.Background(), small)
	if err != nil || count != 2 {
		t.Errorf("expected 2 lines, got %d (%v)", count, err)
	}
}

// The line-count phase consults no context of its own, so without a check it
// would let a large working tree overrun the caller's budget unboundedly.
func TestComputeTotalLinesStopsOnExpiredContext(t *testing.T) {
	repo := initTestRepo(t)
	if err := writeTestFile(repo, "a.txt", "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	files := []CommitFile{{Path: "a.txt", Status: StatusAdded}}

	ctx, cancel := expiredContext()
	defer cancel()

	if got := computeTotalLines(ctx, repo, "", files); len(got) != 0 {
		t.Errorf("expected no counting past the deadline, got %v", got)
	}
	// The same input is counted when there is budget for it.
	if got := computeTotalLines(context.Background(), repo, "", files); got["a.txt"] != 2 {
		t.Errorf("expected 2 lines with a live context, got %v", got)
	}
}

// A single unterminated line is the shape that pits the scanner's token limit
// against the byte cap: the token limit must not fire first, or an in-limit
// file is dropped and an over-limit one is misreported as a scanner error
// rather than as ErrFileTooLarge.
// The scanner's buffer needs a byte over the cap: at exactly maxFileSizeBytes
// it cannot grow to look for the delimiter and reports ErrTooLong before
// observing EOF, so a single-line file within the limit would be dropped.
func TestCountFileLinesSingleLongLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one-line.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", int(maxFileSizeBytes))), 0644); err != nil {
		t.Fatal(err)
	}

	count, err := countFileLines(context.Background(), path)
	if err != nil {
		t.Fatalf("a single line exactly at the limit must count: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}
