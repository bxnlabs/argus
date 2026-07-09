package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
)

// initHomeGitRepo creates a git repo under $HOME (so SafeExpandPath accepts its
// path) with an initial commit on main, and returns its path.
func initHomeGitRepo(t *testing.T) string {
	t.Helper()
	dir := homeTempDir(t) // shared helper in files_test.go
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
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// newWorktreeHandler builds a worktreeHandler with a manager rooted at a
// resolved temp state dir (resolved so IsManagedPath matches git's
// symlink-resolved worktree paths on macOS).
func newWorktreeHandler(t *testing.T) *worktreeHandler {
	t.Helper()
	stateDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &worktreeHandler{mgr: worktree.NewManager(stateDir, &config.Config{})}
}

func doCreate(t *testing.T, h *worktreeHandler, repo, branch string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", "/git/worktree?path="+repo+"&branch="+branch, nil)
	w := httptest.NewRecorder()
	h.create(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("create: unmarshal: %v", err)
	}
	return resp
}

func listWorktreesResp(t *testing.T, h *worktreeHandler, repo string) []worktree.ManagedWorktree {
	t.Helper()
	req := httptest.NewRequest("GET", "/git/worktrees?path="+repo, nil)
	w := httptest.NewRecorder()
	h.list(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Worktrees []worktree.ManagedWorktree `json:"worktrees"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("list: unmarshal: %v", err)
	}
	return resp.Worktrees
}

func TestWorktreeHandler_CreateAndList(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)

	resp := doCreate(t, h, repo, "feature-x")
	if resp["created"] != true {
		t.Errorf("created = %v, want true", resp["created"])
	}
	if resp["branch"] != "feature-x" {
		t.Errorf("branch = %v, want feature-x", resp["branch"])
	}

	got := listWorktreesResp(t, h, repo)
	if len(got) != 1 || got[0].Branch != "feature-x" {
		t.Errorf("list = %+v, want one feature-x", got)
	}
}

func TestWorktreeHandler_CreateReuse(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)

	first := doCreate(t, h, repo, "feature-x")
	if first["created"] != true {
		t.Errorf("first created = %v, want true", first["created"])
	}
	second := doCreate(t, h, repo, "feature-x")
	if second["created"] != false {
		t.Errorf("second created = %v, want false (reuse)", second["created"])
	}
}

func TestWorktreeHandler_Delete(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	doCreate(t, h, repo, "feature-x")

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=feature-x", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if got := listWorktreesResp(t, h, repo); len(got) != 0 {
		t.Errorf("after delete list = %+v, want empty", got)
	}
}

func TestWorktreeHandler_DeleteMissing(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=nope", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWorktreeHandler_DeleteDirty(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	resp := doCreate(t, h, repo, "feature-x")
	wtPath, _ := resp["path"].(string)

	// Untracked file makes `git worktree remove` (non-force) fail as dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=feature-x", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("dirty delete: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWorktreeHandler_DeleteUnmanaged(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)

	// Create an UNMANAGED linked worktree by hand, outside the manager's state
	// dir, mirroring TestListManaged's "loose" worktree in manager_test.go.
	unmanaged := filepath.Join(t.TempDir(), "loose")
	cmd := exec.Command("git", "worktree", "add", "-b", "loose-branch", unmanaged)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	// It must be invisible to list (already covered by ListManaged), and
	// delete must refuse to touch it: 404, not 200.
	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=loose-branch", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("delete unmanaged: expected 404, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := os.Stat(unmanaged); err != nil {
		t.Errorf("unmanaged worktree should still exist on disk after refused delete: %v", err)
	}
}

func TestWorktreeHandler_CreateMissingParams(t *testing.T) {
	h := newWorktreeHandler(t)
	req := httptest.NewRequest("POST", "/git/worktree?path=/tmp", nil) // no branch
	w := httptest.NewRecorder()
	h.create(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
