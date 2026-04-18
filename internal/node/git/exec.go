package git

import (
	"bytes"
	"context"
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
	fetchTimeout           = 60 * time.Second
	defaultMaxBuffer int64 = 10 * 1024 * 1024 // 10MB
	diffMaxBuffer    int64 = 5 * 1024 * 1024  // 5MB
)

// limitedWriter wraps a bytes.Buffer and fails fast when the output exceeds
// a configured limit, preventing unbounded memory growth from large git output.
type limitedWriter struct {
	buf   bytes.Buffer
	limit int64
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if int64(w.buf.Len())+int64(len(p)) > w.limit {
		return 0, fmt.Errorf("output exceeds %d bytes", w.limit)
	}
	return w.buf.Write(p)
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
