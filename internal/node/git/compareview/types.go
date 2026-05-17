// Package compareview composes a branch compare diff with its review comments
// into a single rendering payload. The frontend renders the response without
// further classification: every comment is hosted by a hunk, and every hunk
// carries a Kind that says how to render it.
package compareview

import (
	"github.com/bxnlabs/argus/internal/node/git"
	"github.com/bxnlabs/argus/internal/node/git/review"
)

// HunkKind classifies a hunk's origin.
//
//   - HunkKindDiff: emitted by `git diff` between base and head.
//   - HunkKindContext: synthesized by the backend from current file content
//     (via GetFileLines) so a comment whose anchor lies outside any real diff
//     hunk still has surrounding lines to render.
//   - HunkKindSnippet: synthesized from the comment's stored snippet text
//     when the anchor cannot be re-resolved (file missing at ref, line past
//     EOF). Carries AnchorMissing=true so the renderer can surface it.
type HunkKind string

const (
	HunkKindDiff    HunkKind = "diff"
	HunkKindContext HunkKind = "context"
	HunkKindSnippet HunkKind = "snippet"
)

// HunkLine matches the existing frontend DiffLine shape exactly. Pointers
// distinguish "no line number" (deletion on R side, addition on L side) from
// "line zero" — JSON marshals nil as `null`.
type HunkLine struct {
	Type          string `json:"type"` // "context" | "addition" | "deletion" | "header"
	Content       string `json:"content"`
	OldLineNumber *int   `json:"oldLineNumber"`
	NewLineNumber *int   `json:"newLineNumber"`
}

// Hunk is a contiguous block of lines for one file. Kind distinguishes
// real-diff hunks from comment-anchored context hunks.
type Hunk struct {
	Kind     HunkKind   `json:"kind"`
	Header   string     `json:"header"`
	OldStart int        `json:"oldStart"`
	OldCount int        `json:"oldCount"`
	NewStart int        `json:"newStart"`
	NewCount int        `json:"newCount"`
	Lines    []HunkLine `json:"lines"`

	// AnchorMissing is true for HunkKindSnippet hunks built from stored snippet
	// text because the file or line could not be located at the relevant ref.
	AnchorMissing bool `json:"anchorMissing,omitempty"`
}

// FileView is one entry in the compare response. Mirrors the rendering needs
// of LazyFileDiff/ExpandableUnifiedDiff after a tiny adapter on the frontend.
type FileView struct {
	Path      string         `json:"path"`
	OldPath   string         `json:"oldPath,omitempty"`
	Status    git.FileStatus `json:"status"`
	Additions int            `json:"additions"`
	Deletions int            `json:"deletions"`
	IsBinary  bool           `json:"isBinary,omitempty"`
	Hunks     []Hunk         `json:"hunks"`
}

// ReviewPayload mirrors the existing /api/git/review GET response so the
// frontend can drop the separate useReviewQuery on the read path.
type ReviewPayload struct {
	Head     string                 `json:"head"`
	Base     string                 `json:"base"`
	Body     *review.ReviewBody     `json:"body,omitempty"`
	Comments []review.ReviewComment `json:"comments"`
}

// View is the full compare-with-review response.
type View struct {
	BaseRef        string         `json:"baseRef"`
	HeadRef        string         `json:"headRef"`
	BaseUpstream   string         `json:"baseUpstream"`
	BaseBehindBy   int            `json:"baseBehindBy"`
	Files          []FileView     `json:"files"`
	TotalLines     map[string]int `json:"totalLines"`
	TotalAdditions int            `json:"totalAdditions"`
	TotalDeletions int            `json:"totalDeletions"`
	Review         ReviewPayload  `json:"review"`
}
