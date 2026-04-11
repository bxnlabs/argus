package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/git"
)

// initRepo creates a temporary git repo with an initial commit on "main".
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	return dir
}

func TestRun(t *testing.T) {
	dir := initRepo(t)

	// Successful command
	if err := git.Run(dir, "status"); err != nil {
		t.Errorf("Run(status): %v", err)
	}

	// Failing command
	if err := git.Run(dir, "checkout", "nonexistent-branch"); err == nil {
		t.Error("Run(checkout nonexistent) should fail")
	}
}

func TestOutput(t *testing.T) {
	dir := initRepo(t)

	out, err := git.Output(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	// Resolve symlinks for macOS /var -> /private/var
	want, _ := filepath.EvalSymlinks(dir)
	got := filepath.Clean(out[:len(out)-1]) // trim trailing newline
	got, _ = filepath.EvalSymlinks(got)
	if got != want {
		t.Errorf("Output(show-toplevel) = %q, want %q", got, want)
	}
}

func TestFindMainRepo(t *testing.T) {
	dir := initRepo(t)
	realDir, _ := filepath.EvalSymlinks(dir)

	// From repo root
	got, err := git.FindMainRepo(dir)
	if err != nil {
		t.Fatalf("FindMainRepo(root): %v", err)
	}
	if got != realDir {
		t.Errorf("FindMainRepo(root) = %q, want %q", got, realDir)
	}

	// From subdirectory
	sub := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	got, err = git.FindMainRepo(sub)
	if err != nil {
		t.Fatalf("FindMainRepo(subdir): %v", err)
	}
	if got != realDir {
		t.Errorf("FindMainRepo(subdir) = %q, want %q", got, realDir)
	}

	// From non-git directory
	nonGit := t.TempDir()
	_, err = git.FindMainRepo(nonGit)
	if err == nil {
		t.Error("FindMainRepo(non-git) should fail")
	}
}

func TestFindMainRepoFromWorktree(t *testing.T) {
	dir := initRepo(t)
	realDir, _ := filepath.EvalSymlinks(dir)

	// Create a linked worktree
	wtPath := filepath.Join(t.TempDir(), "wt")
	if err := git.Run(dir, "worktree", "add", wtPath, "-b", "test-wt"); err != nil {
		t.Fatalf("create worktree: %v", err)
	}

	got, err := git.FindMainRepo(wtPath)
	if err != nil {
		t.Fatalf("FindMainRepo(worktree): %v", err)
	}
	if got != realDir {
		t.Errorf("FindMainRepo(worktree) = %q, want %q", got, realDir)
	}
}

func TestBranchExists(t *testing.T) {
	dir := initRepo(t)

	exists, err := git.BranchExists(dir, "main")
	if err != nil {
		t.Fatalf("BranchExists(main): %v", err)
	}
	if !exists {
		t.Error("BranchExists(main) = false, want true")
	}

	exists, err = git.BranchExists(dir, "nonexistent")
	if err != nil {
		t.Fatalf("BranchExists(nonexistent): %v", err)
	}
	if exists {
		t.Error("BranchExists(nonexistent) = true, want false")
	}
}

func TestDefaultBranch(t *testing.T) {
	dir := initRepo(t)

	branch, err := git.DefaultBranch(dir)
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("DefaultBranch = %q, want %q", branch, "main")
	}
}

func TestRemoteURL(t *testing.T) {
	dir := initRepo(t)

	// No remote configured — should fail.
	_, err := git.RemoteURL(dir)
	if err == nil {
		t.Error("RemoteURL with no remote should fail")
	}

	// Add origin remote and verify URL returned.
	wantURL := "https://github.com/example/repo.git"
	cmd := exec.Command("git", "remote", "add", "origin", wantURL)
	cmd.Dir = dir
	if out, err2 := cmd.CombinedOutput(); err2 != nil {
		t.Fatalf("git remote add: %v\n%s", err2, out)
	}

	got, err := git.RemoteURL(dir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if got != wantURL {
		t.Errorf("RemoteURL = %q, want %q", got, wantURL)
	}

	// From non-git directory — should fail.
	nonGit := t.TempDir()
	_, err = git.RemoteURL(nonGit)
	if err == nil {
		t.Error("RemoteURL from non-git dir should fail")
	}
}

func TestSanitizeRemoteURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "HTTPS plain",
			input: "https://github.com/example/repo.git",
			want:  "https://github.com/example/repo.git",
		},
		{
			name:  "HTTPS with user and password",
			input: "https://user:pass@github.com/example/repo.git",
			want:  "https://github.com/example/repo.git",
		},
		{
			name:  "HTTPS with user only",
			input: "https://token@github.com/example/repo.git",
			want:  "https://github.com/example/repo.git",
		},
		{
			name:  "SSH URL",
			input: "git@github.com:example/repo.git",
			want:  "git@github.com:example/repo.git",
		},
		{
			name:  "HTTP with credentials",
			input: "http://user:secret@gitlab.com/org/repo.git",
			want:  "http://gitlab.com/org/repo.git",
		},
		{
			name:  "No scheme",
			input: "github.com/example/repo",
			want:  "github.com/example/repo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := git.SanitizeRemoteURL(tc.input)
			if got != tc.want {
				t.Errorf("SanitizeRemoteURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRemoteTrackingBranchExists(t *testing.T) {
	dir := initRepo(t)
	exists, err := git.RemoteTrackingBranchExists(dir, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected false for repo with no remote")
	}
}

func TestHasRemote(t *testing.T) {
	dir := initRepo(t)
	if git.HasRemote(dir) {
		t.Error("expected no remote for fresh repo")
	}
}

func TestLsRemoteBranches(t *testing.T) {
	remote := initRepo(t)
	cmd := exec.Command("git", "branch", "feature/test")
	cmd.Dir = remote
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v\n%s", err, out)
	}

	branches, err := git.LsRemoteBranches(remote)
	if err != nil {
		t.Fatalf("LsRemoteBranches: %v", err)
	}

	found := map[string]bool{}
	for _, b := range branches {
		found[b] = true
	}
	if !found["main"] {
		t.Error("expected 'main' in branches")
	}
	if !found["feature/test"] {
		t.Error("expected 'feature/test' in branches")
	}
}
