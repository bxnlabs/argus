package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitInDir runs a git command in dir and fails the test on non-zero exit.
func gitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// initTestRepo creates a git repo with one commit for testing.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitInDir(t, dir, "init", "-b", "main")
	gitInDir(t, dir, "config", "user.email", "test@test.com")
	gitInDir(t, dir, "config", "user.name", "Test")
	return dir
}

// cloneTestRepo clones remote into a fresh temp dir and configures the local
// user identity (required for any subsequent commits in the clone).
func cloneTestRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v\n%s", err, out)
	}
	gitInDir(t, dir, "config", "user.email", "test@test.com")
	gitInDir(t, dir, "config", "user.name", "Test")
	return dir
}

// commitFile creates a file and commits it in the test repo.
func commitFile(t *testing.T, dir, name, content, message string) {
	t.Helper()
	if err := writeTestFile(dir, name, content); err != nil {
		t.Fatal(err)
	}
	gitInDir(t, dir, "add", name)
	gitInDir(t, dir, "commit", "-m", message)
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
		"abcdef1", // 7 chars
		"abcdef1234567890abcdef1234567890abcdef12", // 40 chars
		"1234567890abcdef1234",                     // 20 chars
	}
	for _, h := range valid {
		if err := validateHash(h); err != nil {
			t.Errorf("validateHash(%q) = %v, want nil", h, err)
		}
	}

	invalid := []string{
		"abc",              // too short
		"ABCDEF1",          // uppercase
		"abcdefg",          // non-hex
		"abc123; rm -rf /", // injection
		"$(whoami)",        // injection
		"abc|grep",         // injection
		"",                 // empty
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

	// Oversized output makes limitedWriter fail the copy, which closes the pipe
	// and kills git with SIGPIPE. Without an explicit check, cmd.Run's
	// "signal: broken pipe" masks the real cause and the caller can only report
	// a generic internal error.
	t.Run("output over limit reports the limit, not a signal", func(t *testing.T) {
		repo := initTestRepo(t)
		commitFile(t, repo, "big.txt", strings.Repeat("x=1\n", 300_000), "big")

		ctx := context.Background()
		_, err := runGit(ctx, repo, 1024, "show", "HEAD:big.txt")
		if err == nil {
			t.Fatal("expected error for oversized output")
		}
		if !errors.Is(err, ErrOutputTooLarge) {
			t.Errorf("expected ErrOutputTooLarge, got: %v", err)
		}
		if strings.Contains(err.Error(), "broken pipe") {
			t.Errorf("oversized output masked as a signal error: %v", err)
		}
		// Assert against the limit this call actually passed, not a hardcoded
		// string: otherwise dropping formatByteLimit(maxBuffer) from the real
		// error would still pass.
		if !strings.Contains(err.Error(), "1024 bytes") {
			t.Errorf("expected error to name the limit it was given, got: %v", err)
		}
	})
}

