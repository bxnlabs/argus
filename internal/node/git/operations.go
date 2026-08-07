package git

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const (
	maxFileSizeBytes = 2 * 1024 * 1024 // 2 MB
	maxLineSpan      = 500

	// maxUntrackedFiles bounds the per-file work GetWorkingDiff does. Only the
	// `git status` output was otherwise bounded, and at 10 MB that admits
	// roughly 170k-800k paths depending on path length — each costing a diff
	// process in appendUntrackedDiffs, a numstat process in untrackedFileStats,
	// and a file read in computeTotalLines, on every poll.
	maxUntrackedFiles = 5000

	// gitignoreHint is appended to the limit errors whose most likely cause is
	// a large untracked directory. GitPanel renders these messages verbatim, so
	// the limit alone would leave the user with nothing to act on.
	gitignoreHint = "a large untracked directory (e.g. node_modules) may need to be gitignored"
)

var fullHexOIDRegex = regexp.MustCompile(`^[0-9a-f]{40}$`)

// GetStatus returns the git status for a directory.
// Matches lib/git-status.ts:getGitStatus().
func GetStatus(dir string) (*GitStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	// Get current branch
	branch, err := runGit(ctx, dir, defaultMaxBuffer, "branch", "--show-current")
	if err != nil {
		return nil, err
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "HEAD" // detached HEAD
	}

	// Get ahead/behind counts (may fail if no upstream)
	ahead, behind := 0, 0
	if out, err := runGit(ctx, dir, defaultMaxBuffer, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
		parts := strings.Fields(strings.TrimSpace(out))
		if len(parts) == 2 {
			behind, _ = strconv.Atoi(parts[0])
			ahead, _ = strconv.Atoi(parts[1])
		}
	}

	// Get porcelain status. -uall enumerates individual files inside untracked
	// directories rather than collapsing them to a single directory entry.
	out, err := runGit(ctx, dir, defaultMaxBuffer, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}

	staged := []GitFile{}
	unstaged := []GitFile{}
	untracked := []GitFile{}

	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}

		indexStatus := line[0]
		workTreeStatus := line[1]
		filePath := line[3:]

		// Handle renames: "old -> new"
		var oldPath string
		if idx := strings.Index(filePath, " -> "); idx != -1 {
			oldPath = filePath[:idx]
			filePath = filePath[idx+4:]
		}

		// Untracked
		if indexStatus == '?' && workTreeStatus == '?' {
			untracked = append(untracked, GitFile{
				Path:   filePath,
				Status: StatusUntracked,
				Staged: false,
			})
			continue
		}

		// Staged (index has changes)
		if indexStatus != ' ' && indexStatus != '?' {
			staged = append(staged, GitFile{
				Path:    filePath,
				OldPath: oldPath,
				Status:  parseStatus(indexStatus),
				Staged:  true,
			})
		}

		// Unstaged (worktree has changes)
		if workTreeStatus != ' ' && workTreeStatus != '?' {
			unstaged = append(unstaged, GitFile{
				Path:    filePath,
				OldPath: oldPath,
				Status:  parseStatus(workTreeStatus),
				Staged:  false,
			})
		}
	}

	return &GitStatus{
		Branch:    branch,
		Ahead:     ahead,
		Behind:    behind,
		Staged:    staged,
		Unstaged:  unstaged,
		Untracked: untracked,
	}, nil
}

// GetFileDiff returns the diff for a file.
// Matches lib/git-status.ts:getFileDiff() and getUntrackedFileDiff().
func GetFileDiff(dir, file string, staged, untracked bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	return getFileDiff(ctx, dir, file, staged, untracked)
}

