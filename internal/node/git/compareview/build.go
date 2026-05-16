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