func TestFormatByteLimit(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{1024, "1024 bytes"},
		{5 * 1024 * 1024, "5 MB"},
		{10 * 1024 * 1024, "10 MB"},
	}
	for _, tt := range tests {
		if got := formatByteLimit(tt.n); got != tt.want {
			t.Errorf("formatByteLimit(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

// The discriminator behind timeout typing. os/exec erases the cause when the
// command exits non-zero, so a killed process is indistinguishable from a
// failed one by runErr alone.
func TestKilledByContext(t *testing.T) {
	repo := initTestRepo(t)
	commitFile(t, repo, "file.txt", "hello\n", "initial commit")

	t.Run("killed by its deadline", func(t *testing.T) {
		// cat-file --batch blocks reading requests from stdin and the write end
		// stays open, so the deadline is the only thing that can end it. A
		// short-lived command here would sometimes finish first.
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer pw.Close()
		defer pr.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
		cmd.Dir = repo
		cmd.Stdin = pr
		runErr := cmd.Run()

		if runErr == nil {
			t.Fatal("premise: the command should not have exited on its own")
		}
		if !killedByContext(ctx, runErr, cmd.ProcessState) {
			t.Errorf("expected the context kill to be detected, got runErr: %v", runErr)
		}
	})

	t.Run("never started", func(t *testing.T) {
		ctx, cancel := expiredContext()
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "status")
		cmd.Dir = repo
		runErr := cmd.Run()

		if !killedByContext(ctx, runErr, cmd.ProcessState) {
			t.Errorf("expected an expired deadline at Start to count, got: %v", runErr)
		}
	})

	// The case the exit-code heuristic got wrong: a real git failure must keep
	// its own diagnostic even if the deadline expires immediately afterwards.
	// The context is cancelled *after* git has already failed on its own, so
	// only the exit status can tell the two apart — a done context alone would
	// claim this one and discard the stderr that says what to fix.
	t.Run("genuine failure is not a cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "no-such-ref")
		cmd.Dir = repo
		runErr := cmd.Run()
		cancel()

		if runErr == nil || cmd.ProcessState.ExitCode() <= 0 {
			t.Fatalf("premise: expected a natural non-zero exit, got %v", runErr)
		}
		if killedByContext(ctx, runErr, cmd.ProcessState) {
			t.Error("a genuine git failure must not be reported as a cancellation")
		}
	})
}

// A caller-cancelled request is not a timeout. Fetch runs on the HTTP request
// context, so a client disconnect must not claim the server blew a deadline.
func TestContextErrorSeparatesCancelFromDeadline(t *testing.T) {
	deadline, cancelDeadline := expiredContext()
	defer cancelDeadline()
	if err := contextError(deadline, "git status"); !errors.Is(err, ErrTimeout) {
		t.Errorf("a deadline must be typed ErrTimeout, got: %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err := contextError(canceled, "git status")
	if errors.Is(err, ErrTimeout) {
		t.Errorf("a cancellation must not claim a deadline: %v", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled to survive, got: %v", err)
	}
}

// Callers that translate git failures into domain errors must let operational
// ones through. respondGitError tests ErrNotFound and ErrInvalidInput ahead of
// ErrTimeout, so a folded-in deadline answers with a confident 404 or 400 for
// a request that never actually got an answer.
func TestIsOperationalError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"timeout", fmt.Errorf("%w: git status exceeded", ErrTimeout), true},
		{"output limit", fmt.Errorf("%w: too big", ErrOutputTooLarge), true},
		{"cancellation", fmt.Errorf("git status canceled: %w", context.Canceled), true},
		{"never ran", fmt.Errorf("%w: git show: chdir", ErrGitUnavailable), true},
		{"missing object", fmt.Errorf("git cat-file: bad object"), false},
		{"nil", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isOperationalError(tc.err); got != tc.want {
				t.Errorf("isOperationalError = %v, want %v", got, tc.want)
			}
		})
	}
}

// A command that never started determined nothing, so no caller may read it as
// the object being absent. git cannot report this itself — it exits 128 both
// for absence and for refusing to run — but a process that never started has
// no exit status at all, which is what separates the two.
func TestCommandThatNeverRanIsNotAnAnswer(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "deleted-worktree")

	t.Run("runGit types it", func(t *testing.T) {
		_, err := runGit(context.Background(), gone, defaultMaxBuffer, "rev-parse", "--git-dir")
		if !errors.Is(err, ErrGitUnavailable) {
			t.Fatalf("expected ErrGitUnavailable, got: %v", err)
		}
		if !isOperationalError(err) {
			t.Error("a command that never ran must be operational")
		}
	})

	// The three conversions that would otherwise answer confidently.
	t.Run("not reported as a new file", func(t *testing.T) {
		_, isNew, err := getFileContent(context.Background(), gone, "committed.txt")
		if err == nil {
			t.Fatalf("expected an error, got isNew=%v for a worktree that is gone", isNew)
		}
		if isNew {
			t.Error("a committed file must not be reported as new because the worktree vanished")
		}
	})

	t.Run("not reported as an invalid branch name", func(t *testing.T) {
		err := validateBranchRef(context.Background(), gone, "perfectly-fine-name")
		if errors.Is(err, ErrInvalidInput) {
			t.Errorf("a vanished worktree must not answer 400: %v", err)
		}
		if !errors.Is(err, ErrGitUnavailable) {
			t.Errorf("expected ErrGitUnavailable, got: %v", err)
		}
	})

	t.Run("not reported as not-a-repo", func(t *testing.T) {
		isRepo, err := check(context.Background(), gone)
		if err == nil {
			t.Fatalf("expected an error, got isGitRepo=%v", isRepo)
		}
	})
}