// getFileDiff is GetFileDiff with a caller-supplied context. Batch callers must
// use this rather than GetFileDiff: a fresh longTimeout per file lets a loop
// outrun the deadline its caller set, and the failure then surfaces from
// whichever command runs next, well away from the loop that burned the time.
// --no-ext-diff on every patch-producing diff here and in getWorkingDiff: a
// configured diff.external replaces git's output with the driver's, which the
// client's parser cannot read, and a driver that exits 0 silently yields an
// empty diff reported as a success.
func getFileDiff(ctx context.Context, dir, file string, staged, untracked bool) (string, error) {
	if untracked {
		return runGitNoIndex(ctx, dir, fmt.Sprintf("git diff of %q", file), diffMaxBuffer,
			"diff", "--no-ext-diff", "-U20", "--no-index", "/dev/null", file)
	}

	var args []string
	if staged {
		args = []string{"diff", "--no-ext-diff", "-U3", "--cached", "--", file}
	} else {
		args = []string{"diff", "--no-ext-diff", "-U3", "--", file}
	}

	return runGit(ctx, dir, diffMaxBuffer, args...)
}

// runGitNoIndex runs a `git diff --no-index` command and returns stdout even
// when git exits 1, which here means "files differ" — the expected result, not
// a failure. runGit cannot be used for these: it discards stdout on any
// non-zero exit. what names the work in error messages.
func runGitNoIndex(ctx context.Context, dir, what string, maxBuffer int64, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	stdout := &limitedWriter{limit: maxBuffer}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.buf.String(), nil
	}
	// The limit first: an over-limit run is killed by SIGPIPE (see
	// limitedWriter), so it arrives with an empty stderr and a signal status
	// that explains nothing.
	if stdout.exceeded {
		return "", fmt.Errorf("%w: %s produced more than %s", ErrOutputTooLarge, what, formatByteLimit(maxBuffer))
	}
	// Cancellation before the exit code, so a kill is never mistaken for
	// "files differ" and a partial diff returned as a success.
	if killedByContext(ctx, err, cmd.ProcessState) {
		return "", contextError(ctx, what)
	}
	if cmd.ProcessState == nil {
		return "", fmt.Errorf("%w: %s: %w", ErrGitUnavailable, what, err)
	}
	// Exit 1 means "files differ" only when a diff came with it: git reports
	// "could not access" the same way, and accepting the status alone returns a
	// vanished path as an answered request with no changes. Every real diff
	// writes something, given --no-ext-diff below.
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 && stdout.buf.Len() > 0 {
		return stdout.buf.String(), nil
	}
	errMsg := strings.TrimSpace(stderr.String())
	if errMsg == "" {
		errMsg = err.Error()
	}
	return "", fmt.Errorf("%s: %s", what, errMsg)
}

// GetFileContent returns the HEAD version of a file.
// Returns (content, isNew, error). isNew=true if file doesn't exist in HEAD.
func GetFileContent(dir, file string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	return getFileContent(ctx, dir, file)
}

// getFileContent is GetFileContent with a caller-supplied context.
func getFileContent(ctx context.Context, dir, file string) (string, bool, error) {
	content, err := runGit(ctx, dir, diffMaxBuffer, "show", fmt.Sprintf("HEAD:%s", file))
	if err != nil {
		// The catch-all below reads any remaining failure as "not in HEAD",
		// so anything that did not actually determine that has to leave
		// first — otherwise it is reported as a new empty file, which is
		// wrong data rather than a missing answer. Operational failures are
		// filtered by cause, not by message, so a git that ran and failed for
		// its own reasons (a corrupt object, an unreadable .git) still lands
		// in the catch-all. Narrowing that further needs per-command parsing
		// of git's output.
		if isOperationalError(err) {
			return "", false, err
		}
		// File not in HEAD = new file
		return "", true, nil
	}
	return content, false, nil
}

// Check reports whether dir is inside a git work tree.
func Check(dir string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	return check(ctx, dir)
}

