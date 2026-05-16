package compareview

import (
	"strconv"

	"github.com/bxnlabs/argus/internal/node/git"
	"github.com/bxnlabs/argus/internal/node/git/review"
)

// Build composes the compare diff and the review file into a single
// rendering payload. The frontend renders v.Files[].Hunks[] without further
// classification — every comment is hosted by some hunk, every hunk carries
// a Kind explaining how it was produced.
//
// projectDir is the per-project state directory (where reviews live).
// repoDir is the working repository.
// head and base are branch names; refs are resolved by git.GetCompare.
func Build(projectDir, repoDir, head, base string) (*View, error) {
	compare, err := git.GetCompare(repoDir, base)
	if err != nil {
		return nil, err
	}
	files, err := ParseUnifiedDiff(compare.Diff)
	if err != nil {
		return nil, err
	}
	overlayCompareMetadata(files, compare)

	r, err := review.Load(projectDir, repoDir, head, base, compare.HeadRef, compare.BaseRef)
	if err != nil {
		return nil, err
	}
	rp := ReviewPayload{
		Head:     head,
		Base:     base,
		Comments: []review.ReviewComment{},
	}
	if r != nil {
		rp.Head = r.Head
		rp.Base = r.Base
		rp.Body = r.Body
		rp.Comments = r.Comments
		files = applyOrphanHunks(files, r.Comments, repoDir, compare.HeadRef, compare.BaseRef)
	}
	return &View{
		BaseRef:        compare.BaseRef,
		HeadRef:        compare.HeadRef,
		BaseUpstream:   compare.BaseUpstream,
		BaseBehindBy:   compare.BaseBehindBy,
		Files:          files,
		TotalLines:     compare.TotalLines,
		TotalAdditions: compare.TotalAdditions,
		TotalDeletions: compare.TotalDeletions,
		Review:         rp,
	}, nil
}

// overlayCompareMetadata copies per-file additions/deletions/oldPath/status
// from CompareResult.Files onto the parsed FileView entries. ParseUnifiedDiff
// gets path and basic status from the diff text; CompareResult has the
// authoritative per-file totals from git's --numstat output.
func overlayCompareMetadata(files []FileView, compare *git.CompareResult) {
	byPath := make(map[string]int, len(compare.Files))
	for i, cf := range compare.Files {
		// CompareResult.Files always keys on the post-rename path (normalised
		// inside GetCompare), matching FileView.Path emitted by the parser.
		byPath[cf.Path] = i
	}
	for i := range files {
		idx, ok := byPath[files[i].Path]
		if !ok {
			continue
		}
		cf := compare.Files[idx]
		files[i].Additions = cf.Additions
		files[i].Deletions = cf.Deletions
		if cf.OldPath != "" {
			files[i].OldPath = cf.OldPath
		}
		// Trust git's status (handles copy/rename detection thresholds).
		files[i].Status = cf.Status
	}
}

// orphanContextWindow is how many lines on each side of the anchor we fetch
// for a comment-anchored context hunk. Matches the synthetic-fetch window
// the frontend used previously (3 lines).
const orphanContextWindow = 3

// applyOrphanHunks adds a context hunk to each FileView for every comment
// that isn't already hosted by a real diff hunk in that file. For case-(b)
// (comment is on a file IN the diff but anchor line is outside any real
// hunk), the context hunk is fetched from the appropriate ref and inserted
// into the FileView in file order.
//
// Case-(a) (file not in compare diff at all) and case-(d) (snippet not
// found) are handled in later tasks.
func applyOrphanHunks(files []FileView, comments []review.ReviewComment, repoDir, headRef, baseRef string) []FileView {
	for _, c := range comments {
		if !c.Submitted {
			continue
		}
		if isCommentHostedByAnyFile(c, files) {
			continue
		}
		idx := findFileIndexForComment(c, files)
		h, ok := buildContextHunk(c, repoDir, headRef, baseRef)
		if !ok {
			continue // case-(d), handled in next task
		}
		if idx == -1 {
			// case-(a): synthesize a FileView for the unchanged file.
			files = appendOrUpdateContextFile(files, c, h)
			continue
		}
		files[idx].Hunks = insertHunkInOrder(files[idx].Hunks, h)
	}
	return files
}

// findFileIndexForComment returns the index of the FileView that hosts this
// comment's authored side, or -1 if no such file exists (case-(a)).
func findFileIndexForComment(c review.ReviewComment, files []FileView) int {
	side := c.Line.From.Side
	for i, f := range files {
		if side == review.DiffSideLeft {
			leftPath := c.OldPath
			if leftPath == "" {
				leftPath = c.File
			}
			if f.OldPath != "" && f.OldPath == leftPath {
				return i
			}
			if f.OldPath == "" && f.Path == leftPath {
				return i
			}
		} else {
			if f.Path == c.File {
				return i
			}
		}
	}
	return -1
}

