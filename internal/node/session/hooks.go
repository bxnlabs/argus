package session

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/source"
)

// Hook file name constants.
const (
	HookPreCreate        = "pre_create.sh"
	HookPostCreate       = "post_create.sh"
	HookOnCreateWorktree = "on_create_worktree.sh"
	HookPreDestroy       = "pre_destroy.sh"
)

var profileNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ValidateProfileName checks that a profile name is safe for use as a
// directory name. Empty names are rejected; use "" to mean "no profile".
func ValidateProfileName(name string) error {
	if name == "" || !profileNameRe.MatchString(name) {
		return fmt.Errorf("invalid profile name %q: must match [a-zA-Z0-9_-]+", name)
	}
	return nil
}

// ProjectKeyForSession derives the project key from session state.
// Worktree-backed sessions use git_parent_dir; others use working_directory.
func ProjectKeyForSession(sess *db.Session) string {
	if sess.GitParentDir != nil {
		return source.ParentKeyFromPath(*sess.GitParentDir)
	}
	return source.ParentKeyFromPath(sess.WorkingDirectory)
}

// HookEnv holds context variables passed to subprocess hooks.
type HookEnv struct {
	SessionID    string
	WorkingDir   string
	ProviderType string
	WorktreePath string
	Profile      string
}

// HookRunner resolves and executes lifecycle hooks.
type HookRunner struct {
	stateDir string
	Timeout  time.Duration
}

// NewHookRunner creates a HookRunner rooted at the given state directory.
func NewHookRunner(stateDir string) *HookRunner {
	return &HookRunner{stateDir: stateDir, Timeout: 30 * time.Second}
}

// ResolveHookPaths returns hook script paths in setup order (profile then project).
// Only existing, executable files are returned. Missing hooks are silently skipped.
func (hr *HookRunner) ResolveHookPaths(hookName, profileName, projectKey string) []string {
	return hr.resolveHooks(hookName, profileName, projectKey, false, true)
}

// ResolveHookPathsTeardown returns hook script paths in teardown order
// (project first, then profile — LIFO).
func (hr *HookRunner) ResolveHookPathsTeardown(hookName, profileName, projectKey string) []string {
	return hr.resolveHooks(hookName, profileName, projectKey, true, true)
}

// ResolvePostCreateHookPaths returns post_create hook paths in setup order.
// Unlike other hooks, post_create does not require the executable bit.
func (hr *HookRunner) ResolvePostCreateHookPaths(profileName, projectKey string) []string {
	return hr.resolveHooks(HookPostCreate, profileName, projectKey, false, false)
}

func (hr *HookRunner) resolveHooks(hookName, profileName, projectKey string, teardown, requireExec bool) []string {
	// Resolve profile name: explicit or default.
	if profileName == "" {
		profileName = "default"
	}

	profilePath := filepath.Join(hr.stateDir, "profiles", profileName, "hooks", hookName)
	projectPath := filepath.Join(hr.stateDir, "projects", projectKey, "hooks", hookName)

	var setupOrder []string
	if hr.hookExists(profilePath, requireExec) {
		setupOrder = append(setupOrder, profilePath)
	}
	if hr.hookExists(projectPath, requireExec) {
		setupOrder = append(setupOrder, projectPath)
	}

	if teardown {
		// Reverse for LIFO.
		for i, j := 0, len(setupOrder)-1; i < j; i, j = i+1, j-1 {
			setupOrder[i], setupOrder[j] = setupOrder[j], setupOrder[i]
		}
	}
	return setupOrder
}

func (hr *HookRunner) hookExists(path string, requireExec bool) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if requireExec && info.Mode()&0111 == 0 {
		return false
	}
	return true
}

// RunHook executes a subprocess hook with timeout and environment variables.
// Returns an error if the hook exits non-zero or times out.
func (hr *HookRunner) RunHook(hookPath string, env HookEnv) error {
	ctx, cancel := context.WithTimeout(context.Background(), hr.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, hookPath)
	cmd.Dir = env.WorkingDir
	cmd.Env = append(os.Environ(),
		"ARGUS_SESSION_ID="+env.SessionID,
		"ARGUS_WORKING_DIR="+env.WorkingDir,
		"ARGUS_PROVIDER_TYPE="+env.ProviderType,
		"ARGUS_WORKTREE_PATH="+env.WorktreePath,
		"ARGUS_PROFILE="+env.Profile,
	)

	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("hook %s timed out after %s", filepath.Base(hookPath), hr.Timeout)
	}
	if err != nil {
		return fmt.Errorf("hook %s failed: %w\n%s", filepath.Base(hookPath), err, string(out))
	}
	return nil
}

// RunHooksBestEffort runs hooks and logs errors but does not return them.
func (hr *HookRunner) RunHooksBestEffort(paths []string, env HookEnv) {
	for _, p := range paths {
		if err := hr.RunHook(p, env); err != nil {
			log.Printf("hook %s: %v", filepath.Base(p), err)
		}
	}
}
