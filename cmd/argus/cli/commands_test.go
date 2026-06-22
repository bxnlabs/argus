package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCheckStatus_OK(t *testing.T) {
	if err := checkStatus([]byte(`{}`), 200, "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckStatus_ErrorWithMessage(t *testing.T) {
	body := []byte(`{"error":"session not found"}`)
	err := checkStatus(body, 404, "delete")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "delete failed: session not found") {
		t.Errorf("error = %q, want to contain %q", err, "delete failed: session not found")
	}
}

func TestCheckStatus_ErrorNoMessage(t *testing.T) {
	err := checkStatus([]byte(`{}`), 500, "create")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "create failed (HTTP 500)") {
		t.Errorf("error = %q, want to contain %q", err, "create failed (HTTP 500)")
	}
}

func TestPostLongRunning_OpLabel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":"profile in use"}`))
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	_, err := c.postLongRunning("/profiles/acme/down", nil, "stop profile stack")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "stop profile stack failed: profile in use") {
		t.Errorf("error = %q, want to contain %q", err, "stop profile stack failed: profile in use")
	}
}

func TestFetchAndResolve_OK(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc", Name: "my-session", TmuxName: "claude-sess_abc"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, _ := json.Marshal(map[string]any{"sessions": sessions})
		w.Header().Set("Content-Type", "application/json")
		w.Write(resp)
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	s, err := fetchAndResolve(c, "my-session")
	if err != nil {
		t.Fatalf("fetchAndResolve: %v", err)
	}
	if s.ID != "sess_abc" {
		t.Errorf("id = %q, want %q", s.ID, "sess_abc")
	}
}

func TestFetchAndResolve_NoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sessions":[]}`))
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	_, err := fetchAndResolve(c, "nonexistent")
	if err == nil {
		t.Fatal("expected error for no match")
	}
}

func TestNewClient_AcceptsLoopback(t *testing.T) {
	dir := t.TempDir()
	path := writeTestDiscovery(t, dir, os.Getpid(), "127.0.0.1:3000")
	c, err := newClient(path)
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if c.baseURL != "http://127.0.0.1:3000/api/node" {
		t.Errorf("baseURL = %q", c.baseURL)
	}
}

func TestDeleteCmd_NoArgs(t *testing.T) {
	cmd := newDeleteCmd()
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing args")
	}
}

func TestRenameCmd_TooFewArgs(t *testing.T) {
	cmd := newRenameCmd()
	cmd.SetArgs([]string{"only-one"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for too few args")
	}
}
