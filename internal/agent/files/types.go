package files

import "fmt"

// FileNode represents a file or directory in the file tree.
type FileNode struct {
	Name      string     `json:"name"`
	Path      string     `json:"path"`
	Type      string     `json:"type"` // "file" or "directory"
	Size      int64      `json:"size,omitempty"`
	Extension string     `json:"extension,omitempty"` // without dot: "go", "ts"
	Children  []FileNode `json:"children,omitempty"`
}

// FileMetaResult contains metadata about a file without its content.
type FileMetaResult struct {
	Size        int64  `json:"size"`
	IsBinary    bool   `json:"isBinary"`
	ContentType string `json:"contentType"`
}

// FileSizeError indicates that content exceeds a size limit.
type FileSizeError struct {
	Size    int64
	MaxSize int64
}

func (e *FileSizeError) Error() string {
	return fmt.Sprintf("content size %d exceeds %d byte limit", e.Size, e.MaxSize)
}
