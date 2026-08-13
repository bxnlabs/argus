package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bxnlabs/argus/internal/node/files"
	"github.com/bxnlabs/argus/internal/node/filesearch"
	"github.com/bxnlabs/argus/internal/shared"
)

const maxReadSize int64 = 100 << 20 // 100 MB

type filesHandler struct {
	uploadDirOverride string
}

func (h *filesHandler) getUploadDir() string {
	if h.uploadDirOverride != "" {
		return h.uploadDirOverride
	}
	return uploadDir
}

// GET /files?path=...&recursive=false
func (h *filesHandler) list(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.CleanPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	recursive := r.URL.Query().Get("recursive") == "true"
	maxDepth := 1
	if recursive {
		maxDepth = 2 // V1 parity
	}

	nodes, err := files.ListDirectory(expandedPath, recursive, maxDepth)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondError(w, http.StatusNotFound, "directory not found")
			return
		}
		if errors.Is(err, os.ErrPermission) {
			respondError(w, http.StatusForbidden, "permission denied")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"files": nodes,
		"path":  expandedPath,
	})
}

// GET /files/meta?path=...
func (h *filesHandler) meta(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.CleanPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := files.FileMeta(expandedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, os.ErrPermission) {
			respondError(w, http.StatusForbidden, "permission denied")
			return
		}
		if strings.Contains(err.Error(), "directory") {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"size":        result.Size,
		"isBinary":    result.IsBinary,
		"contentType": result.ContentType,
		"path":        expandedPath,
	})
}

// GET /files/content?path=...
func (h *filesHandler) readContent(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}
	expandedPath, err := shared.CleanPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	f, err := os.Open(expandedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondError(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, os.ErrPermission) {
			respondError(w, http.StatusForbidden, "permission denied")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if info.IsDir() {
		respondError(w, http.StatusBadRequest, "path is a directory, not a file")
		return
	}
	if info.Size() > maxReadSize {
		respondError(w, http.StatusRequestEntityTooLarge, "file too large to serve")
		return
	}

	// http.ServeContent handles ETag, Range, Last-Modified, Content-Type
	http.ServeContent(w, r, expandedPath, info.ModTime(), f)
}

const (
	maxUploadSize int64  = 50 << 20 // 50 MB
	uploadDir     string = "/tmp/argus-uploads"
)

var sanitizeFilename = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

type uploadResult struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

func (h *filesHandler) saveUploadedFile(fh *multipart.FileHeader) (uploadResult, error) {
	src, err := fh.Open()
	if err != nil {
		return uploadResult{}, fmt.Errorf("failed to read file: %s", fh.Filename)
	}
	defer src.Close()

	sanitized := sanitizeFilename.ReplaceAllString(fh.Filename, "_")
	pattern := fmt.Sprintf("%d-*-%s", time.Now().UnixMilli(), sanitized)

	dst, err := os.CreateTemp(h.getUploadDir(), pattern)
	if err != nil {
		return uploadResult{}, fmt.Errorf("failed to save file: %s", fh.Filename)
	}
	destPath := dst.Name()
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(destPath)
		return uploadResult{}, fmt.Errorf("failed to save file: %s", fh.Filename)
	}

	return uploadResult{
		Path: destPath,
		Name: fh.Filename,
		Size: n,
	}, nil
}

// POST /files/upload
func (h *filesHandler) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+10<<10) // +10KB for multipart framing overhead
	defer r.Body.Close()

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			respondError(w, http.StatusBadRequest, "invalid multipart request")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) == 0 {
		respondError(w, http.StatusBadRequest, "no files provided")
		return
	}

	uploadDir := h.getUploadDir()
	if err := shared.EnsureSecureDir(uploadDir); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create upload directory")
		return
	}

	results := make([]uploadResult, 0, len(fileHeaders))

	for _, fh := range fileHeaders {
		result, err := h.saveUploadedFile(fh)
		if err != nil {
			// Rollback: remove files that were already saved
			for _, saved := range results {
				os.Remove(saved.Path)
			}
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		results = append(results, result)
	}

	respondJSON(w, http.StatusOK, map[string]any{"files": results})
}

// GET /files/search?q=...&type=directory&limit=20
func (h *filesHandler) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		respondError(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	searchType := r.URL.Query().Get("type")
	if searchType != "" && searchType != "file" && searchType != "directory" {
		respondError(w, http.StatusBadRequest, "type must be 'file', 'directory', or omitted")
		return
	}

	limit := 20
	if s := r.URL.Query().Get("limit"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			limit = v
		}
	}

	// Default search root: $HOME
	searchPath := r.URL.Query().Get("path")
	if searchPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to get home directory")
			return
		}
		searchPath = home
	} else {
		expanded, err := shared.SafeExpandPath(searchPath)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		searchPath = expanded
	}

	info, err := os.Stat(searchPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			respondError(w, http.StatusBadRequest, "search path does not exist")
			return
		}
		if errors.Is(err, os.ErrPermission) {
			respondError(w, http.StatusForbidden, "permission denied")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !info.IsDir() {
		respondError(w, http.StatusBadRequest, "search path must be a directory")
		return
	}

	result, err := filesearch.Search(r.Context(), searchPath, query, searchType, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, result)
}
