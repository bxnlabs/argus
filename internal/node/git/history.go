package git

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	filesChangedRe = regexp.MustCompile(`(\d+) files? changed`)
	insertionsRe   = regexp.MustCompile(`(\d+) insertions?\(\+\)`)
	deletionsRe    = regexp.MustCompile(`(\d+) deletions?\(-\)`)
)

// GetHistory returns the commit log for a directory.
// Matches lib/git-history.ts:getCommitHistory().
// Uses %x1E (record separator) between commits to handle multi-line bodies.
func GetHistory(dir string, limit int) ([]CommitSummary, error) {
	if limit <= 0 {
		limit = 30
	}
	if limit > 200 {
		limit = 200
	}

	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	// Use record separator (%x1E) at the START of each commit's format output,
	// with null (%x00) between fields. %b can contain newlines (e.g. Co-Authored-By
	// trailers), so we can't rely on line-based splitting. By placing %x1E at the
	// start, each record contains both the commit metadata and its --shortstat output.
	format := "%x1E%H%x00%h%x00%s%x00%b%x00%an%x00%ae%x00%at"
	out, err := runGit(ctx, dir, defaultMaxBuffer,
		"log", fmt.Sprintf("--format=%s", format), "--shortstat",
		"-n", strconv.Itoa(limit))
	if err != nil {
		if strings.Contains(err.Error(), "does not have any commits") ||
			strings.Contains(err.Error(), "bad default revision") {
			return []CommitSummary{}, nil
		}
		return nil, err
	}

	var commits []CommitSummary

	// Split by record separator — each record contains commit metadata
	// followed optionally by a shortstat line.
	records := strings.Split(out, "\x1E")

	for _, record := range records {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		// The record may contain the commit fields + optional shortstat after a newline.
		// Find the field part (contains \x00 separators) and the shortstat part.
		parts := strings.SplitN(record, "\x00", 7)
		if len(parts) < 7 {
			continue
		}

		// The last field (timestamp) may have trailing shortstat lines after it
		tsAndStat := parts[6]
		tsLines := strings.SplitN(tsAndStat, "\n", 2)
		tsStr := strings.TrimSpace(tsLines[0])

		tsInt, _ := strconv.ParseInt(tsStr, 10, 64)

		commit := CommitSummary{
			Hash:         parts[0],
			ShortHash:    parts[1],
			Subject:      parts[2],
			Body:         strings.TrimSpace(parts[3]),
			Author:       parts[4],
			AuthorEmail:  parts[5],
			Timestamp:    time.Unix(tsInt, 0).Format(time.RFC3339),
			RelativeTime: relativeTime(tsInt),
		}

		// Parse shortstat from remaining lines
		if len(tsLines) > 1 {
			for _, statLine := range strings.Split(tsLines[1], "\n") {
				statLine = strings.TrimSpace(statLine)
				if statLine == "" {
					continue
				}
				if m := filesChangedRe.FindStringSubmatch(statLine); m != nil {
					commit.FilesChanged, _ = strconv.Atoi(m[1])
				}
				if m := insertionsRe.FindStringSubmatch(statLine); m != nil {
					commit.Additions, _ = strconv.Atoi(m[1])
				}
				if m := deletionsRe.FindStringSubmatch(statLine); m != nil {
					commit.Deletions, _ = strconv.Atoi(m[1])
				}
			}
		}

		commits = append(commits, commit)
	}

	if commits == nil {
		commits = []CommitSummary{}
	}
	return commits, nil
}

