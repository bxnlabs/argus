package git

// FileStatus represents the status of a file in git.
type FileStatus string

const (
	StatusModified  FileStatus = "modified"
	StatusAdded     FileStatus = "added"
	StatusDeleted   FileStatus = "deleted"
	StatusRenamed   FileStatus = "renamed"
	StatusCopied    FileStatus = "copied"
	StatusUntracked FileStatus = "untracked"
	StatusUnmerged  FileStatus = "unmerged"
)

const (
	// StatusContext is a synthetic status for files that exist in the compare
	// response only to host comment-anchored context hunks. The file has no
	// real changes between base and head; it appears so reviewers can see
	// comments whose anchor is on an otherwise-unchanged file.
	StatusContext FileStatus = "context"
)

// GitFile represents a single file in git status output.
type GitFile struct {
	Path    string     `json:"path"`
	Status  FileStatus `json:"status"`
	Staged  bool       `json:"staged"`
	OldPath string     `json:"oldPath,omitempty"`
}

// GitStatus represents the full git status of a repository.
type GitStatus struct {
	Branch    string    `json:"branch"`
	Ahead     int       `json:"ahead"`
	Behind    int       `json:"behind"`
	Staged    []GitFile `json:"staged"`
	Unstaged  []GitFile `json:"unstaged"`
	Untracked []GitFile `json:"untracked"`
}

// CommitSummary represents a single commit in the log.
type CommitSummary struct {
	Hash         string `json:"hash"`
	ShortHash    string `json:"shortHash"`
	Subject      string `json:"subject"`
	Body         string `json:"body"`
	Author       string `json:"author"`
	AuthorEmail  string `json:"authorEmail"`
	Timestamp    string `json:"timestamp"`
	RelativeTime string `json:"relativeTime"`
	FilesChanged int    `json:"filesChanged"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
}

// CommitFile represents a file changed in a commit.
type CommitFile struct {
	Path      string     `json:"path"`
	Status    FileStatus `json:"status"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	OldPath   string     `json:"oldPath,omitempty"`
}

// CommitDetail is a commit with its list of changed files.
type CommitDetail struct {
	CommitSummary
	Files []CommitFile `json:"files"`
}

// CompareResult holds the diff and file metadata for a branch comparison.
type CompareResult struct {
	Diff           string         `json:"diff"`
	Files          []CommitFile   `json:"files"`
	TotalAdditions int            `json:"totalAdditions"`
	TotalDeletions int            `json:"totalDeletions"`
	BaseRef        string         `json:"baseRef"`
	HeadRef        string         `json:"headRef"`
	TotalLines     map[string]int `json:"totalLines"`

	// BaseUpstream is the short name of the upstream tracking branch that was
	// used as the effective comparison base (e.g. "origin/main"). Empty when
	// the local base had no upstream, matched upstream, or had diverged (both
	// ahead of and behind upstream) — in all of those cases no substitution
	// happened and the local ref was used as-is.
	BaseUpstream string `json:"baseUpstream"`

	// BaseBehindBy is the number of commits the local base ref is behind its
	// upstream when the substitution fired. >0 means the diff was computed
	// against the upstream tip; 0 means the local ref was used as-is (no
	// upstream, not behind, or diverged).
	BaseBehindBy int `json:"baseBehindBy"`
}

// BranchList holds available branches and the auto-detected default base.
type BranchList struct {
	Branches    []string `json:"branches"`
	DefaultBase string   `json:"defaultBase"`
}

// WorkingDiffResult holds the combined diff and per-file metadata for
// all working-tree changes (staged + unstaged + untracked).
type WorkingDiffResult struct {
	Diff           string         `json:"diff"`
	Files          []CommitFile   `json:"files"`
	TotalAdditions int            `json:"totalAdditions"`
	TotalDeletions int            `json:"totalDeletions"`
	TotalLines     map[string]int `json:"totalLines"`
	Fingerprint    string         `json:"fingerprint"`
}

// CommitFullDiffResult holds the full diff and per-file total line counts
// for a single commit.
type CommitFullDiffResult struct {
	Diff       string         `json:"diff"`
	TotalLines map[string]int `json:"totalLines"`
}

// FileLinesResult holds lines fetched from a file for context expansion.
type FileLinesResult struct {
	Lines      []string `json:"lines"`
	Start      int      `json:"start"`
	End        int      `json:"end"`
	TotalLines int      `json:"totalLines"`
}