// A branch name is not invalid just because the check timed out; answering 400
// would blame the user for a failure that was ours.
func TestValidateBranchRefDoesNotBlameTheUserForTimeouts(t *testing.T) {
	repo := initTestRepo(t)
	commitFile(t, repo, "file.txt", "hello\n", "initial commit")
	ctx, cancel := expiredContext()
	defer cancel()

	err := validateBranchRef(ctx, repo, "main")
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
	if errors.Is(err, ErrInvalidInput) {
		t.Errorf("a timeout must not be reported as invalid input: %v", err)
	}
}

// Building the message from err.Error() dropped the chain, so a fetch that
// timed out matched only ErrFetchFailed and answered 502 instead of 504.
func TestWrapFetchErrorPreservesCause(t *testing.T) {
	timeout := fmt.Errorf("%w: git fetch exceeded its deadline: %w", ErrTimeout, context.DeadlineExceeded)
	err := wrapFetchError("origin", timeout)

	if !errors.Is(err, ErrFetchFailed) {
		t.Errorf("expected ErrFetchFailed to still match, got: %v", err)
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout to survive the wrap, got: %v", err)
	}
	if !strings.Contains(err.Error(), "origin") {
		t.Errorf("expected the remote in the message, got: %v", err)
	}
}

// Reporting "no upstream" for a blown deadline leaves the caller with no
// targets, which Fetch reports as a successful fetch that never ran.
func TestUpstreamRemoteDoesNotReportTimeoutAsNoUpstream(t *testing.T) {
	repo := initTestRepo(t)
	commitFile(t, repo, "file.txt", "hello\n", "initial commit")
	remotes := map[string]struct{}{"origin": {}}

	// A branch with no upstream is an ordinary "none", not an error.
	remote, err := upstreamRemote(context.Background(), repo, "HEAD", remotes)
	if err != nil || remote != "" {
		t.Errorf("expected no upstream and no error, got %q / %v", remote, err)
	}

	ctx, cancel := expiredContext()
	defer cancel()
	remote, err = upstreamRemote(ctx, repo, "HEAD", remotes)
	if err == nil {
		t.Fatalf("expected an error rather than %q", remote)
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got: %v", err)
	}
}

// rev-parse rejecting the directory is the answer "no"; the command never
// completing is not. Collapsing both reported isGitRepo=false with a 200.
func TestCheckSeparatesNotARepoFromTimeout(t *testing.T) {
	repo := initTestRepo(t)

	if ok, err := check(context.Background(), repo); !ok || err != nil {
		t.Errorf("expected a git repo, got ok=%v err=%v", ok, err)
	}
	if ok, err := check(context.Background(), t.TempDir()); ok || err != nil {
		t.Errorf("expected a plain false for a non-repo, got ok=%v err=%v", ok, err)
	}

	ctx, cancel := expiredContext()
	defer cancel()
	ok, err := check(ctx, repo)
	if ok {
		t.Error("expected false on a blown deadline")
	}
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout rather than a silent false, got: %v", err)
	}
}
