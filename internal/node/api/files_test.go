package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/node/files"
)

// homeTempDir creates a temporary directory under $HOME so that
// SafeExpandPath validation passes in tests.
func homeTempDir(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(home, "argus-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestFilesHandlerList(t *testing.T) {
	dir := homeTempDir(t)
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files?path="+dir, nil)
	w := httptest.NewRecorder()
	handler.list(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "hello.txt") {
		t.Errorf("expected hello.txt in response, got %s", body)
	}
	if !strings.Contains(body, "subdir") {
		t.Errorf("expected subdir in response, got %s", body)
	}
}

func TestFilesHandlerList_MissingPath(t *testing.T) {
	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files", nil)
	w := httptest.NewRecorder()
	handler.list(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

// decodeView reads the one response the viewer gets: bytes and classification
// together.
type viewResponse struct {
	Content   string `json:"content"`
	Size      int64  `json:"size"`
	IsBinary  bool   `json:"isBinary"`
	IsLarge   bool   `json:"isLarge"`
	ETag      string `json:"etag"`
	Unchanged bool   `json:"unchanged"`
}

func decodeView(t *testing.T, w *httptest.ResponseRecorder) viewResponse {
	t.Helper()
	var view viewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, w.Body.String())
	}
	return view
}

// readView issues one GET, optionally carrying the etag of a version the
// caller already has.
func readView(t *testing.T, handler *filesHandler, path, known string) viewResponse {
	t.Helper()
	url := "/files/content?path=" + path
	if known != "" {
		url += "&known=" + known
	}
	req := httptest.NewRequest("GET", url, nil)
	w := httptest.NewRecorder()
	handler.readContent(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	return decodeView(t, w)
}

// The poll re-reads an open file every 30s and the file has usually not moved,
// so the round trip that sends nothing back is the common one.
func TestFilesHandlerReadContent_Unchanged(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "poll.txt")
	os.WriteFile(p, []byte("watch me"), 0644)
	handler := &filesHandler{}

	first := readView(t, handler, p, "")
	if first.ETag == "" {
		t.Fatal("expected an etag on the first read")
	}

	again := readView(t, handler, p, first.ETag)
	if !again.Unchanged {
		t.Error("unchanged = false, want true for a file that did not move")
	}
	if again.Content != "" {
		t.Errorf("content = %q, want empty when unchanged", again.Content)
	}

	os.WriteFile(p, []byte("rewritten by an agent"), 0644)
	after := readView(t, handler, p, first.ETag)
	if after.Unchanged {
		t.Fatal("unchanged = true, want false after a rewrite")
	}
	if after.Content != "rewritten by an agent" {
		t.Errorf("content = %q, want the new bytes", after.Content)
	}
}

func TestFilesHandlerReadContent(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "read.txt")
	os.WriteFile(p, []byte("stream me"), 0644)

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
	w := httptest.NewRecorder()
	handler.readContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	view := decodeView(t, w)
	if view.Content != "stream me" {
		t.Errorf("content = %q, want %q", view.Content, "stream me")
	}
	if view.Size != 9 {
		t.Errorf("size = %d, want 9", view.Size)
	}
	if view.IsBinary || view.IsLarge {
		t.Errorf("isBinary = %v, isLarge = %v, want both false", view.IsBinary, view.IsLarge)
	}
}

// Classification and bytes come from the same response, so a viewer can never
// act on one version's size with another version's content.
func TestFilesHandlerReadContent_Binary(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "blob.bin")
	os.WriteFile(p, []byte{'M', 'Z', 0x00, 0x01, 0x02}, 0644)

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
	w := httptest.NewRecorder()
	handler.readContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	view := decodeView(t, w)
	if !view.IsBinary {
		t.Error("isBinary = false, want true for a file with null bytes")
	}
	if view.Content != "" {
		t.Errorf("content = %q, want empty for a binary file", view.Content)
	}
}

// Over the ceiling the node reports the size and sends nothing, rather than
// making the browser decode bytes Monaco will refuse to render.
func TestFilesHandlerReadContent_TooLarge(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "big.txt")
	if err := os.WriteFile(p, bytes.Repeat([]byte("a"), int(files.ViewerMaxBytes)+1), 0644); err != nil {
		t.Fatal(err)
	}

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
	w := httptest.NewRecorder()
	handler.readContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	view := decodeView(t, w)
	if !view.IsLarge {
		t.Error("isLarge = false, want true past the viewer ceiling")
	}
	if view.Content != "" {
		t.Errorf("content is %d bytes, want empty for an oversized file", len(view.Content))
	}
}

func TestFilesHandlerReadContent_NotFound(t *testing.T) {
	dir := homeTempDir(t)
	missingPath := filepath.Join(dir, "nonexistent.txt")

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/content?path="+missingPath, nil)
	w := httptest.NewRecorder()
	handler.readContent(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestFilesHandlerReadContent_Directory(t *testing.T) {
	dir := homeTempDir(t)
	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/content?path="+dir, nil)
	w := httptest.NewRecorder()
	handler.readContent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for directory", w.Code)
	}
}

// The editor is read-only, so nothing writes files through the API. An
// unreachable write endpoint is reach for nothing.
func TestWriteContentRouteIsGone(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	deps := Deps{} // filesHandler has no deps
	router := NewRouter(deps)
	req := httptest.NewRequest("PUT", "/files/content?path="+p, strings.NewReader("nope"))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PUT /files/content = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Errorf("file content = %q, want it untouched", data)
	}
}

