package worktree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git"
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

// IsManaged reports whether the given worktree path matches the layout used
// by argus-created worktrees: <stateDir>/projects/<parentKey>/worktrees/<name>.
// It determines the parent key by resolving the worktree's parent git
// repository and deriving the key from that path.
// External worktrees (user-provided paths outside this structure) return false
// and are never deleted on session cleanup.
func (m *Manager) IsManaged(worktreePath string) bool {
	mainRepo, err := git.FindMainRepo(worktreePath)
	if err != nil {
		return false
	}

	parentKey := source.ParentKeyFromPath(mainRepo)
	name := filepath.Base(worktreePath)
	expected := filepath.Join(m.stateDir, "projects", parentKey, "worktrees", name)
	return worktreePath == expected
}

// CreateForLocalRepo creates an isolated git worktree for a local git repo.
// gitRoot must be the absolute path to the repo root.
// Returns the worktree path, git branch name, and whether the worktree was
// newly created (true) or an existing one was reused (false).
func (m *Manager) CreateForLocalRepo(gitRoot, sessionName string) (worktreePath, branch string, created bool, err error) {
	src := &source.Source{LocalPath: gitRoot}
	return m.createWorktree(gitRoot, src.ParentKey(), sessionName)
}

// EnsureClone clones the remote repo if not already cloned, or fetches
// updates if it is. When fetchOnly is false (the default for worktree
// creation), existing clones are reset to the latest default branch.
// When fetchOnly is true (used for shell session CWDs), existing clones
// are only fetched so that uncommitted user work is preserved.
func (m *Manager) EnsureClone(src *source.Source, fetchOnly bool) (string, error) {
	cloneDir := filepath.Join(m.stateDir, "projects", src.ParentKey(), "gitrepo")

	_, statErr := os.Stat(cloneDir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return "", fmt.Errorf("stat clone dir: %w", statErr)
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0755); err != nil {
			return "", fmt.Errorf("create project dir: %w", err)
		}
		if err := git.Run("", "clone", src.RemoteURL, cloneDir); err != nil {
			os.RemoveAll(cloneDir)
			return "", fmt.Errorf("clone repo: %w", err)
		}
	} else if fetchOnly {
		if err := git.Run(cloneDir, "fetch", "origin"); err != nil {
			return "", fmt.Errorf("fetch: %w", err)
		}
	} else {
		defaultBranch, err := git.DefaultBranch(cloneDir)
		if err != nil {
			return "", err
		}
		if err := git.Run(cloneDir, "fetch", "origin"); err != nil {
			return "", fmt.Errorf("fetch: %w", err)
		}
		if err := git.Run(cloneDir, "checkout", defaultBranch); err != nil {
			return "", fmt.Errorf("checkout default branch: %w", err)
		}
		if err := git.Run(cloneDir, "reset", "--hard", "origin/"+defaultBranch); err != nil {
			return "", fmt.Errorf("reset to origin: %w", err)
		}
	}
	return cloneDir, nil
}

// CreateForRemoteRepo clones (or fetches) the remote repo and creates a worktree.
// Returns the worktree path, git branch name, and whether the worktree was
// newly created (true) or an existing one was reused (false).
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string) (worktreePath, branch string, created bool, err error) {
	cloneDir, err := m.EnsureClone(src, false)
	if err != nil {
		return "", "", false, err
	}
	return m.createWorktree(cloneDir, src.ParentKey(), sessionName)
}

// FindWorktree checks whether a git worktree already exists for the given
// branch. repoDir can be the main repo root or any existing worktree
// directory (git worktree list works from either).
// Returns the worktree path if found, empty string if not.
// Only linked worktrees are considered — the main working tree is excluded.
func (m *Manager) FindWorktree(repoDir, branch string) (string, error) {
	entries, err := listWorktrees(repoDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.branch == branch {
			return e.path, nil
		}
	}
	return "", nil
}

// FindWorktreeByPath checks if the given path is a known git worktree and
// returns its branch name. Returns empty string if the path is not a worktree
// or is the main working tree.
func (m *Manager) FindWorktreeByPath(dir string) (branch string, err error) {
	entries, err := listWorktrees(dir)
	if err != nil {
		return "", err
	}

	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}

	for _, e := range entries {
		if e.path == resolvedDir {
			return e.branch, nil
		}
	}
	return "", nil
}

func (m *Manager) createWorktree(repoDir, parentKey, sessionName string) (worktreePath, branch string, created bool, err error) {
	slug := slugify(sessionName)
	baseBranch := m.branchName(slug)

	// Check if a worktree already exists for this branch.
	existing, err := m.FindWorktree(repoDir, baseBranch)
	if err != nil {
		return "", "", false, err
	}
	if existing != "" {
		return existing, baseBranch, false, nil
	}

	branch, err = m.uniqueBranch(repoDir, baseBranch)
	if err != nil {
		return "", "", false, err
	}

	defaultBranch, err := git.DefaultBranch(repoDir)
	if err != nil {
		return "", "", false, err
	}

	worktreePath = filepath.Join(m.stateDir, "projects", parentKey, "worktrees", worktreeDirName(branch))

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", "", false, fmt.Errorf("create worktrees dir: %w", err)
	}

	if err := git.Run(repoDir, "worktree", "add", worktreePath, "-b", branch, defaultBranch); err != nil {
		return "", "", false, fmt.Errorf("git worktree add: %w", err)
	}

	return worktreePath, branch, true, nil
}

func (m *Manager) branchName(slug string) string {
	if m.cfg.Git.BranchPrefix != "" {
		return m.cfg.Git.BranchPrefix + "/" + slug
	}
	return slug
}

func (m *Manager) uniqueBranch(repoDir, branch string) (string, error) {
	exists, err := git.BranchExists(repoDir, branch)
	if err != nil {
		return "", err
	}
	if !exists {
		return branch, nil
	}
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", branch, i)
		exists, err := git.BranchExists(repoDir, candidate)
		if err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find unique branch name for %s", branch)
}

// CheckWorktreeDirty returns ErrWorktreeDirty if the worktree has uncommitted
// changes (modified, untracked, or staged files). Returns nil if clean.
func (m *Manager) CheckWorktreeDirty(worktreePath string) error {
	out, err := git.Output(worktreePath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if strings.TrimSpace(out) != "" {
		return fmt.Errorf("%w: use --force to delete anyway", ErrWorktreeDirty)
	}
	return nil
}

// RemoveWorktree removes a git worktree directory. The branch is intentionally
// preserved so the user can recover work.
//
// When force is false and the worktree contains uncommitted changes,
// ErrWorktreeDirty is returned. When force is true, the worktree is removed
// unconditionally.
func (m *Manager) RemoveWorktree(worktreePath string, force bool) error {
	repoDir, err := git.FindMainRepo(worktreePath)
	if err != nil {
		return fmt.Errorf("locate parent repo: %w", err)
	}

	args := []string{"worktree", "remove", worktreePath}
	if force {
		args = []string{"worktree", "remove", "--force", worktreePath}
	}

	if err := git.Run(repoDir, args...); err != nil {
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