// GetCommitDetail returns full detail for a single commit.
// Matches lib/git-history.ts:getCommitDetail().
func GetCommitDetail(dir, hash string) (*CommitDetail, error) {
	if err := validateHash(hash); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	// Step 1: metadata — use %x1E record terminator so multi-line %b is safe
	format := "%H%x00%h%x00%s%x00%b%x00%an%x00%ae%x00%at%x1E"
	metaOut, err := runGit(ctx, dir, defaultMaxBuffer, "show", fmt.Sprintf("--format=%s", format), "-s", hash)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: commit %q", ErrNotFound, hash)
		}
		return nil, err
	}

	// Strip the record separator and trim
	metaOut = strings.TrimRight(metaOut, "\x1E \n")

	parts := strings.SplitN(metaOut, "\x00", 7)
	if len(parts) < 7 {
		return nil, fmt.Errorf("unexpected git show output")
	}

	tsInt, _ := strconv.ParseInt(strings.TrimSpace(parts[6]), 10, 64)

	detail := &CommitDetail{
		CommitSummary: CommitSummary{
			Hash:         parts[0],
			ShortHash:    parts[1],
			Subject:      parts[2],
			Body:         strings.TrimSpace(parts[3]),
			Author:       parts[4],
			AuthorEmail:  parts[5],
			Timestamp:    time.Unix(tsInt, 0).Format(time.RFC3339),
			RelativeTime: relativeTime(tsInt),
		},
	}

	// Step 2: name-status (for detecting renames/adds/deletes)
	statusMap := map[string]struct {
		status  string
		oldPath string
	}{}
	nsOut, err := runGit(ctx, dir, diffMaxBuffer, "show", "--name-status", "--format=", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get file statuses: %w", err)
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
			// Rename: R100\told\tnew
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

	// Step 3: numstat
	var files []CommitFile
	totalAdds, totalDels := 0, 0
	numOut, err := runGit(ctx, dir, diffMaxBuffer, "show", "--numstat", "--format=", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
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

	if files == nil {
		files = []CommitFile{}
	}
	detail.Files = files
	detail.FilesChanged = len(files)
	detail.Additions = totalAdds
	detail.Deletions = totalDels

	return detail, nil
}

// GetCommitFullDiff returns the full combined diff for all files in a commit,
// along with per-file total line counts for context expansion.
func GetCommitFullDiff(dir, hash string) (*CommitFullDiffResult, error) {
	if err := validateHash(hash); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), longTimeout)
	defer cancel()

	diff, err := runGit(ctx, dir, diffMaxBuffer, "show", "--format=", "-U3", "-m", "--first-parent", hash)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: commit %q", ErrNotFound, hash)
		}
		return nil, err
	}

	return &CommitFullDiffResult{
		Diff:       diff,
		TotalLines: computeTotalLines(ctx, dir, hash, commitDiffFiles(ctx, dir, hash)),
	}, nil
}

// commitDiffFiles lists the files a commit touched, feeding totalLines and
// nothing else — the caller's diff is already complete before this runs. It
// reports no error by design: only an expand button is at stake, so a deadline
// that expires here must not turn an assembled diff into a 504 with nothing.
//
// diff-tree rather than hash^: it handles root commits, where hash^ is
// invalid. -m --first-parent matches the diff command's merge-commit semantics.
func commitDiffFiles(ctx context.Context, dir, hash string) []CommitFile {
	nsOut, err := runGit(ctx, dir, diffMaxBuffer,
		"diff-tree", "--root", "--name-status", "-r", "-m", "--first-parent", hash)
	if err != nil {
		return nil
	}

	var files []CommitFile
	for _, line := range strings.Split(strings.TrimSpace(nsOut), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		statusChar := fields[0][:1]
		var cf CommitFile
		if len(fields) == 3 {
			cf = CommitFile{Path: fields[2], OldPath: fields[1]}
		} else {
			cf = CommitFile{Path: fields[1]}
		}
		switch statusChar {
		case "A":
			cf.Status = StatusAdded
		case "D":
			cf.Status = StatusDeleted
		case "R":
			cf.Status = StatusRenamed
		case "C":
			cf.Status = StatusCopied
		default:
			cf.Status = StatusModified
		}
		files = append(files, cf)
	}
	return files
}
