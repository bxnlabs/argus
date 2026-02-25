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

// CreateForRemoteRepo clones (or fetches) the remote repo and creates a worktree.
// Returns the worktree path and the git branch name.
func (m *Manager) CreateForRemoteRepo(src *source.Source, sessionName string) (worktreePath, branch string, err error) {
	cloneDir := filepath.Join(m.stateDir, "projects", src.ParentKey(), "gitrepo")

	_, statErr := os.Stat(cloneDir)
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return "", "", fmt.Errorf("stat clone dir: %w", statErr)
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cloneDir), 0755); err != nil {
			return "", "", fmt.Errorf("create project dir: %w", err)
		}
		if err := runGit("", "clone", src.RemoteURL, cloneDir); err != nil {
			os.RemoveAll(cloneDir)
			return "", "", fmt.Errorf("clone repo: %w", err)
		}
	} else {
		defaultBranch, err := getDefaultBranch(cloneDir)
		if err != nil {
			return "", "", err
		}
		if err := runGit(cloneDir, "fetch", "origin"); err != nil {
			return "", "", fmt.Errorf("fetch: %w", err)
		}
		if err := runGit(cloneDir, "checkout", defaultBranch); err != nil {
			return "", "", fmt.Errorf("checkout default branch: %w", err)
		}
		if err := runGit(cloneDir, "reset", "--hard", "origin/"+defaultBranch); err != nil {
			return "", "", fmt.Errorf("reset to origin: %w", err)
		}
	}

	return m.createWorktree(cloneDir, src.ParentKey(), sessionName)
}

func (m *Manager) createWorktree(repoDir, parentKey, sessionName string) (worktreePath, branch string, err error) {
	slug := slugify(sessionName)
	baseBranch := m.branchName(slug)

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
	if m.cfg.BranchPrefix != "" {
		return m.cfg.BranchPrefix + "/" + slug
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
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
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
