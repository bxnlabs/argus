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

// IsManagedPath reports whether the given path matches the managed worktree
// layout based solely on the path structure, without accessing the filesystem.
// This is useful when the worktree directory may no longer exist.
func (m *Manager) IsManagedPath(worktreePath string) bool {
	worktreesRoot := filepath.Join(m.stateDir, "projects") + string(filepath.Separator)
	if !strings.HasPrefix(worktreePath, worktreesRoot) {
		return false
	}
	// Expected: <stateDir>/projects/<parentKey>/worktrees/<name>
	rel := worktreePath[len(worktreesRoot):]
	parts := strings.Split(rel, string(filepath.Separator))
	return len(parts) == 3 && parts[1] == "worktrees"
}

// CreateForLocalRepo creates an isolated git worktree for a local git repo.
// gitRoot must be the absolute path to the repo root.
// Returns the worktree path, git branch name, whether the worktree was
// newly created (true) or an existing one was reused (false), and whether
// the branch was newly created by Argus (as opposed to an existing branch).
func (m *Manager) CreateForLocalRepo(gitRoot, sessionName string, branchOverride string) (worktreePath, branch string, worktreeCreated, branchCreated bool, err error) {
	src := &source.Source{LocalPath: gitRoot}
	return m.createWorktree(gitRoot, src.ParentKey(), sessionName, branchOverride)
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
			return "", fmt.Errorf("resolve default branch: %w", err)
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
// Returns the worktree path, git branch name, whether the worktree was
// newly created (true) or an existing one was reused (false), and whether
// the branch was newly created by Argus (as opposed to an existing branch).
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string, branchOverride string) (worktreePath, branch string, worktreeCreated, branchCreated bool, err error) {
	cloneDir, err := m.EnsureClone(src, false)
	if err != nil {
		return "", "", false, false, err
	}
	return m.createWorktree(cloneDir, src.ParentKey(), sessionName, branchOverride)
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

// ManagedWorktree is an Argus-managed linked worktree (living under
// <stateDir>/projects/<key>/worktrees/), as returned by ListManaged.
type ManagedWorktree struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// ListManaged returns the Argus-managed linked worktrees for the repo at
// repoDir. The main working tree is always excluded (via listWorktrees), and
// any linked worktree that is not under the managed
// <stateDir>/projects/.../worktrees/ layout is filtered out.
func (m *Manager) ListManaged(repoDir string) ([]ManagedWorktree, error) {
	entries, err := listWorktrees(repoDir)
	if err != nil {
		return nil, err
	}
	managed := make([]ManagedWorktree, 0, len(entries))
	for _, e := range entries {
		if m.IsManagedPath(e.path) {
			managed = append(managed, ManagedWorktree{Path: e.path, Branch: e.branch})
		}
	}
	return managed, nil
}

func (m *Manager) createWorktree(repoDir, parentKey, sessionName string, branchOverride string) (worktreePath, branch string, worktreeCreated, branchCreated bool, err error) {
	var baseBranch string
	if branchOverride != "" {
		baseBranch = branchOverride
	} else {
		slug := slugify(sessionName)
		baseBranch = m.branchName(slug)
	}

	// Check if a worktree already exists for this branch.
	existing, err := m.FindWorktree(repoDir, baseBranch)
	if err != nil {
		return "", "", false, false, err
	}
	if existing != "" {
		return existing, baseBranch, false, false, nil
	}

	// Check if the branch already exists locally.
	localBranchExists, err := git.BranchExists(repoDir, baseBranch)
	if err != nil {
		return "", "", false, false, fmt.Errorf("check branch exists: %w", err)
	}

	// If branch doesn't exist locally and we have a branch override,
	// try to find it on the remote and fetch it.
	remoteOnly := false
	if !localBranchExists && branchOverride != "" && git.HasRemote(repoDir) {
		remoteExists, err := git.RemoteTrackingBranchExists(repoDir, baseBranch)
		if err == nil && !remoteExists {
			// Tracking ref not found — try a targeted fetch
			if fetchErr := git.FetchBranch(repoDir, baseBranch); fetchErr == nil {
				remoteExists, _ = git.RemoteTrackingBranchExists(repoDir, baseBranch)
			}
		}
		if remoteExists {
			remoteOnly = true
		}
	}

	branchExists := localBranchExists || remoteOnly

	worktreePath = filepath.Join(m.stateDir, "projects", parentKey, "worktrees", worktreeDirName(baseBranch))

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", "", false, false, fmt.Errorf("create worktrees dir: %w", err)
	}

	if branchExists {
		mainBranch, err := mainWorktreeBranch(repoDir)
		if err != nil {
			return "", "", false, false, err
		}
		if mainBranch == baseBranch {
			return "", "", false, false, fmt.Errorf(
				"branch %q is currently checked out in the main working tree at %s; "+
					"switch to a different branch there before starting this session",
				baseBranch, repoDir,
			)
		}
		if remoteOnly {
			// Branch exists only on the remote — create a local tracking branch.
			// Plain "git worktree add <path> <branch>" can fail in repos without
			// auto-creation of local tracking branches (e.g. single-branch clones).
			if err := git.Run(repoDir, "worktree", "add", "-b", baseBranch, worktreePath, "origin/"+baseBranch); err != nil {
				return "", "", false, false, fmt.Errorf("git worktree add (remote branch): %w", err)
			}
			// Argus created the local branch (tracking the remote). Clean it up
			// on session delete — the upstream branch on origin is unaffected.
			return worktreePath, baseBranch, true, true, nil
		}
		if err := git.Run(repoDir, "worktree", "add", worktreePath, baseBranch); err != nil {
			return "", "", false, false, fmt.Errorf("git worktree add: %w", err)
		}
		// Worktree created, but branch already existed locally — Argus did not create it.
		return worktreePath, baseBranch, true, false, nil
	}

	defaultBranch, err := git.DefaultBranch(repoDir)
	if err != nil {
		return "", "", false, false, fmt.Errorf("resolve default branch: %w", err)
	}

	if err := git.Run(repoDir, "worktree", "add", worktreePath, "-b", baseBranch, defaultBranch); err != nil {
		return "", "", false, false, fmt.Errorf("git worktree add: %w", err)
	}

	// Both worktree and branch are newly created by Argus.
	return worktreePath, baseBranch, true, true, nil
}

func (m *Manager) branchName(slug string) string {
	if m.cfg.Git.BranchPrefix != "" {
		return m.cfg.Git.BranchPrefix + "/" + slug
	}
	return slug
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

// DeleteBranch force-deletes a local branch (git branch -D).
func (m *Manager) DeleteBranch(repoDir, branch string) error {
	if err := git.Run(repoDir, "branch", "-D", branch); err != nil {
		return fmt.Errorf("git branch -D %s: %w", branch, err)
	}
	return nil
}
