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

// IsManaged reports whether the given worktree path matches the layout used
// by argus-created worktrees: <stateDir>/projects/<parentKey>/worktrees/<name>.
// It determines the parent key by resolving the worktree's parent git
// repository and deriving the key from that path.
// External worktrees (user-provided paths outside this structure) return false
// and are never deleted on session cleanup.
func (m *Manager) IsManaged(worktreePath string) bool {
	mainRepo, err := mainRepoFromWorktree(worktreePath)
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
		if err := runGit("", "clone", src.RemoteURL, cloneDir); err != nil {
			os.RemoveAll(cloneDir)
			return "", fmt.Errorf("clone repo: %w", err)
		}
	} else if fetchOnly {
		if err := runGit(cloneDir, "fetch", "origin"); err != nil {
			return "", fmt.Errorf("fetch: %w", err)
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
// Returns the worktree path, git branch name, and whether the worktree was
// newly created (true) or an existing one was reused (false).
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string) (worktreePath, branch string, created bool, err error) {
	cloneDir, err := m.EnsureClone(src, false)
	if err != nil {
		return "", "", false, err
	}
	return m.createWorktree(cloneDir, src.ParentKey(), sessionName)
}

// worktreeEntry represents a parsed entry from git worktree list --porcelain.
type worktreeEntry struct {
	path   string // worktree path (may contain spaces)
	branch string // short branch name (e.g. "main"), empty for detached HEAD
}

// listWorktrees returns parsed worktree entries, excluding the first entry
// (main working tree). Uses --porcelain for reliable parsing of paths with
// spaces.
func listWorktrees(repoDir string) ([]worktreeEntry, error) {
	out, err := gitOutput(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}

	var entries []worktreeEntry
	var current worktreeEntry
	first := true

	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			if current.path != "" {
				if first {
					first = false
				} else {
					entries = append(entries, current)
				}
			}
			current = worktreeEntry{}
			continue
		}
		if strings.HasPrefix(line, "worktree ") {
			current.path = line[len("worktree "):]
		} else if strings.HasPrefix(line, "branch refs/heads/") {
			current.branch = line[len("branch refs/heads/"):]
		}
	}
	// Flush last entry (porcelain output may not end with blank line).
	if current.path != "" && !first {
		entries = append(entries, current)
	}
	return entries, nil
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

	defaultBranch, err := getDefaultBranch(repoDir)
	if err != nil {
		return "", "", false, err
	}

	worktreePath = filepath.Join(m.stateDir, "projects", parentKey, "worktrees", worktreeDirName(branch))

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", "", false, fmt.Errorf("create worktrees dir: %w", err)
	}

	if err := runGit(repoDir, "worktree", "add", worktreePath, "-b", branch, defaultBranch); err != nil {
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
