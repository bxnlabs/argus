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
