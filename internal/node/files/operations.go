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
