package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeTestDiscovery(t *testing.T, dir string, pid int, address string) string {
	t.Helper()
	path := filepath.Join(dir, "node.json")
	data, _ := json.Marshal(map[string]any{"pid": pid, "address": address})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscover_Valid(t *testing.T) {
	dir := t.TempDir()
	// Use current process PID so the liveness check passes.
	path := writeTestDiscovery(t, dir, os.Getpid(), "127.0.0.1:3000")

	info, err := discover(path)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if info.Address != "127.0.0.1:3000" {
		t.Errorf("address = %q, want %q", info.Address, "127.0.0.1:3000")
	}
}

func TestDiscover_MissingFile(t *testing.T) {
	_, err := discover("/tmp/nonexistent-argus-test.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDiscover_StalePID(t *testing.T) {
	dir := t.TempDir()
	// PID 2147483647 should not be alive.
	path := writeTestDiscovery(t, dir, 2147483647, "127.0.0.1:3000")

	_, err := discover(path)
	if err == nil {
		t.Fatal("expected error for stale PID")
	}

	// File should be cleaned up.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("stale discovery file was not removed")
	}
}

func TestAPIClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/node/api/sessions" {
			t.Errorf("path = %q, want /node/api/sessions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sessions":[]}`))
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/node"}
	body, err := c.get("/api/sessions")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(body) != `{"sessions":[]}` {
		t.Errorf("body = %q", string(body))
	}
}