// check is Check with a caller-supplied context.
func check(ctx context.Context, dir string) (bool, error) {
	// rev-parse rejecting the directory is the answer "no"; the command never
	// completing is not. Collapsing both reports isGitRepo=false with a 200.
	if _, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--git-dir"); err != nil {
		if isOperationalError(err) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// errCombinedDiffTooLarge reports a combined working diff over the limit. The
// hint is omitted when there are no untracked files, where the cause is instead
// two large tracked diffs and gitignore is no help.
func errCombinedDiffTooLarge(untracked int) error {
	msg := fmt.Sprintf("combined working diff exceeds %s", formatByteLimit(diffMaxBuffer))
	if untracked > 0 {
		msg += fmt.Sprintf(" (%d untracked files); %s", untracked, gitignoreHint)
	}
	return fmt.Errorf("%w: %s", ErrOutputTooLarge, msg)
}

// asTimeoutError re-attributes a blown deadline to the working diff as a
// whole. GetWorkingDiff runs many commands, so the one that reports the
// deadline is rarely the one that consumed it; naming the overall work is more
// use to the caller than naming that command. A size failure is left alone as
// the more specific diagnosis.
//
// The test is the error chain, not ctx.Err(), so a failure that merely lands
// just before expiry keeps its own message. The cause stays wrapped so the
// command that reported the deadline is still reachable behind the summary.
func asTimeoutError(err error) error {
	if err == nil || errors.Is(err, ErrOutputTooLarge) || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: computing the working diff exceeded %s; %s: %w",
		ErrTimeout, longTimeout, gitignoreHint, err)
}

// GetWorkingDiff returns the full combined diff for all working-tree changes
// (staged, unstaged, and untracked), along with per-file metadata.
func GetWorkingDiff(dir string) (*WorkingDiffResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	res, err := getWorkingDiff(ctx, dir)
	return res, asTimeoutError(err)
}

func getWorkingDiff(ctx context.Context, dir string) (*WorkingDiffResult, error) {
	hasHEAD := true
	if _, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--verify", "HEAD"); err != nil {
		hasHEAD = false
	}

	// Status is read before any diff work so the untracked count can gate it.
	// -uall enumerates individual files inside untracked directories.
	statusOut, err := runGit(ctx, dir, defaultMaxBuffer, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}

	var untrackedPaths []string
	for _, line := range strings.Split(statusOut, "\n") {
		if len(line) < 4 {
			continue
		}
		if line[0] == '?' && line[1] == '?' {
			untrackedPaths = append(untrackedPaths, line[3:])
		}
	}

	if len(untrackedPaths) > maxUntrackedFiles {
		return nil, fmt.Errorf("%w: %d untracked files exceeds the limit of %d; %s",
			ErrOutputTooLarge, len(untrackedPaths), maxUntrackedFiles, gitignoreHint)
	}

	// Each git command below is bounded on its own, but nothing bounded their
	// sum. Accumulating into one limitedWriter caps the total, and avoids the
	// extra full-size copy a closing strings.Join would allocate.
	combined := &limitedWriter{limit: diffMaxBuffer}
	addPart := func(s string) error {
		if s == "" {
			return nil
		}
		if combined.buf.Len() > 0 {
			if _, err := combined.Write([]byte("\n")); err != nil {
				return errCombinedDiffTooLarge(len(untrackedPaths))
			}
		}
		if _, err := combined.Write([]byte(s)); err != nil {
			return errCombinedDiffTooLarge(len(untrackedPaths))
		}
		return nil
	}

	if hasHEAD {
		trackedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--no-ext-diff", "-U3", "HEAD")
		if err != nil {
			return nil, err
		}
		if err := addPart(trackedDiff); err != nil {
			return nil, err
		}
	} else {
		cachedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--no-ext-diff", "-U3", "--cached")
		if err != nil {
			return nil, err
		}
		unstagedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "--no-ext-diff", "-U3")
		if err != nil {
			return nil, err
		}
		// Two parts rather than a pre-concatenated one, so the pair and their
		// join are never held at once. addPart skips empties and separates the
		// rest with "\n", so the resulting bytes — and the fingerprint clients
		// compare against — are unchanged.
		if err := addPart(cachedDiff); err != nil {
			return nil, err
		}
		if err := addPart(unstagedDiff); err != nil {
			return nil, err
		}
	}

	if err := appendUntrackedDiffs(ctx, dir, untrackedPaths, addPart); err != nil {
		return nil, err
	}

	combinedDiff := combined.buf.String()

	files, totalAdds, totalDels, err := getWorkingDiffFileStats(ctx, dir, hasHEAD, untrackedPaths)
	if err != nil {
		return nil, err
	}

	// Compute per-file total line counts from working tree
	fileTotalLines := computeTotalLines(ctx, dir, "", files)

	// Fingerprint: SHA-256 of raw diff for staleness detection
	h := sha256.Sum256([]byte(combinedDiff))
	fingerprint := fmt.Sprintf("%x", h)

	return &WorkingDiffResult{
		Diff:           combinedDiff,
		Files:          files,
		TotalAdditions: totalAdds,
		TotalDeletions: totalDels,
		TotalLines:     fileTotalLines,
		Fingerprint:    fingerprint,
	}, nil
}

