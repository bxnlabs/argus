package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initWtGitRepo creates a temporary git repo with an initial commit on main and
// returns its path.
func initWtGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// TestResolveRepoRootFromLinkedWorktree verifies that resolveRepoRoot returns
// the MAIN repo root even when run from inside a linked worktree, so worktree
// operations are keyed by the main repo (matching how node sessions key repos)
// rather than by the current worktree path.
func TestResolveRepoRootFromLinkedWorktree(t *testing.T) {
	main := initWtGitRepo(t)
	linked := filepath.Join(t.TempDir(), "linked")
	cmd := exec.Command("git", "worktree", "add", "-b", "wt-branch", linked)
	cmd.Dir = main
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	t.Chdir(linked)
	got, err := resolveRepoRoot()
	if err != nil {
		t.Fatalf("resolveRepoRoot: %v", err)
	}
	want, err := filepath.EvalSymlinks(main)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("resolveRepoRoot from linked worktree = %q, want main repo %q", got, want)
	}
}

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
