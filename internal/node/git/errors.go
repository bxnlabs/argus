package git

import "errors"

// ErrInvalidInput is returned when the caller provides syntactically invalid
// input (bad commit hash format, invalid branch name, etc.).
var ErrInvalidInput = errors.New("invalid input")

// ErrNotFound is returned when a referenced git object does not exist
// (nonexistent branch, unknown revision, etc.).
var ErrNotFound = errors.New("not found")

// ErrFileTooLarge is returned when a file exceeds the size limit for
// line-based reads (2 MB).
var ErrFileTooLarge = errors.New("file too large")

// ErrBinaryFile is returned when a binary file is detected during
// line-based reads.
var ErrBinaryFile = errors.New("binary file")

// ErrOutputTooLarge is returned when work exceeds one of this package's size
// bounds: a single command's stdout, the combined working diff, or the number
// of untracked files. Callers should surface the wrapped message: the limit is
// the user's only signal for why a diff won't render, and in the stdout case
// the failure otherwise reaches the client as "signal: broken pipe" (git is
// killed by SIGPIPE once the bounded writer closes the pipe) collapsed into a
// generic 500.
var ErrOutputTooLarge = errors.New("output too large")

// ErrTimeout is returned when a git operation exceeds its deadline. Callers
// should surface the wrapped message: a bare deadline error carries no
// attribution, so it reaches the client as a generic internal error blamed on
// whichever command happened to run once the clock ran out rather than on the
// work that actually consumed it.
var ErrTimeout = errors.New("timed out")

// ErrGitUnavailable is returned when the command never started — the working
// directory is gone, or git is not installed. Nothing it was asked about was
// determined, so callers that translate git failures into domain errors must
// let it through: a process that never ran cannot report that a file, ref or
// branch is absent. Git's own text cannot make this distinction, since it
// exits 128 for absence and for refusing to run alike; the process state can.
var ErrGitUnavailable = errors.New("git could not run")

// ErrFetchFailed is returned by Fetch when an underlying `git fetch` invocation
// exits non-zero (auth failure, network error, dead remote, etc.). Callers
// should surface the wrapped error's message rather than masking it as a
// generic internal error — the message is the user's only signal for what to
// fix (re-auth, check connectivity, remove a stale remote, …).
var ErrFetchFailed = errors.New("git fetch failed")