// appendUntrackedDiffs diffs each untracked path into addPart.
func appendUntrackedDiffs(ctx context.Context, dir string, paths []string, addPart func(string) error) error {
	for _, path := range paths {
		fileDiff, err := getFileDiff(ctx, dir, path, false, true)
		if err != nil {
			// Operational failures are fatal, as they are for the tracked
			// diffs. Skipping the file would return 200 with it listed in
			// Files but missing from Diff — and a fingerprint over that.
			if isOperationalError(err) {
				return err
			}
			// Other errors stay non-fatal: a file listed by status can
			// legitimately vanish before its diff runs.
			log.Printf("warning: diff for untracked %q: %v", path, err)
			continue
		}
		if err := addPart(fileDiff); err != nil {
			return err
		}
	}
	return nil
}

func getWorkingDiffFileStats(ctx context.Context, dir string, hasHEAD bool, untrackedPaths []string) ([]CommitFile, int, int, error) {
	var files []CommitFile
	totalAdds, totalDels := 0, 0

	if hasHEAD {
		statusMap := map[string]struct {
			status  string
			oldPath string
		}{}

		nsOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--name-status", "HEAD")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("name-status: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(nsOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			statusChar := fields[0]
			if len(fields) == 3 {
				statusMap[fields[2]] = struct {
					status  string
					oldPath string
				}{statusChar[:1], fields[1]}
			} else {
				statusMap[fields[1]] = struct {
					status  string
					oldPath string
				}{statusChar[:1], ""}
			}
		}

		numOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--numstat", "HEAD")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("numstat: %w", err)
		}
		for _, line := range strings.Split(strings.TrimSpace(numOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) != 3 {
				continue
			}

			adds, dels := 0, 0
			isBinary := fields[0] == "-" && fields[1] == "-"
			if !isBinary {
				adds, _ = strconv.Atoi(fields[0])
				dels, _ = strconv.Atoi(fields[1])
			}

			path := normalizeNumstatPath(fields[2])

			st := StatusModified
			var oldPath string
			if info, ok := statusMap[path]; ok {
				switch info.status {
				case "A":
					st = StatusAdded
				case "D":
					st = StatusDeleted
				case "R":
					st = StatusRenamed
					oldPath = info.oldPath
				case "C":
					st = StatusCopied
				}
			}

			files = append(files, CommitFile{
				Path:      path,
				Status:    st,
				Additions: adds,
				Deletions: dels,
				OldPath:   oldPath,
			})
			totalAdds += adds
			totalDels += dels
		}
	} else {
		nsOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--name-status", "--cached")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("name-status --cached: %w", err)
		}
		numOut, err := runGit(ctx, dir, defaultMaxBuffer, "diff", "--numstat", "--cached")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("numstat --cached: %w", err)
		}

		cachedStatusMap := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(nsOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) >= 2 {
				cachedStatusMap[fields[len(fields)-1]] = fields[0][:1]
			}
		}

		for _, line := range strings.Split(strings.TrimSpace(numOut), "\n") {
			if line == "" {
				continue
			}
			fields := strings.SplitN(line, "\t", 3)
			if len(fields) != 3 {
				continue
			}
			adds, dels := 0, 0
			if fields[0] != "-" {
				adds, _ = strconv.Atoi(fields[0])
				dels, _ = strconv.Atoi(fields[1])
			}
			path := normalizeNumstatPath(fields[2])
			st := StatusAdded
			if s, ok := cachedStatusMap[path]; ok && s == "M" {
				st = StatusModified
			}
			files = append(files, CommitFile{
				Path:      path,
				Status:    st,
				Additions: adds,
				Deletions: dels,
			})
			totalAdds += adds
			totalDels += dels
		}
	}

	untrackedFiles, untrackedAdds, err := untrackedFileStats(ctx, dir, untrackedPaths)
	if err != nil {
		return nil, 0, 0, err
	}
	files = append(files, untrackedFiles...)
	totalAdds += untrackedAdds

	if files == nil {
		files = []CommitFile{}
	}
	return files, totalAdds, totalDels, nil
}

