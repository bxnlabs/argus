package files

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sniffSize is how many bytes to read for binary detection.
const sniffSize = 8192

// IsBinary checks if data contains null bytes (same heuristic as V1).
func IsBinary(data []byte) bool {
	return bytes.ContainsRune(data, 0)
}

// FileMeta returns file metadata (size, binary detection, content type)
// without loading the full file into memory.
func FileMeta(path string) (*FileMetaResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	result := &FileMetaResult{
		Size: info.Size(),
	}

	// Detect content type from extension
	result.ContentType = mime.TypeByExtension(filepath.Ext(path))
	if result.ContentType == "" {
		result.ContentType = "application/octet-stream"
	}

	// Detect binary by reading first 8KB
	if info.Size() > 0 {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()

		buf := make([]byte, sniffSize)
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read file for binary detection: %w", err)
		}
		result.IsBinary = IsBinary(buf[:n])
	}

	return result, nil
}

// StreamWrite streams content from r to a file at path.
// Creates parent directories. Uses a temp file + rename for atomicity.
// Preserves file permissions on overwrite. Returns bytes written.
// Returns *FileSizeError if content exceeds maxSize.
func StreamWrite(path string, r io.Reader, maxSize int64) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return 0, err
	}

	// Write to temp file first, then rename for atomicity
	tmp, err := os.CreateTemp(filepath.Dir(path), ".argus-write-*")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	closed := false

	// Clean up temp file on any error
	success := false
	defer func() {
		if !success {
			if !closed {
				tmp.Close()
			}
			os.Remove(tmpPath)
		}
	}()

	// io.LimitReader caps at maxSize+1 so we can detect overflow
	limited := io.LimitReader(r, maxSize+1)
	n, err := io.Copy(tmp, limited)
	if err != nil {
		return 0, fmt.Errorf("write: %w", err)
	}
	if n > maxSize {
		return 0, &FileSizeError{Size: n, MaxSize: maxSize}
	}

	if err := tmp.Close(); err != nil {
		return 0, fmt.Errorf("close temp file: %w", err)
	}
	closed = true

	// Capture existing file mode just before chmod to minimize TOCTOU window
	existingMode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		existingMode = info.Mode()
	}

	// Preserve original file permissions on overwrite
	if err := os.Chmod(tmpPath, existingMode); err != nil {
		return 0, fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return 0, fmt.Errorf("rename: %w", err)
	}

	success = true
	return n, nil
}

// ListDirectory lists files and directories, filtering excluded names.
// maxDepth controls recursion depth (V1 uses: recursive ? 2 : 1).
func ListDirectory(dir string, recursive bool, maxDepth int) ([]FileNode, error) {
	return listDir(dir, recursive, maxDepth, 0)
}

func listDir(dir string, recursive bool, maxDepth, depth int) ([]FileNode, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	nodes := []FileNode{}
	for _, entry := range entries {
		if shouldExclude(entry.Name()) {
			continue
		}

		node := FileNode{
			Name: entry.Name(),
			Path: filepath.Join(dir, entry.Name()),
		}

		if entry.IsDir() {
			node.Type = "directory"
			if recursive && depth < maxDepth {
				children, err := listDir(node.Path, true, maxDepth, depth+1)
				if err == nil {
					node.Children = children
				}
				// Silently skip unreadable directories (V1 parity)
			}
		} else {
			node.Type = "file"
			// V1 parity: extension without leading dot
			ext := filepath.Ext(entry.Name())
			if ext != "" {
				node.Extension = strings.ToLower(ext[1:]) // trim leading dot
			}
			if info, err := entry.Info(); err == nil {
				node.Size = info.Size()
			}
		}

		nodes = append(nodes, node)
	}

	// Sort: directories first, then case-insensitive alphabetical
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Type != nodes[j].Type {
			return nodes[i].Type == "directory"
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}
