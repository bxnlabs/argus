package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRenderNewOutput_ID(t *testing.T) {
	raw := json.RawMessage(`{"id":"sess_abc","name":"foo"}`)
	var buf bytes.Buffer
	if err := renderNewOutput(&buf, raw, sessionInfo{ID: "sess_abc"}, false); err != nil {
		t.Fatalf("renderNewOutput: %v", err)
	}
	if got := buf.String(); got != "sess_abc\n" {
		t.Errorf("output = %q, want %q", got, "sess_abc\n")
	}
}

func TestRenderNewOutput_JSON(t *testing.T) {
	raw := json.RawMessage(`{"id":"sess_abc","name":"foo"}`)
	var buf bytes.Buffer
	if err := renderNewOutput(&buf, raw, sessionInfo{ID: "sess_abc"}, true); err != nil {
		t.Fatalf("renderNewOutput: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\"id\": \"sess_abc\"") {
		t.Errorf("output = %q, want indented json with id", out)
	}
}

func TestCreateCmd_NoArgs(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing name arg")
	}
}

// TestCreateCmd_AttachJSONMutuallyExclusive verifies the new --attach/--json
// conflict is rejected at the command layer: --attach never prints the record,
// so --json would be silently ignored.
func TestCreateCmd_AttachJSONMutuallyExclusive(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetArgs([]string{"foo", "--attach", "--json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when --attach and --json are used together")
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was
// written. renderNewOutput writes the machine-facing result to os.Stdout
// directly, so command-flow assertions must capture it here.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestCreateCmd_DefaultOutputsID drives `session new` end-to-end against a fake
// node and asserts the default (non-attach) mode sends the expected request and
// prints the bare session ID.
func TestCreateCmd_DefaultOutputsID(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/node/sessions" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]any{
				"id": "sess_xyz", "name": "foo", "provider_type": "claude",
			},
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	writeTestDiscovery(t, dir, os.Getpid(), strings.TrimPrefix(srv.URL, "http://"))

	out := captureStdout(t, func() {
		cmd := newCreateCmd()
		cmd.SetArgs([]string{"foo"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if out != "sess_xyz\n" {
		t.Errorf("stdout = %q, want the bare session ID %q", out, "sess_xyz\n")
	}
	if gotBody["name"] != "foo" {
		t.Errorf("request name = %v, want foo", gotBody["name"])
	}
	if gotBody["provider_type"] != "claude" {
		t.Errorf("request provider_type = %v, want claude", gotBody["provider_type"])
	}
	if _, ok := gotBody["source"]; !ok {
		t.Error("request missing source field")
	}
	if gotBody["auto_approve"] != true {
		t.Errorf("request auto_approve = %v, want true (yolo default)", gotBody["auto_approve"])
	}
}