// untrackedFileStats returns one StatusAdded entry per untracked path, with
// its addition count.
func untrackedFileStats(ctx context.Context, dir string, untrackedPaths []string) ([]CommitFile, int, error) {
	var files []CommitFile
	totalAdds := 0

	for _, path := range untrackedPaths {
		adds := 0
		out, err := runGitNoIndex(ctx, dir, fmt.Sprintf("git numstat of %q", path), defaultMaxBuffer,
			"diff", "--numstat", "--no-index", "/dev/null", "--", path)
		switch {
		case err == nil:
			if fields := strings.SplitN(strings.TrimSpace(out), "\t", 3); fields[0] != "-" {
				adds, _ = strconv.Atoi(fields[0])
			}
		case isOperationalError(err):
			// Fatal, unlike the transient failures below. Continuing does not
			// degrade the result, it falsifies it: every remaining file would
			// be reported with 0 additions and a nil error, so a blown deadline
			// would reach the client as a 200 describing changes that are not
			// the ones on disk.
			return nil, 0, err
		default:
			log.Printf("warning: numstat for untracked %q: %v", path, err)
		}
		files = append(files, CommitFile{
			Path:      path,
			Status:    StatusAdded,
			Additions: adds,
		})
		totalAdds += adds
	}

	return files, totalAdds, nil
}

// openRegular opens path without following a final symlink and without
// blocking on a special file.
//
// The syscall flags carry no build tag on purpose: argus is unix-only anyway
// (cmd/argus/cli calls syscall.Kill untagged, sessions are tmux), so a non-unix
// fallback would only be portability the binary never had.
//
// os.Open blocks indefinitely on a FIFO, waiting for a writer that may never
// arrive — a hang no deadline in this package can interrupt. Checking the type
// by path first cannot close that window, since the path can be replaced
// between the check and the open. O_NONBLOCK returns immediately whatever it
// finds, so the caller can inspect the descriptor it actually got. O_NOFOLLOW
// is the separate half the descriptor check does not cover, since a symlink to
// a regular file passes it; the cost is that a symlinked file gets no line
// count.
func openRegular(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(fd), path), nil
}

