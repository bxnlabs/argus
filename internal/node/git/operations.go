package git

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxFileSizeBytes = 2 * 1024 * 1024 // 2 MB
	maxLineSpan      = 500
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

	if untracked {
		// git diff --no-index exits 1 when files differ (normal behavior).
		// runGit discards stdout on non-zero exit, so we run this directly
		// to capture the diff output despite the expected exit code 1.
		return runGitDiffNoIndex(ctx, dir, file)
	}

	var args []string
	if staged {
		args = []string{"diff", "-U3", "--cached", "--", file}
	} else {
		args = []string{"diff", "-U3", "--", file}
	}

	return runGit(ctx, dir, diffMaxBuffer, args...)
}

// runGitDiffNoIndex runs git diff --no-index /dev/null <file> and returns
// stdout even when git exits 1 (which means files differ — normal behavior).
func runGitDiffNoIndex(ctx context.Context, dir, file string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "diff", "-U20", "--no-index", "/dev/null", file)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	stdout := &limitedWriter{limit: diffMaxBuffer}
	var stderr bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Exit code 1 = files differ, which is expected for untracked files
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
			return stdout.buf.String(), nil
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("git diff: %s", errMsg)
	}
	return stdout.buf.String(), nil
}

// GetFileContent returns the HEAD version of a file.
// Returns (content, isNew, error). isNew=true if file doesn't exist in HEAD.
func GetFileContent(dir, file string) (string, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	content, err := runGit(ctx, dir, diffMaxBuffer, "show", fmt.Sprintf("HEAD:%s", file))
	if err != nil {
		// File not in HEAD = new file
		return "", true, nil
	}
	return content, false, nil
}

// Check reports whether dir is inside a git work tree.
func Check(dir string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	_, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--git-dir")
	return err == nil, nil
}

// GetWorkingDiff returns the full combined diff for all working-tree changes
// (staged, unstaged, and untracked), along with per-file metadata.
func GetWorkingDiff(dir string) (*WorkingDiffResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	hasHEAD := true
	if _, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", "--verify", "HEAD"); err != nil {
		hasHEAD = false
	}

	var trackedDiff string
	if hasHEAD {
		var err error
		trackedDiff, err = runGit(ctx, dir, diffMaxBuffer, "diff", "-U3", "HEAD")
		if err != nil {
			return nil, err
		}
	} else {
		cachedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U3", "--cached")
		if err != nil {
			return nil, err
		}
		unstagedDiff, err := runGit(ctx, dir, diffMaxBuffer, "diff", "-U3")
		if err != nil {
			return nil, err
		}
		switch {
		case cachedDiff != "" && unstagedDiff != "":
			trackedDiff = cachedDiff + "\n" + unstagedDiff
		case cachedDiff != "":
			trackedDiff = cachedDiff
		default:
			trackedDiff = unstagedDiff
		}
	}

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

	var diffParts []string
	if trackedDiff != "" {
		diffParts = append(diffParts, trackedDiff)
	}
	for _, path := range untrackedPaths {
		fileDiff, err := GetFileDiff(dir, path, false, true)
		if err != nil {
			log.Printf("warning: diff for untracked %q: %v", path, err)
			continue
		}
		if fileDiff != "" {
			diffParts = append(diffParts, fileDiff)
		}
	}

	combinedDiff := strings.Join(diffParts, "\n")

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

	// Untracked file numstat: uses exec.Command directly (not runGit) because
	// git diff --no-index exits 1 when files differ, and runGit discards stdout
	// on non-zero exit. Same pattern as runGitDiffNoIndex.
	for _, path := range untrackedPaths {
		adds := 0
		cmd := exec.CommandContext(ctx, "git", "diff", "--numstat", "--no-index", "/dev/null", "--", path)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
		numBytes, err := cmd.Output()
		if err != nil {
			if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 1 {
				err = nil
			} else {
				log.Printf("warning: numstat for untracked %q: %v", path, err)
			}
		}
		if err == nil && len(numBytes) > 0 {
			fields := strings.SplitN(strings.TrimSpace(string(numBytes)), "\t", 3)
			if len(fields) >= 1 && fields[0] != "-" {
				adds, _ = strconv.Atoi(fields[0])
			}
		}
		files = append(files, CommitFile{
			Path:      path,
			Status:    StatusAdded,
			Additions: adds,
		})
		totalAdds += adds
	}

	if files == nil {
		files = []CommitFile{}
	}
	return files, totalAdds, totalDels, nil
}

// countFileLines returns the logical line count of a file on disk using
// a scanner. This counts lines yielded by bufio.Scanner (including a final
// unterminated line), which may differ from wc -l for files without a
// trailing newline.
func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxFileSizeBytes)
	for scanner.Scan() {
		count++
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

		path := f.Path
		var count int
		var err error
		if ref == "" {
			count, err = countFileLines(filepath.Join(dir, path))
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

	// Empty result (start > totalLines or empty file): signal with End=start-1.
	// Mirrors the silent-clamp behavior used when end > totalLines so
	// out-of-range anchors uniformly take the success path on the frontend.
	actualEnd := start + len(lines) - 1
	if len(lines) == 0 {
		actualEnd = start - 1
	}

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
		return nil, fmt.Errorf("%w: ref %q", ErrNotFound, ref)
	}
	if strings.TrimSpace(objType) != "commit" {
		return nil, fmt.Errorf("%w: ref %q is not a commit", ErrInvalidInput, ref)
	}

	// Resolve file to blob OID via rev-parse <ref>:<file>
	blobOID, err := runGit(ctx, dir, defaultMaxBuffer, "rev-parse", ref+":"+file)
	if err != nil {
		return nil, fmt.Errorf("%w: %s at ref %s", ErrNotFound, file, ref[:12])
	}
	blobOID = strings.TrimSpace(blobOID)

	// Verify it's a blob (not a tree entry for a directory)
	blobType, err := runGit(ctx, dir, defaultMaxBuffer, "cat-file", "-t", blobOID)
	if err != nil {
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

	// Empty result: signal with End=start-1. Same contract as the working-tree
	// path; see comment in getFileLinesFromDisk for rationale.
	actualEnd := start + len(lines) - 1
	if len(lines) == 0 {
		actualEnd = start - 1
	}

	return &FileLinesResult{
		Lines:      lines,
		Start:      start,
		End:        actualEnd,
		TotalLines: totalLines,
	}, nil
}
