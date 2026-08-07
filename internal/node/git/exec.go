package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const (
	shortTimeout           = 10 * time.Second
	longTimeout            = 30 * time.Second
	defaultMaxBuffer int64 = 10 * 1024 * 1024 // 10MB
	diffMaxBuffer    int64 = 10 * 1024 * 1024 // 10MB
)

// limitedWriter wraps a bytes.Buffer and fails fast when the output exceeds a
// configured limit, preventing unbounded memory growth from large git output.
// It serves both as a cmd.Stdout and, in getWorkingDiff, as a plain accumulator
// bounding the sum of several already-bounded commands.
//
// As a cmd.Stdout, failing the write closes the pipe under os/exec, which kills
// git with SIGPIPE — so cmd.Run reports "signal: broken pipe" and this writer's
// own error is lost. `exceeded` records the real reason; a caller writing
// directly gets it from Write instead.
type limitedWriter struct {
	buf      bytes.Buffer
	limit    int64
	exceeded bool
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(w.buf.Len())+int64(len(p)) > w.limit {
		w.exceeded = true
		return 0, fmt.Errorf("output exceeds %d bytes", w.limit)
	}
	return w.buf.Write(p)
}

// formatByteLimit renders a byte limit for user-facing error messages.
func formatByteLimit(n int64) string {
	if n >= 1024*1024 {
		return fmt.Sprintf("%d MB", n/(1024*1024))
	}
	return fmt.Sprintf("%d bytes", n)
}

// killedByContext reports whether ctx, rather than git itself, ended the
// command. It cannot be read off runErr alone: os/exec applies the context
// error only when the command otherwise succeeded, so a killed process arrives
// here as a plain ExitError with the cause erased.
//
// A done context is necessary but not sufficient — a git failure landing in the
// same instant would be relabelled a cancellation, discarding the stderr that
// says what to fix. The exit status rules that out: Cancel is Process.Kill, and
// a signal death never carries a normal exit status, so a status of 0 or above
// proves git chose its own ending. The test is one-way, since -1 is merely
// "died by signal" (a segfault or an OOM kill qualifies) and a nil state means
// the process never started, which rules out nothing.
func killedByContext(ctx context.Context, runErr error, state *os.ProcessState) bool {
	if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, context.Canceled) {
		return true
	}
	return ctx.Err() != nil && (state == nil || state.ExitCode() < 0)
}

// contextError types a command that its context ended. Only a deadline is a
// timeout: Fetch runs on the caller's request context, so a client disconnect
// or a shutdown arrives here as context.Canceled and must not claim a 504.
func contextError(ctx context.Context, what string) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s exceeded its deadline: %w", ErrTimeout, what, ctx.Err())
	}
	return fmt.Errorf("%s canceled: %w", what, ctx.Err())
}

// isOperationalError reports whether err describes the operation itself
// failing — a blown deadline, a cancellation, an output limit — rather than
// the object it asked about being absent or malformed. Callers that translate
// git failures into domain errors must let these through: respondGitError
// tests ErrNotFound and ErrInvalidInput ahead of ErrTimeout, so folding one in
// answers a request that never got an answer with a confident 404 or 400.
func isOperationalError(err error) bool {
	return errors.Is(err, ErrTimeout) ||
		errors.Is(err, ErrOutputTooLarge) ||
		errors.Is(err, ErrGitUnavailable) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled)
}

// runGit executes a git command in dir with the given context.
// It sets GIT_TERMINAL_PROMPT=0 and LC_ALL=C, and captures stderr for error messages.
// Output is bounded by maxBuffer — writes beyond the limit fail immediately.
func runGit(ctx context.Context, dir string, maxBuffer int64, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	stdout := &limitedWriter{limit: maxBuffer}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Check the limit first: it masquerades as a SIGPIPE death, and
		// git's stderr is empty in that case.
		if stdout.exceeded {
			return "", fmt.Errorf("%w: git %s produced more than %s", ErrOutputTooLarge, args[0], formatByteLimit(maxBuffer))
		}
		// Typed here rather than at each exported operation, so every caller
		// gets a 504 with a message instead of a bare 500. Both sentinels are
		// wrapped: ErrTimeout is what the API maps, context.DeadlineExceeded
		// is what the internal callers test.
		if killedByContext(ctx, err, cmd.ProcessState) {
			return "", contextError(ctx, fmt.Sprintf("git %s", args[0]))
		}
		// After the cancellation check, which owns the other way a command has
		// no process state: a deadline that expired before Start.
		if cmd.ProcessState == nil {
			return "", fmt.Errorf("%w: git %s: %w", ErrGitUnavailable, args[0], err)
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], errMsg)
	}

	return stdout.buf.String(), nil
}

// writeFile is a helper for writing content to a file path.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// isNotFoundError returns true when a runGit error indicates a missing git
// object (unknown revision, bad object, invalid ref) rather than an
// operational failure (timeout, corruption, permission denied).
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "bad object") ||
		strings.Contains(msg, "unknown revision") ||
		strings.Contains(msg, "Not a valid object")
}

var hashRegex = regexp.MustCompile(`^[a-f0-9]{7,40}$`)

// validateHash checks that a string looks like a git hash.
func validateHash(hash string) error {
	if !hashRegex.MatchString(hash) {
		return fmt.Errorf("%w: invalid commit hash: %q", ErrInvalidInput, hash)
	}
	return nil
}

// parseStatus maps a porcelain status character to a FileStatus.
func parseStatus(c byte) FileStatus {
	switch c {
	case 'M':
		return StatusModified
	case 'A':
		return StatusAdded
	case 'D':
		return StatusDeleted
	case 'R':
		return StatusRenamed
	case 'C':
		return StatusCopied
	case 'U':
		return StatusUnmerged
	default:
		return StatusModified
	}
}

// relativeTime converts a Unix timestamp to a human-readable relative time.
func relativeTime(unixTS int64) string {
	diff := time.Now().Unix() - unixTS
	if diff < 0 {
		diff = 0
	}

	const (
		minute = 60
		hour   = 60 * minute
		day    = 24 * hour
		week   = 7 * day
		month  = 30 * day
	)

	switch {
	case diff < minute:
		return "just now"
	case diff < hour:
		return fmt.Sprintf("%dm ago", diff/minute)
	case diff < day:
		return fmt.Sprintf("%dh ago", diff/hour)
	case diff < week:
		return fmt.Sprintf("%dd ago", diff/day)
	case diff < month:
		return fmt.Sprintf("%dw ago", diff/week)
	default:
		return fmt.Sprintf("%dmo ago", diff/month)
	}
}