// countFileLines returns the logical line count of a file on disk using
// a scanner. This counts lines yielded by bufio.Scanner (including a final
// unterminated line), which may differ from wc -l for files without a
// trailing newline.
func countFileLines(ctx context.Context, path string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	f, err := openRegular(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Stat the descriptor, never the path. Validating by name and then opening
	// by name are two different files whenever something rewrites the working
	// tree in between — routine here, since the poller runs while an agent edits.
	fi, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if !fi.Mode().IsRegular() {
		return 0, fmt.Errorf("not a regular file: %s", path)
	}
	// Cheap rejection, and the only path that knows the actual size. This is
	// the limit GetFileLines refuses to expand past, so a count above it would
	// only describe a button that cannot work.
	if fi.Size() > maxFileSizeBytes {
		return 0, fmt.Errorf("%w: %s (%d bytes)", ErrFileTooLarge, path, fi.Size())
	}

	// That size is a snapshot, not a bound — the file can grow after the fstat
	// — and Scanner.Buffer caps a token rather than the total, so this is what
	// actually bounds the read. One byte of headroom in both, so a single line
	// at exactly the cap still counts instead of reporting ErrTooLong.
	limited := &io.LimitedReader{R: f, N: maxFileSizeBytes + 1}

	count := 0
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 64*1024), maxFileSizeBytes+1)
	for scanner.Scan() {
		count++
	}
	// Before scanner.Err, which may report ErrTooLong for the same file.
	if limited.N == 0 {
		return 0, fmt.Errorf("%w: %s (over %d bytes)", ErrFileTooLarge, path, maxFileSizeBytes)
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

// countBlobLines returns the logical line count of a git blob using
// git cat-file blob streaming.
func countBlobLines(ctx context.Context, dir, ref, file string) (int, error) {
	out, err := runGit(ctx, dir, diffMaxBuffer, "cat-file", "blob", ref+":"+file)
	if err != nil {
		return 0, err
	}
	if out == "" {
		return 0, nil
	}
	count := 0
	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 64*1024), maxFileSizeBytes)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// computeTotalLines builds a map of file path -> total line count for a set
// of changed files. For working tree mode (ref==""), it reads from disk.
// For ref-based mode, it reads from the git object at the given ref.
// Deleted files are keyed by their old path. Renames use the new path.
func computeTotalLines(ctx context.Context, dir, ref string, files []CommitFile) map[string]int {
	totalLines := make(map[string]int, len(files))
	for _, f := range files {
		if f.Status == StatusDeleted {
			// Deleted files: use old path as key, skip counting (no postimage)
			continue
		}
		if f.Status == StatusUnmerged {
			continue
		}

		// This map is best-effort — a missing entry only costs an expand
		// button — so a blown deadline stops the work rather than failing the
		// request that already assembled a valid diff. Checking per file keeps
		// the overrun to at most one file's scan.
		if ctx.Err() != nil {
			break
		}

		path := f.Path
		var count int
		var err error
		if ref == "" {
			count, err = countFileLines(ctx, filepath.Join(dir, path))
		} else {
			count, err = countBlobLines(ctx, dir, ref, path)
		}
		if err != nil {
			// Non-fatal: file may not exist (e.g. binary, submodule).
			// Skip silently — frontend will just not show expand buttons.
			continue
		}
		totalLines[path] = count
	}
	return totalLines
}

// GetFileLines returns a range of lines from a file, either from the working
// tree (ref=="") or from a git object at the specified commit OID.
func GetFileLines(dir, file string, start, end int, ref string) (*FileLinesResult, error) {
	if start < 1 {
		return nil, fmt.Errorf("%w: start must be >= 1", ErrInvalidInput)
	}
	if end < start {
		return nil, fmt.Errorf("%w: end must be >= start", ErrInvalidInput)
	}
	if end-start+1 > maxLineSpan {
		return nil, fmt.Errorf("%w: line span exceeds maximum of %d", ErrInvalidInput, maxLineSpan)
	}

	if ref == "" {
		return getFileLinesFromDisk(dir, file, start, end)
	}
	return getFileLinesFromRef(dir, file, start, end, ref)
}

