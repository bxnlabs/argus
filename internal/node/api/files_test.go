package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

func TestFilesHandlerMeta(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "test.txt")
	os.WriteFile(p, []byte("hello world"), 0644)

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/meta?path="+p, nil)
	w := httptest.NewRecorder()
	handler.meta(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"size":11`) {
		t.Errorf("expected size 11, got %s", body)
	}
	if !strings.Contains(body, `"isBinary":false`) {
		t.Errorf("expected isBinary false, got %s", body)
	}
}

func TestFilesHandlerMeta_NotFound(t *testing.T) {
	dir := homeTempDir(t)
	missingPath := filepath.Join(dir, "nonexistent.txt")

	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/meta?path="+missingPath, nil)
	w := httptest.NewRecorder()
	handler.meta(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestFilesHandlerMeta_Directory(t *testing.T) {
	dir := homeTempDir(t)
	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/files/meta?path="+dir, nil)
	w := httptest.NewRecorder()
	handler.meta(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for directory", w.Code)
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
	if w.Body.String() != "stream me" {
		t.Errorf("body = %q, want %q", w.Body.String(), "stream me")
	}
	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Error("expected Content-Type header")
	}
	if w.Header().Get("Last-Modified") == "" {
		t.Error("expected Last-Modified header")
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

func TestFilesHandlerWriteContent(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "write.txt")

	handler := &filesHandler{}
	body := strings.NewReader("written via PUT")
	req := httptest.NewRequest("PUT", "/files/content?path="+p, body)
	w := httptest.NewRecorder()
	handler.writeContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "written via PUT" {
		t.Errorf("file content = %q, want %q", data, "written via PUT")
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"size":15`) {
		t.Errorf("expected size 15 in response, got %s", resp)
	}
	if !strings.Contains(resp, `"path":`+`"`+p) {
		t.Errorf("expected expanded path in response, got %s", resp)
	}
}

func TestFilesHandlerWriteContent_CreatesParentDirs(t *testing.T) {
	dir := homeTempDir(t)
	p := filepath.Join(dir, "a", "b", "deep.txt")

	handler := &filesHandler{}
	body := strings.NewReader("deep write")
	req := httptest.NewRequest("PUT", "/files/content?path="+p, body)
	w := httptest.NewRecorder()
	handler.writeContent(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	data, _ := os.ReadFile(p)
	if string(data) != "deep write" {
		t.Errorf("content = %q, want %q", data, "deep write")
	}
}

func TestFilesHandlerWriteContent_MissingPath(t *testing.T) {
	handler := &filesHandler{}
	req := httptest.NewRequest("PUT", "/files/content", strings.NewReader("data"))
	w := httptest.NewRecorder()
	handler.writeContent(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
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

	t.Run("meta outside-home file succeeds", func(t *testing.T) {
		p := filepath.Join(outsideHome, "readable.txt")
		req := httptest.NewRequest("GET", "/files/meta?path="+p, nil)
		w := httptest.NewRecorder()
		handler.meta(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
	})

	t.Run("read outside-home file succeeds", func(t *testing.T) {
		p := filepath.Join(outsideHome, "readable.txt")
		req := httptest.NewRequest("GET", "/files/content?path="+p, nil)
		w := httptest.NewRecorder()
		handler.readContent(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if w.Body.String() != "hello" {
			t.Errorf("body = %q, want %q", w.Body.String(), "hello")
		}
	})

	t.Run("write outside-home file succeeds", func(t *testing.T) {
		p := filepath.Join(outsideHome, "written.txt")
		req := httptest.NewRequest("PUT", "/files/content?path="+p, strings.NewReader("data"))
		w := httptest.NewRecorder()
		handler.writeContent(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
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

	t.Run("meta permission denied returns 403", func(t *testing.T) {
		p := filepath.Join(forbidden, "file.txt")
		req := httptest.NewRequest("GET", "/files/meta?path="+p, nil)
		w := httptest.NewRecorder()
		handler.meta(w, req)
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

	t.Run("write permission denied returns 403", func(t *testing.T) {
		p := filepath.Join(forbidden, "file.txt")
		req := httptest.NewRequest("PUT", "/files/content?path="+p, strings.NewReader("data"))
		w := httptest.NewRecorder()
		handler.writeContent(w, req)
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

	t.Run("GET /files/meta", func(t *testing.T) {
		p := filepath.Join(dir, "routed.txt")
		req := httptest.NewRequest("GET", "/files/meta?path="+p, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
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
		if w.Body.String() != "via router" {
			t.Errorf("body = %q, want %q", w.Body.String(), "via router")
		}
	})

	t.Run("PUT /files/content", func(t *testing.T) {
		p := filepath.Join(dir, "new-via-router.txt")
		req := httptest.NewRequest("PUT", "/files/content?path="+p, strings.NewReader("put data"))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
		}
		data, _ := os.ReadFile(p)
		if string(data) != "put data" {
			t.Errorf("file = %q, want %q", data, "put data")
		}
	})

	t.Run("old POST /files/content returns 405", func(t *testing.T) {
		p := filepath.Join(dir, "old.txt")
		req := httptest.NewRequest("POST", "/files/content?path="+p, strings.NewReader("post data"))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Logf("POST /api/files/content returned %d (expected 405)", w.Code)
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
