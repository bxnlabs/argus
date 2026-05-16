package compareview

import (
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
