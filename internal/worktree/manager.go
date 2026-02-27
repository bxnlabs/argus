package worktree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/source"
)

// ErrWorktreeDirty is returned when attempting to remove a worktree that has
// uncommitted changes without the force flag.
var ErrWorktreeDirty = errors.New("worktree has uncommitted changes")

// Manager handles git worktree creation and remote repo cloning.
type Manager struct {
	stateDir string
	cfg      *config.Config
}

// NewManager creates a new worktree Manager.
// stateDir is the ~/.argus directory; cfg is the loaded user config.
func NewManager(stateDir string, cfg *config.Config) *Manager {
	return &Manager{stateDir: stateDir, cfg: cfg}
}

// CreateForLocalRepo creates an isolated git worktree for a local git repo.
// gitRoot must be the absolute path to the repo root.
// Returns the worktree path and the git branch name.
func (m *Manager) CreateForLocalRepo(gitRoot, sessionName string) (worktreePath, branch string, err error) {
	src := &source.Source{LocalPath: gitRoot}
	return m.createWorktree(gitRoot, src.ParentKey(), sessionName)
}

// EnsureClone clones the remote repo if not already cloned, or fetches
// updates if it is. Returns the clone directory path.
func (m *Manager) EnsureClone(src *source.Source) (string, error) {
	cloneDir := filepath.Join(m.stateDir, "projects", src.ParentKey(), "gitrepo")

	_, statErr := os.Stat(cloneDir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return "", fmt.Errorf("stat clone dir: %w", statErr)
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0755); err != nil {
			return "", fmt.Errorf("create project dir: %w", err)
		}
		if err := runGit("", "clone", src.RemoteURL, cloneDir); err != nil {
			os.RemoveAll(cloneDir)
			return "", fmt.Errorf("clone repo: %w", err)
		}
	} else {
		defaultBranch, err := getDefaultBranch(cloneDir)
		if err != nil {
			return "", err
		}
		if err := runGit(cloneDir, "fetch", "origin"); err != nil {
			return "", fmt.Errorf("fetch: %w", err)
		}
		if err := runGit(cloneDir, "checkout", defaultBranch); err != nil {
			return "", fmt.Errorf("checkout default branch: %w", err)
		}
		if err := runGit(cloneDir, "reset", "--hard", "origin/"+defaultBranch); err != nil {
			return "", fmt.Errorf("reset to origin: %w", err)
		}
	}
	return cloneDir, nil
}

// CreateForRemoteRepo clones (or fetches) the remote repo and creates a worktree.
// Returns the worktree path and the git branch name.
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string) (worktreePath, branch string, err error) {
	cloneDir, err := m.EnsureClone(src)
	if err != nil {
		return "", "", err
	}
	return m.createWorktree(cloneDir, src.ParentKey(), sessionName)
}

// FindWorktree checks whether a git worktree already exists for the given
// branch. repoDir can be the main repo root or any existing worktree
// directory (git worktree list works from either).
// Returns the worktree path if found, empty string if not.
func (m *Manager) FindWorktree(repoDir, branch string) (string, error) {
	out, err := gitOutput(repoDir, "worktree", "list")
	if err != nil {
		return "", fmt.Errorf("git worktree list: %w", err)
	}

	target := "[" + branch + "]"
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[2] == target {
			// Note: git worktree list resolves symlinks (e.g.
			// /var -> /private/var on macOS), so the returned path
			// may differ from the path originally passed to
			// "git worktree add".
			return fields[0], nil
		}
	}
	return "", nil
}