// buildContextHunk fetches ±orphanContextWindow lines around the comment's
// anchor from the relevant ref and assembles a Kind=HunkKindContext hunk.
// Returns ok=false when the fetch fails (file missing at ref, line past EOF,
// binary, too-large, etc.) — caller falls back to snippet-fallback handling.
func buildContextHunk(c review.ReviewComment, repoDir, headRef, baseRef string) (Hunk, bool) {
	side := c.Line.From.Side
	ref := headRef
	lookupPath := c.File
	if side == review.DiffSideLeft {
		ref = baseRef
		if c.OldPath != "" {
			lookupPath = c.OldPath
		}
	}
	if ref == "" {
		return Hunk{}, false
	}
	anchor := c.Line.From.Line
	start := anchor - orphanContextWindow
	if start < 1 {
		start = 1
	}
	end := anchor + orphanContextWindow
	res, err := git.GetFileLines(repoDir, lookupPath, start, end, ref)
	if err != nil {
		return Hunk{}, false
	}
	lines := make([]HunkLine, 0, len(res.Lines))
	for i, content := range res.Lines {
		n := res.Start + i
		var ol, nl *int
		if side == review.DiffSideLeft {
			v := n
			ol = &v
		} else {
			v := n
			nl = &v
		}
		lines = append(lines, HunkLine{
			Type:          "context",
			Content:       content,
			OldLineNumber: ol,
			NewLineNumber: nl,
		})
	}
	h := Hunk{
		Kind:             HunkKindContext,
		Header:           "",
		OldStart:         res.Start,
		OldCount:         len(lines),
		NewStart:         res.Start,
		NewCount:         len(lines),
		Lines:            lines,
		AnchorCommentIDs: []string{c.ID},
	}
	if side == review.DiffSideLeft {
		h.NewStart = 0
		h.NewCount = 0
	} else {
		h.OldStart = 0
		h.OldCount = 0
	}
	h.Header = formatHunkHeader(h)
	return h, true
}

func formatHunkHeader(h Hunk) string {
	return "@@ -" + itoaPos(h.OldStart, h.OldCount) + " +" + itoaPos(h.NewStart, h.NewCount) + " @@"
}

func itoaPos(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "," + strconv.Itoa(count)
}

// insertHunkInOrder inserts h into hunks at the position that keeps NewStart
// ascending (or OldStart ascending for L-side context hunks where NewStart=0).
func insertHunkInOrder(hunks []Hunk, h Hunk) []Hunk {
	keyOf := func(hh Hunk) int {
		if hh.NewStart > 0 {
			return hh.NewStart
		}
		return hh.OldStart
	}
	target := keyOf(h)
	for i, existing := range hunks {
		if target < keyOf(existing) {
			return append(hunks[:i:i], append([]Hunk{h}, hunks[i:]...)...)
		}
	}
	return append(hunks, h)
}

// appendOrUpdateContextFile finds or creates a synthetic FileView for a
// comment whose file is not in the compare diff. Multiple comments on the
// same unchanged file share one FileView. Synthetic files always sort after
// real-diff files (preserved by the caller's order: real-diff files were
// appended first by ParseUnifiedDiff).
func appendOrUpdateContextFile(files []FileView, c review.ReviewComment, h Hunk) []FileView {
	path := c.File
	oldPath := c.OldPath
	for i, f := range files {
		if f.Status == git.StatusContext && f.Path == path && f.OldPath == oldPath {
			files[i].Hunks = insertHunkInOrder(f.Hunks, h)
			return files
		}
	}
	files = append(files, FileView{
		Path:    path,
		OldPath: oldPath,
		Status:  git.StatusContext,
		Hunks:   []Hunk{h},
	})
	return files
}

// isCommentHostedByHunk returns true if the comment's anchor line appears on
// the comment's authored side in this hunk's lines. Context lines are valid
// hosts on either side. Side determines which line-number field to compare.
func isCommentHostedByHunk(c review.ReviewComment, h Hunk) bool {
	side := c.Line.From.Side
	target := c.Line.From.Line
	for _, ln := range h.Lines {
		var lineNum *int
		if side == review.DiffSideLeft {
			lineNum = ln.OldLineNumber
		} else {
			lineNum = ln.NewLineNumber
		}
		if lineNum != nil && *lineNum == target {
			return true
		}
	}
	return false
}

// isCommentHostedByAnyFile returns true if any hunk in any file hosts the
// comment's anchor on its authored side. The file match honors renames:
// L-side resolves against OldPath (or File if OldPath is empty), R-side
// resolves against the file's new Path.
func isCommentHostedByAnyFile(c review.ReviewComment, files []FileView) bool {
	side := c.Line.From.Side
	for _, f := range files {
		var match bool
		if side == review.DiffSideLeft {
			leftPath := c.OldPath
			if leftPath == "" {
				leftPath = c.File
			}
			// L-side matches when this file's old (pre-rename) path is leftPath,
			// or when there's no rename and File equals the file's path.
			if f.OldPath != "" && f.OldPath == leftPath {
				match = true
			} else if f.OldPath == "" && f.Path == leftPath {
				match = true
			}
		} else {
			// R-side: comment.File names the head-side path. With a rename,
			// comment.File is the new name and f.Path is the new name.
			if f.Path == c.File {
				match = true
			}
		}
		if !match {
			continue
		}
		for _, h := range f.Hunks {
			if isCommentHostedByHunk(c, h) {
				return true
			}
		}
	}
	return false
}
