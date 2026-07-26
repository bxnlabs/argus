package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
)

// initGitRepoAt initializes a git repo with an initial commit on main at dir.
func initGitRepoAt(t *testing.T, dir string) {
	t.Helper()
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
}

// initHomeGitRepo creates a git repo under $HOME with an initial commit on main
// and returns its path. Repos live under $HOME here as a convenient default; the
// worktree route no longer confines the source path to $HOME (see
// resolveRepoParam and TestWorktreeHandler_CreateOutsideHome).
func initHomeGitRepo(t *testing.T) string {
	t.Helper()
	dir := homeTempDir(t) // shared helper in files_test.go
	initGitRepoAt(t, dir)
	return dir
}

// newWorktreeHandler builds a worktreeHandler with a manager rooted at a
// resolved temp state dir (resolved so IsManagedPath matches git's
// symlink-resolved worktree paths on macOS) and a fresh test database.
func newWorktreeHandler(t *testing.T) *worktreeHandler {
	t.Helper()
	stateDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return &worktreeHandler{mgr: worktree.NewManager(stateDir, &config.Config{}), db: database}
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

// TestWorktreeHandler_CreateOutsideHome verifies the source repo is not confined
// to $HOME: a repo under a plain temp dir (outside $HOME on macOS/Linux) is
// accepted, and its worktree is created and listed. This locks in the Model A
// relaxation (canonicalize-only, trust the network/OS boundary) — under the old
// SafeExpandPath guard this create returned 400.
func TestWorktreeHandler_CreateOutsideHome(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := t.TempDir() // outside $HOME
	initGitRepoAt(t, repo)

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

func TestWorktreeHandler_DeleteDirtyForce(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	resp := doCreate(t, h, repo, "feature-x")
	wtPath, _ := resp["path"].(string)

	// Same dirty worktree as above, but force=true discards the changes and
	// removes it — the branch is still preserved by RemoveWorktree.
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=feature-x&force=true", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("dirty force delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree should be gone after force delete, stat err = %v", err)
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

func TestWorktreeHandler_CreateReuseUnmanaged(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)

	// A hand-made, unmanaged worktree for a branch, outside the manager's state
	// dir. create must not silently "reuse" it (which would return a path that
	// list hides and rm refuses) — it must reject with 409.
	unmanaged := filepath.Join(t.TempDir(), "loose")
	cmd := exec.Command("git", "worktree", "add", "-b", "loose-branch", unmanaged)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v\n%s", err, out)
	}

	req := httptest.NewRequest("POST", "/git/worktree?path="+repo+"&branch=loose-branch", nil)
	w := httptest.NewRecorder()
	h.create(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("create reuse of unmanaged worktree: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWorktreeHandler_CreateInvalidBranch(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)

	// Branch names that are user errors, not internal ones: each must yield 400,
	// not a generic 500. "bad..branch" is syntactically invalid; "-foo" is a
	// leading-dash name that check-ref-format accepts as a refname but that
	// `git worktree add -b` treats as a flag (mirrors session validation).
	for _, branch := range []string{"bad..branch", "-foo"} {
		req := httptest.NewRequest("POST", "/git/worktree?path="+repo+"&branch="+url.QueryEscape(branch), nil)
		w := httptest.NewRecorder()
		h.create(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("invalid branch %q: expected 400, got %d: %s", branch, w.Code, w.Body.String())
		}
	}
}

func TestWorktreeHandler_DeleteInUseBySession(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	resp := doCreate(t, h, repo, "feature-x")
	wtPath, _ := resp["path"].(string)

	// An active session occupies the worktree. delete must refuse (409) rather
	// than pull the directory out from under it, and leave it on disk.
	if err := h.db.CreateSession(&db.Session{
		ID: "s1", Name: "s1", TmuxName: "claude-s1", ProviderType: "claude",
		WorkingDirectory: wtPath,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=feature-x", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("in-use delete: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree should still exist on disk after refused delete: %v", err)
	}
}

func TestWorktreeHandler_DeleteInUseBySessionSubdir(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	resp := doCreate(t, h, repo, "feature-x")
	wtPath, _ := resp["path"].(string)

	// A session runs from a subdirectory of the worktree, not its root. Removing
	// the worktree would still pull the directory out from under it, so the guard
	// must match any cwd at or under the worktree — not only an exact-root match.
	if err := h.db.CreateSession(&db.Session{
		ID: "s1", Name: "s1", TmuxName: "claude-s1", ProviderType: "claude",
		WorkingDirectory: filepath.Join(wtPath, "subdir"),
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=feature-x", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("in-use (subdir cwd) delete: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree should still exist on disk after refused delete: %v", err)
	}
}

func TestWorktreeHandler_DeleteInUseBySessionSymlinkedPath(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	resp := doCreate(t, h, repo, "feature-x")
	wtPath, _ := resp["path"].(string)

	// The session persisted its working directory under a DIFFERENT spelling
	// that resolves to the same worktree (as reuse-by-path under a symlinked
	// ARGUS_HOME would). Exact-string matching would miss it; the guard must
	// compare by symlink-resolved path.
	alt := filepath.Join(t.TempDir(), "alt-worktree")
	if err := os.Symlink(wtPath, alt); err != nil {
		t.Fatal(err)
	}
	if err := h.db.CreateSession(&db.Session{
		ID: "s1", Name: "s1", TmuxName: "claude-s1", ProviderType: "claude",
		WorkingDirectory: alt,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/git/worktree?path="+repo+"&branch=feature-x", nil)
	w := httptest.NewRecorder()
	h.delete(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("in-use (symlinked spelling) delete: expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Errorf("worktree should still exist on disk after refused delete: %v", err)
	}
}

func TestWorktreeHandler_CreateBranchCheckedOut(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t) // on "main" with a commit

	// Requesting a worktree for the branch checked out in the main working tree
	// is a user conflict (409), not a generic internal error (500).
	req := httptest.NewRequest("POST", "/git/worktree?path="+repo+"&branch=main", nil)
	w := httptest.NewRecorder()
	h.create(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("checked-out branch: expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWorktreeHandler_CreateNormalizesSubdirPath(t *testing.T) {
	h := newWorktreeHandler(t)
	repo := initHomeGitRepo(t)
	sub := filepath.Join(repo, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	fromRoot := doCreate(t, h, repo, "from-root")
	fromSub := doCreate(t, h, sub, "from-sub")

	// Passing a subdirectory must be normalized to the main repo root before
	// keying, so both worktrees land under the same projects/<key>/worktrees
	// parent.
	rootParent := filepath.Dir(fromRoot["path"].(string))
	subParent := filepath.Dir(fromSub["path"].(string))
	if rootParent != subParent {
		t.Errorf("subdir path keyed differently:\n root: %s\n sub:  %s", rootParent, subParent)
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
