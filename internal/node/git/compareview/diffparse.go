package compareview

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/bxnlabs/argus/internal/node/git"
)

var hunkHeaderRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

var diffGitHeaderRe = regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)

// ParseUnifiedDiff parses a multi-file unified diff (as emitted by `git diff`)
// into structured FileView entries. All emitted hunks have Kind=HunkKindDiff.
// Additions/Deletions counts and TotalLines/TotalAdditions/TotalDeletions
// metadata are populated by the caller from the CompareResult — this function
// only handles diff text parsing.
//
// Semantics mirror web/src/lib/diff-parser.ts:parseDiff / parseMultiFileDiff.
func ParseUnifiedDiff(text string) ([]FileView, error) {
	if text == "" {
		return nil, nil
	}
	// Split on "diff --git " section boundaries (multi-file).
	var sections []string
	idx := 0
	for {
		next := strings.Index(text[idx+1:], "\ndiff --git ")
		if next == -1 {
			sections = append(sections, text[idx:])
			break
		}
		sections = append(sections, text[idx:idx+1+next])
		idx = idx + 1 + next + 1 // skip the \n
	}
	out := make([]FileView, 0, len(sections))
	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		if !strings.HasPrefix(sec, "diff --git ") {
			continue
		}
		fv, err := parseSingleFile(sec)
		if err != nil {
			return nil, err
		}
		out = append(out, fv)
	}
	return out, nil
}

func parseSingleFile(sec string) (FileView, error) {
	lines := strings.Split(sec, "\n")
	var (
		oldFile, newFile string
		hunks            []Hunk
		current          *Hunk
		oldLineNum       int
		newLineNum       int
		isNew, isDel     bool
		isRenamed        bool
		isBinary         bool
	)
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "diff --git "):
			// Fallback parse for renames / binary where --- +++ are absent.
			if m := diffGitHeaderRe.FindStringSubmatch(ln); m != nil {
				if oldFile == "" {
					oldFile = m[1]
				}
				if newFile == "" {
					newFile = m[2]
				}
			}
		case strings.HasPrefix(ln, "Binary files"):
			isBinary = true
		case strings.HasPrefix(ln, "--- "):
			s := strings.TrimPrefix(ln, "--- ")
			if s == "/dev/null" {
				isNew = true
			} else if strings.HasPrefix(s, "a/") {
				oldFile = strings.TrimPrefix(s, "a/")
			} else {
				oldFile = s
			}
		case strings.HasPrefix(ln, "+++ "):
			s := strings.TrimPrefix(ln, "+++ ")
			if s == "/dev/null" {
				isDel = true
			} else if strings.HasPrefix(s, "b/") {
				newFile = strings.TrimPrefix(s, "b/")
			} else {
				newFile = s
			}
		case strings.HasPrefix(ln, "rename from ") || strings.HasPrefix(ln, "rename to "):
			isRenamed = true
		default:
			if m := hunkHeaderRe.FindStringSubmatch(ln); m != nil {
				if current != nil {
					hunks = append(hunks, *current)
				}
				oldStart, _ := strconv.Atoi(m[1])
				oldCount := 1
				if m[2] != "" {
					oldCount, _ = strconv.Atoi(m[2])
				}
				newStart, _ := strconv.Atoi(m[3])
				newCount := 1
				if m[4] != "" {
					newCount, _ = strconv.Atoi(m[4])
				}
				current = &Hunk{
					Kind:     HunkKindDiff,
					Header:   ln,
					OldStart: oldStart, OldCount: oldCount,
					NewStart: newStart, NewCount: newCount,
				}
				if ctx := strings.TrimSpace(m[5]); ctx != "" {
					current.Lines = append(current.Lines, HunkLine{
						Type:    "header",
						Content: ctx,
					})
				}
				oldLineNum, newLineNum = oldStart, newStart
				continue
			}
			if current == nil {
				continue
			}
			switch {
			case strings.HasPrefix(ln, "+"):
				n := newLineNum
				current.Lines = append(current.Lines, HunkLine{
					Type: "addition", Content: ln[1:],
					NewLineNumber: &n,
				})
				newLineNum++
			case strings.HasPrefix(ln, "-"):
				n := oldLineNum
				current.Lines = append(current.Lines, HunkLine{
					Type: "deletion", Content: ln[1:],
					OldLineNumber: &n,
				})
				oldLineNum++
			case strings.HasPrefix(ln, " ") || ln == "":
				content := ln
				if strings.HasPrefix(ln, " ") {
					content = ln[1:]
				}
				oln, nln := oldLineNum, newLineNum
				current.Lines = append(current.Lines, HunkLine{
					Type: "context", Content: content,
					OldLineNumber: &oln, NewLineNumber: &nln,
				})
				oldLineNum++
				newLineNum++
			}
		}
	}
	if current != nil {
		hunks = append(hunks, *current)
	}
	fv := FileView{
		Hunks:    hunks,
		IsBinary: isBinary,
	}
	switch {
	case isNew:
		fv.Path = newFile
		fv.Status = git.StatusAdded
	case isDel:
		fv.Path = oldFile
		fv.Status = git.StatusDeleted
	case isRenamed:
		fv.Path = newFile
		fv.OldPath = oldFile
		fv.Status = git.StatusRenamed
	default:
		fv.Path = newFile
		if fv.Path == "" {
			fv.Path = oldFile
		}
		fv.Status = git.StatusModified
	}
	return fv, nil
}