func getFileLinesFromDisk(dir, file string, start, end int) (*FileLinesResult, error) {
	fullPath := filepath.Join(dir, file)

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, file)
		}
		return nil, err
	}
	if info.Size() > maxFileSizeBytes {
		return nil, fmt.Errorf("%w: %s (%d bytes)", ErrFileTooLarge, file, info.Size())
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	totalLines := 0
	lineNum := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxFileSizeBytes)

	for scanner.Scan() {
		lineNum++
		totalLines = lineNum
		if lineNum < start {
			continue
		}
		if lineNum > end {
			// Keep scanning to get totalLines
			continue
		}
		line := scanner.Text()
		// Binary detection: check for null bytes
		if bytes.ContainsRune([]byte(line), 0) {
			return nil, fmt.Errorf("%w: %s", ErrBinaryFile, file)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(lines) == 0 && start > totalLines {
		return nil, fmt.Errorf("%w: start line %d beyond file length %d", ErrInvalidInput, start, totalLines)
	}

	actualEnd := start + len(lines) - 1

	return &FileLinesResult{
		Lines:      lines,
		Start:      start,
		End:        actualEnd,
		TotalLines: totalLines,
	}, nil
}

func getFileLinesFromRef(dir, file string, start, end int, ref string) (*FileLinesResult, error) {
	// Validate ref is a full hex OID
	if !fullHexOIDRegex.MatchString(ref) {
		return nil, fmt.Errorf("%w: ref must be a full 40-character hex OID", ErrInvalidInput)
	}

	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	// Validate ref is a commit
	objType, err := runGit(ctx, dir, defaultMaxBuffer, "cat-file", "-t", ref)
	if err != nil {
		// A blown deadline folded into the catch-all below would answer a
		// confident 404 claiming the ref does not exist.
		if isOperationalError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: ref %q", ErrNotFound, ref)
	}
	if strings.TrimSpace(objType) != "commit" {
		return nil, fmt.Errorf("%w: ref %q is not a commit", ErrInvalidInput, ref)
	}

	// Resolve file to blob OID via rev-parse <ref>:<file>
	blobOID, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", ref+":"+file)
	if err != nil {
		if isOperationalError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s at ref %s", ErrNotFound, file, ref[:12])
	}
	blobOID = strings.TrimSpace(blobOID)

	// Verify it's a blob (not a tree entry for a directory)
	blobType, err := runGit(ctx, dir, defaultMaxBuffer, "cat-file", "-t", blobOID)
	if err != nil {
		if isOperationalError(err) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s at ref %s", ErrNotFound, file, ref[:12])
	}
	if strings.TrimSpace(blobType) != "blob" {
		return nil, fmt.Errorf("%w: %s is not a file at ref %s", ErrInvalidInput, file, ref[:12])
	}

	// Size check
	sizeStr, err := runGit(ctx, dir, defaultMaxBuffer, "cat-file", "-s", blobOID)
	if err != nil {
		return nil, err
	}
	size, err := strconv.ParseInt(strings.TrimSpace(sizeStr), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("unexpected cat-file -s output: %s", sizeStr)
	}
	if size > maxFileSizeBytes {
		return nil, fmt.Errorf("%w: %s (%d bytes)", ErrFileTooLarge, file, size)
	}

	// Stream blob content
	content, err := runGit(ctx, dir, diffMaxBuffer, "cat-file", "blob", blobOID)
	if err != nil {
		return nil, err
	}

	var lines []string
	totalLines := 0
	lineNum := 0
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), maxFileSizeBytes)
	for scanner.Scan() {
		lineNum++
		totalLines = lineNum
		if lineNum < start {
			continue
		}
		if lineNum > end {
			continue
		}
		line := scanner.Text()
		if bytes.ContainsRune([]byte(line), 0) {
			return nil, fmt.Errorf("%w: %s", ErrBinaryFile, file)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if len(lines) == 0 && start > totalLines {
		return nil, fmt.Errorf("%w: start line %d beyond file length %d", ErrInvalidInput, start, totalLines)
	}

	actualEnd := start + len(lines) - 1

	return &FileLinesResult{
		Lines:      lines,
		Start:      start,
		End:        actualEnd,
		TotalLines: totalLines,
	}, nil
}
