package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorktreesTable(t *testing.T) {
	items := []wtItem{
		{Branch: "feature-a", Path: "/wt/a"},
		{Branch: "feature-b", Path: "/wt/b"},
	}
	got := worktreesTable(items, "/home/nobody")
	for _, want := range []string{"BRANCH", "PATH", "feature-a", "/wt/a", "feature-b", "/wt/b"} {
		if !strings.Contains(got, want) {
			t.Errorf("table missing %q:\n%s", want, got)
		}
	}
}

func TestRunWtCo(t *testing.T) {
	var gotPath, gotBranch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/node/git/worktree" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		gotPath = r.URL.Query().Get("path")
		gotBranch = r.URL.Query().Get("branch")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"path": "/wt/feature-x", "branch": "feature-x", "created": true,
		})
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	var out, errOut bytes.Buffer
	if err := runWtCo(c, "/repo", "feature-x", &out, &errOut); err != nil {
		t.Fatalf("runWtCo: %v", err)
	}
	if gotPath != "/repo" || gotBranch != "feature-x" {
		t.Errorf("request params path=%q branch=%q", gotPath, gotBranch)
	}
	if out.String() != "/wt/feature-x\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "/wt/feature-x\n")
	}
	if !strings.Contains(errOut.String(), "feature-x") {
		t.Errorf("stderr = %q, want a note about feature-x", errOut.String())
	}
}

func TestRunWtLs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/node/git/worktrees" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("path"); got != "/repo" {
			t.Errorf("path param = %q, want /repo", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"worktrees": []map[string]string{
				{"path": "/wt/a", "branch": "feature-a"},
			},
		})
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	var out bytes.Buffer
	if err := runWtLs(c, "/repo", "/home/nobody", &out); err != nil {
		t.Fatalf("runWtLs: %v", err)
	}
	for _, want := range []string{"BRANCH", "PATH", "feature-a", "/wt/a"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunWtRm(t *testing.T) {
	var gotBranch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/node/git/worktree" {
			t.Errorf("got %s %s", r.Method, r.URL.Path)
		}
		gotBranch = r.URL.Query().Get("branch")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/api/node"}
	var out bytes.Buffer
	if err := runWtRm(c, "/repo", "feature-x", &out); err != nil {
		t.Fatalf("runWtRm: %v", err)
	}
	if gotBranch != "feature-x" {
		t.Errorf("branch param = %q, want feature-x", gotBranch)
	}
	if !strings.Contains(out.String(), "feature-x") {
		t.Errorf("stdout = %q, want a confirmation", out.String())
	}
}

func TestGitCmd_HasWt(t *testing.T) {
	git := NewGitCmd()
	found := false
	for _, c := range git.Commands() {
		if c.Name() == "wt" {
			found = true
		}
	}
	if !found {
		t.Error("git command missing wt subcommand")
	}
}

func TestWtCmd_HasSubcommands(t *testing.T) {
	names := map[string]bool{}
	for _, c := range newWtCmd().Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"co", "ls", "rm"} {
		if !names[want] {
			t.Errorf("wt missing %q subcommand", want)
		}
	}
}