func TestFilesHandlerOutsideHome(t *testing.T) {
	// Create a temp dir outside $HOME (system temp is typically /tmp, not in home).
	outsideHome, err := os.MkdirTemp("", "argus-outside-home-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(outsideHome) })
	os.WriteFile(filepath.Join(outsideHome, "readable.txt"), []byte("hello"), 0644)
	os.MkdirAll(filepath.Join(outsideHome, "subdir"), 0755)

	handler := &filesHandler{}

	t.Run("list outside-home directory succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files?path="+outsideHome, nil)
		w := httptest.NewRecorder()
		handler.list(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "readable.txt") {
			t.Errorf("expected readable.txt in response, got %s", body)
		}
	})

	t.Run("list root succeeds", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files?path=/", nil)
		w := httptest.NewRecorder()
		handler.list(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200 for /", w.Code)
		}
	})

	t.Run("read outside-home file succeeds", func(t *testing.T) {
		p := filepath.Join(outsideHome, "readable.txt")
		req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
		w := httptest.NewRecorder()
		handler.readContent(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if view := decodeView(t, w); view.Content != "hello" {
			t.Errorf("content = %q, want %q", view.Content, "hello")
		}
	})

	t.Run("dotdot traversal is cleaned and allowed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files?path="+outsideHome+"/subdir/..", nil)
		w := httptest.NewRecorder()
		handler.list(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("search still restricted to home", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(outsideHome, filepath.Clean(home)+string(filepath.Separator)) || outsideHome == filepath.Clean(home) {
			t.Skip("temp dir is inside $HOME on this host; cannot test outside-home search restriction")
		}
		req := httptest.NewRequest("GET", "/files/search?q=test&path="+outsideHome, nil)
		w := httptest.NewRecorder()
		handler.search(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400 for search outside home", w.Code)
		}
	})
}

func TestFilesHandlerPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	// Create an unreadable directory
	forbidden, err := os.MkdirTemp("", "argus-forbidden-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.Chmod(forbidden, 0755) // restore so cleanup can remove
		os.RemoveAll(forbidden)
	})
	os.Chmod(forbidden, 0000)

	handler := &filesHandler{}

	t.Run("list permission denied returns 403", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files?path="+forbidden, nil)
		w := httptest.NewRecorder()
		handler.list(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})

	t.Run("read permission denied returns 403", func(t *testing.T) {
		p := filepath.Join(forbidden, "file.txt")
		req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
		w := httptest.NewRecorder()
		handler.readContent(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", w.Code)
		}
	})
}

func TestFilesHandlerViaRouter(t *testing.T) {
	dir := homeTempDir(t)
	os.WriteFile(filepath.Join(dir, "routed.txt"), []byte("via router"), 0644)

	uploadDir := t.TempDir()
	router := NewRouter(Deps{UploadDirOverride: uploadDir})

	t.Run("GET /files", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/files?path="+dir, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})

	// The viewer's classification came from here; folding it into the content
	// response is what removed the gap between the two reads.
	t.Run("GET /files/meta is gone", func(t *testing.T) {
		p := filepath.Join(dir, "routed.txt")
		req := httptest.NewRequest("GET", "/files/meta?path="+p, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("GET /files/meta = %d, want %d", w.Code, http.StatusNotFound)
		}
	})

	t.Run("GET /files/content", func(t *testing.T) {
		p := filepath.Join(dir, "routed.txt")
		req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if view := decodeView(t, w); view.Content != "via router" {
			t.Errorf("content = %q, want %q", view.Content, "via router")
		}
	})

	t.Run("POST /files/upload", func(t *testing.T) {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		part, err := writer.CreateFormFile("files", "routed-upload.txt")
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte("routed content"))
		writer.Close()

		req := httptest.NewRequest("POST", "/files/upload", &buf)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
	})
}

func TestFilesHandlerUpload(t *testing.T) {
	// Create a multipart body with a test file
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("files", "test-image.png")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("fake image content"))
	writer.Close()

	handler := &filesHandler{uploadDirOverride: t.TempDir()}
	req := httptest.NewRequest("POST", "/files/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handler.upload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "test-image.png") {
		t.Errorf("expected filename in response, got %s", body)
	}
	if !strings.Contains(body, handler.uploadDirOverride) {
		t.Errorf("expected upload path in response, got %s", body)
	}
}

func TestFilesHandlerUpload_NoFiles(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.Close()

	handler := &filesHandler{uploadDirOverride: t.TempDir()}
	req := httptest.NewRequest("POST", "/files/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handler.upload(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestFilesHandlerUpload_MultipleFiles(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for _, name := range []string{"file1.txt", "file2.txt"} {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte("content of " + name))
	}
	writer.Close()

	handler := &filesHandler{uploadDirOverride: t.TempDir()}
	req := httptest.NewRequest("POST", "/files/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()
	handler.upload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "file1.txt") || !strings.Contains(body, "file2.txt") {
		t.Errorf("expected both filenames in response, got %s", body)
	}
}