func (m *Manager) createWorktree(repoDir, parentKey, sessionName string) (worktreePath, branch string, err error) {
	slug := slugify(sessionName)
	baseBranch := m.branchName(slug)

	// Check if a worktree already exists for this branch.
	existing, err := m.FindWorktree(repoDir, baseBranch)
	if err != nil {
		return "", "", err
	}
	if existing != "" {
		return existing, baseBranch, nil
	}

	branch, err = m.uniqueBranch(repoDir, baseBranch)
	if err != nil {
		return "", "", err
	}

	defaultBranch, err := getDefaultBranch(repoDir)
	if err != nil {
		return "", "", err
	}

	worktreePath = filepath.Join(m.stateDir, "projects", parentKey, "worktrees", worktreeDirName(branch))

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", "", fmt.Errorf("create worktrees dir: %w", err)
	}

	if err := runGit(repoDir, "worktree", "add", worktreePath, "-b", branch, defaultBranch); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w", err)
	}

	return worktreePath, branch, nil
}

func (m *Manager) branchName(slug string) string {
	if m.cfg.Git.BranchPrefix != "" {
		return m.cfg.Git.BranchPrefix + "/" + slug
	}
	return slug
}

func (m *Manager) uniqueBranch(repoDir, branch string) (string, error) {
	exists, err := branchExists(repoDir, branch)
	if err != nil {
		return "", err
	}
	if !exists {
		return branch, nil
	}
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", branch, i)
		exists, err := branchExists(repoDir, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find unique branch name for %s", branch)
}

// RemoveWorktree removes a git worktree directory. The branch is intentionally
// preserved so the user can recover work.
//
// When force is false and the worktree contains uncommitted changes,
// ErrWorktreeDirty is returned. When force is true, the worktree is removed
// unconditionally.
func (m *Manager) RemoveWorktree(worktreePath string, force bool) error {
	repoDir, err := mainRepoFromWorktree(worktreePath)
	if err != nil {
		return fmt.Errorf("locate parent repo: %w", err)
	}

	args := []string{"worktree", "remove", worktreePath}
	if force {
		args = []string{"worktree", "remove", "--force", worktreePath}
	}

	if err := runGit(repoDir, args...); err != nil {
		errMsg := err.Error()
		if !force && (strings.Contains(errMsg, "contains modified or untracked files") ||
			strings.Contains(errMsg, "is dirty")) {
			return fmt.Errorf("%w: use --force to delete anyway", ErrWorktreeDirty)
		}
		return fmt.Errorf("git worktree remove: %w", err)
	}
	return nil
}

// Cleanup is a best-effort removal used during create-failure rollback.
// It force-removes the worktree since no user work exists yet.
func (m *Manager) Cleanup(worktreePath string) {
	_ = m.RemoveWorktree(worktreePath, true)
}

// mainRepoFromWorktree reads the linked worktree's .git file to find the path
// of the main repository. A linked worktree has a .git FILE (not directory)
// containing "gitdir: /path/to/main/.git/worktrees/<name>".
func mainRepoFromWorktree(worktreePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(worktreePath, ".git"))
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unexpected .git format in %s", worktreePath)
	}
	// gitDir = /abs/path/to/main/.git/worktrees/<name>
	// main repo = Dir(Dir(Dir(gitDir)))
	gitDir := line[len(prefix):]
	return filepath.Dir(filepath.Dir(filepath.Dir(gitDir))), nil
}

func branchExists(repoDir, branch string) (bool, error) {
	out, err := gitOutput(repoDir, "branch", "--list", branch)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// getDefaultBranch returns the repo's default branch name.
// Tries: origin/HEAD symbolic ref → local "main" → local "master" →
// remote origin/main → remote origin/master → error.
func getDefaultBranch(repoDir string) (string, error) {
	out, err := gitOutput(repoDir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		ref := strings.TrimSpace(out)
		if ref != "" {
			parts := strings.Split(ref, "/")
			return parts[len(parts)-1], nil
		}
	}

	for _, branch := range []string{"main", "master"} {
		exists, err := branchExists(repoDir, branch)
		if err == nil && exists {
			return branch, nil
		}
	}

	for _, branch := range []string{"main", "master"} {
		out, err := gitOutput(repoDir, "branch", "-r", "--list", "origin/"+branch)
		if err == nil && strings.TrimSpace(out) != "" {
			return branch, nil
		}
	}

	return "", fmt.Errorf("cannot determine default branch for repo at %s", repoDir)
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
