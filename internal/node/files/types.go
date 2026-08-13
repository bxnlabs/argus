package files

// FileNode represents a file or directory in the file tree.
type FileNode struct {
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	Type      string     `json:"type"` // "file" or "directory"
	Size      int64      `json:"size,omitempty"`
	Extension string     `json:"extension,omitempty"` // without dot: "go", "ts"
	Children  []FileNode `json:"children,omitempty"`
}

// FileView is a file as the viewer needs it: the bytes, or why there are none.
// Content is empty whenever IsBinary, IsLarge or Unchanged is set.
//
// Unchanged means the caller already has this version — it supplied a matching
// ETag — so only the ETag is populated and the file was never read.
type FileView struct {
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	IsBinary  bool   `json:"isBinary"`
	IsLarge   bool   `json:"isLarge"`
	ETag      string `json:"etag"`
	Unchanged bool   `json:"unchanged"`
}
