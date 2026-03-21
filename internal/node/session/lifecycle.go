package session

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/provider"
	"github.com/bxnlabs/argus/internal/git"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
)

// ErrNotFound is returned when a session ID does not exist in the database.
var ErrNotFound = db.ErrNotFound

// ErrInvalidInput is returned when a create/update request contains
// user-fixable validation errors (bad source, unknown agent type, etc.).
var ErrInvalidInput = errors.New("invalid input")

// Manager handles session lifecycle (create, delete, rename, etc.).
type Manager struct {
	db       *db.DB
	wt       *worktree.Manager
	stateDir string
	hooks    *HookRunner
	mu       sync.Mutex
	sessLks  map[string]*sync.Mutex
}

// sessionLock returns a per-session mutex, creating it if needed.
// This serializes EnsureSession and Delete for the same session ID,
// preventing races where a delete removes the DB row mid-ensure.
func (m *Manager) sessionLock(id string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessLks == nil {
		m.sessLks = make(map[string]*sync.Mutex)
	}
	l, ok := m.sessLks[id]
	if !ok {
		l = &sync.Mutex{}
		m.sessLks[id] = l
	}
	return l
}

// NewManager creates a new session manager.
func NewManager(database *db.DB, wt *worktree.Manager, stateDir string) *Manager {
	return &Manager{db: database, wt: wt, stateDir: stateDir, hooks: NewHookRunner(stateDir)}
}

// CreateOptions are the options for creating a new session.
type CreateOptions struct {
	Name            string  `json:"name"`
	ProviderType    string  `json:"provider_type"`
	Source          string  `json:"source"`
	Model           *string `json:"model,omitempty"`
	SystemPrompt    *string `json:"system_prompt,omitempty"`
	AutoApprove     bool    `json:"auto_approve"`
	ResumeSessionID string  `json:"resume_session_id,omitempty"`
	Profile         *string `json:"profile,omitempty"`
}

// Create creates a new session: generates ID, builds CLI command, spawns tmux, inserts DB.
func (m *Manager) Create(opts CreateOptions) (*db.Session, error) {
	if !provider.IsValid(provider.ProviderType(opts.ProviderType)) {
		return nil, fmt.Errorf("%w: invalid provider type: %s", ErrInvalidInput, opts.ProviderType)
	}

	// Resolve profile
	resolvedProfile, err := m.resolveProfile(opts.Profile)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	// Generate session ID (UUID format for tmux name)
	sessionID := shared.GenerateID("sess")
	tmuxName := fmt.Sprintf("%s-%s", opts.ProviderType, sessionID)

	// Resolve source → working directory (and optional worktree branch).
	// cleanup removes the git worktree if a later step fails; it is a no-op
	// for non-worktree sessions or reused worktrees.
	cwd, worktreeBranch, worktreeCreated, cleanup, err := m.resolveSourceToCWD(opts.Source, opts.Name, provider.ProviderType(opts.ProviderType))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	// Resolve the parent (main) repository directory for worktree sessions.
	// This is stored once at creation time so the API layer doesn't need to
	// shell out to git on every request.
	var gitParentDir *string
	if worktreeBranch != nil {
		if dir, err := git.FindMainRepo(cwd); err == nil {
			gitParentDir = &dir
		}
	}

	// Resolve git remote origin URL for any git-backed session.
	// Use git_parent_dir (main repo root) if available, otherwise cwd.
	var gitRemoteURL *string
	{
		remoteDir := cwd
		if gitParentDir != nil {
			remoteDir = *gitParentDir
		}
		if rawURL, err := git.RemoteURL(remoteDir); err == nil {
			sanitized := git.SanitizeRemoteURL(rawURL)
			gitRemoteURL = &sanitized
		}
	}

	// Derive project key for hook resolution
	projectKey := ""
	if gitParentDir != nil {
		projectKey = source.ParentKeyFromPath(*gitParentDir)
	} else {
		projectKey = source.ParentKeyFromPath(cwd)
	}

	// If anything below fails before the DB commit, clean up the worktree.
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	// Run pre_create hooks (blocking, abort on failure)
	hookEnv := HookEnv{
		SessionID: sessionID, WorkingDir: cwd,
		ProviderType: opts.ProviderType, Profile: resolvedProfile,
	}
	preCreatePaths := m.hooks.ResolveHookPaths(HookPreCreate, resolvedProfile, projectKey)
	for _, p := range preCreatePaths {
		if err := m.hooks.RunHook(p, hookEnv); err != nil {
			return nil, fmt.Errorf("pre_create hook: %w", err)
		}
	}

	// Run on_create_worktree hooks if worktree was newly created
	if worktreeCreated {
		hookEnv.WorktreePath = cwd
		wtPaths := m.hooks.ResolveHookPaths(HookOnCreateWorktree, resolvedProfile, projectKey)
		for _, p := range wtPaths {
			if err := m.hooks.RunHook(p, hookEnv); err != nil {
				return nil, fmt.Errorf("on_create_worktree hook: %w", err)
			}
		}
	}

	// Build the agent command
	agentCmd, err := provider.BuildCommand(provider.ProviderType(opts.ProviderType), provider.BuildCommandOptions{
		AutoApprove: opts.AutoApprove,
		SessionID:   opts.ResumeSessionID,
		Model:       ptrStr(opts.Model),
	})
	if err != nil {
		return nil, fmt.Errorf("build command: %w", err)
	}

	// Resolve post_create hooks for init script sourcing
	postCreatePaths := m.hooks.ResolvePostCreateHookPaths(resolvedProfile, projectKey)

	var tmuxCmd string
	if agentCmd != "" {
		pattern := provider.GetSessionIDPattern(provider.ProviderType(opts.ProviderType))
		scriptPath, err := WriteInitScript(sessionID, agentCmd, pattern, postCreatePaths)
		if err != nil {
			return nil, fmt.Errorf("write init script: %w", err)
		}
		tmuxCmd = "bash " + scriptPath
	} else if len(postCreatePaths) > 0 {
		// Shell session with hooks — need init wrapper to source them
		scriptPath, err := WriteShellInitScript(sessionID, postCreatePaths)
		if err != nil {
			return nil, fmt.Errorf("write shell init script: %w", err)
		}
		if scriptPath != "" {
			tmuxCmd = "bash " + scriptPath
		}
	}

	// Spawn tmux session and apply standard styling
	if err := NewSession(tmuxName, cwd, tmuxCmd); err != nil {
		return nil, fmt.Errorf("spawn tmux: %w", err)
	}
	configDir := cwd
	if gitParentDir != nil {
		configDir = *gitParentDir
	}
	configBranch := ""
	if worktreeBranch != nil {
		configBranch = *worktreeBranch
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("resolve home dir: %v", err)
	}
	ConfigureSession(tmuxName, sessionID, configDir, configBranch, home)

	// Insert into database
	var providerSessionID *string
	if opts.ResumeSessionID != "" {
		providerSessionID = &opts.ResumeSessionID
	}
	// Persist resolved profile
	var profilePtr *string
	if resolvedProfile != "" {
		profilePtr = &resolvedProfile
	}
	session := &db.Session{
		ID:                sessionID,
		Name:              opts.Name,
		TmuxName:          tmuxName,
		WorkingDirectory:  cwd,
		ProviderType:      opts.ProviderType,
		Model:             opts.Model,
		SystemPrompt:      opts.SystemPrompt,
		AutoApprove:       opts.AutoApprove,
		ProviderSessionID: providerSessionID,
		WorktreeBranch:    worktreeBranch,
		GitParentDir:      gitParentDir,
		GitRemoteURL:      gitRemoteURL,
		Profile:           profilePtr,
	}

	if err := m.db.CreateSession(session); err != nil {
		// Clean up tmux on DB error; worktree cleanup handled by defer.
		KillSession(tmuxName)
		return nil, fmt.Errorf("insert session: %w", err)
	}
	success = true

	// Re-fetch to get defaults (created_at, updated_at, status)
	created, err := m.db.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// resolveSourceToCWD resolves a source string to a working directory path.
// If the source is a git repo, it creates an isolated worktree and returns
// the worktree path, branch name, and a cleanup function that removes the
// worktree if a subsequent step fails. For non-worktree or reused-worktree
// sessions, cleanup is a no-op. If source is empty, defaults to home dir.
func (m *Manager) resolveSourceToCWD(src, sessionName string, agentType provider.ProviderType) (cwd string, worktreeBranch *string, worktreeCreated bool, cleanup func(), err error) {
	noop := func() {}

	if src == "" {
		home, err := shared.ExpandPath("~")
		if err != nil {
			return "", nil, false, noop, fmt.Errorf("expand home directory: %w", err)
		}
		return home, nil, false, noop, nil
	}

	resolved, err := source.Resolve(src)
	if err != nil {
		return "", nil, false, noop, err
	}

	if resolved.IsRemote() {
		if agentType == provider.ProviderShell {
			// Shell sessions clone but don't create a worktree.
			// Use fetchOnly=true to avoid resetting uncommitted work
			// from other shell sessions sharing the same clone dir.
			cloneDir, err := m.wt.EnsureClone(resolved, true)
			if err != nil {
				return "", nil, false, noop, err
			}
			return cloneDir, nil, false, noop, nil
		}
		wtPath, branch, created, err := m.wt.CreateForRemoteRepo(resolved, sessionName)
		if err != nil {
			return "", nil, false, noop, err
		}
		cleanup := noop
		if created {
			cleanup = func() { m.wt.Cleanup(wtPath) }
		}
		return wtPath, &branch, created, cleanup, nil
	}

	// Local path: check if it's inside a git repo.
	gitRoot, err := git.FindMainRepo(resolved.LocalPath)
	if err != nil {
		// Not a git repo — use the path directly.
		return resolved.LocalPath, nil, false, noop, nil
	}

	if agentType == provider.ProviderShell {
		// Shell sessions use the local path directly, no worktree.
		return resolved.LocalPath, nil, false, noop, nil
	}

	// Check if the resolved path is already a worktree — reuse it.
	existingBranch, err := m.wt.FindWorktreeByPath(resolved.LocalPath)
	if err == nil && existingBranch != "" {
		return resolved.LocalPath, &existingBranch, false, noop, nil
	}

	wtPath, branch, created, err := m.wt.CreateForLocalRepo(gitRoot, sessionName)
	if err != nil {
		return "", nil, false, noop, err
	}
	cleanup2 := noop
	if created {
		cleanup2 = func() { m.wt.Cleanup(wtPath) }
	}
	return wtPath, &branch, created, cleanup2, nil
}

// Delete kills the tmux session and removes from DB. For worktree-backed
// sessions, the worktree is removed but the branch is preserved. If the
// worktree has uncommitted changes and force is false, the delete is refused.
func (m *Manager) Delete(id string, force bool) error {
	l := m.sessionLock(id)
	l.Lock()
	defer l.Unlock()

	session, err := m.db.GetSession(id)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("%w: %s", db.ErrNotFound, id)
	}

	// Preflight: check for dirty worktree BEFORE any side effects.
	// This ensures that a failed delete (dirty worktree, force=false)
	// leaves the session fully intact (tmux alive, DB row untouched).
	needsWorktreeRemoval := false
	if session.WorktreeBranch != nil && m.wt.IsManaged(session.WorkingDirectory) {
		others, err := m.db.CountSessionsByWorkingDir(id, session.WorkingDirectory)
		if err != nil {
			return fmt.Errorf("check shared worktree: %w", err)
		}
		if others == 0 {
			if _, statErr := os.Stat(session.WorkingDirectory); os.IsNotExist(statErr) {
				// Worktree was removed externally; skip git cleanup.
			} else {
				if !force {
					if err := m.wt.CheckWorktreeDirty(session.WorkingDirectory); err != nil {
						return err
					}
				}
				needsWorktreeRemoval = true
			}
		}
	}

	projectKey := ProjectKeyForSession(session)
	profileName := ptrStr(session.Profile)

	hookEnv := HookEnv{
		SessionID:    session.ID,
		WorkingDir:   session.WorkingDirectory,
		ProviderType: session.ProviderType,
		Profile:      profileName,
	}

	// pre_destroy: LIFO order (project first, then profile), best-effort
	preDestroyPaths := m.hooks.ResolveHookPathsTeardown(HookPreDestroy, profileName, projectKey)
	m.hooks.RunHooksBestEffort(preDestroyPaths, hookEnv)

	// Kill tmux (ignore error if already dead)
	if HasSession(session.TmuxName) {
		KillSession(session.TmuxName)
	}

	// Remove worktree. The preflight dirty check already validated the
	// user's intent (worktree was clean or force=true), so force-remove
	// here to avoid TOCTOU issues (e.g. pre_destroy hooks writing files).
	if needsWorktreeRemoval {
		if err := m.wt.RemoveWorktree(session.WorkingDirectory, true); err != nil {
			return err
		}
	}

	// Delete DB record
	if err := m.db.DeleteSession(id); err != nil {
		return err
	}

	return nil
}

// Rename updates the display name of a session. The underlying tmux session
// name is immutable (based on session ID) to preserve WebSocket connections
// and guarantee uniqueness.
func (m *Manager) Rename(id, newName string) error {
	session, err := m.db.GetSession(id)
	if err != nil {
		return err
	}
	if session == nil {
		return fmt.Errorf("%w: %s", db.ErrNotFound, id)
	}

	_, err = m.db.UpdateSession(id, db.SessionUpdate{
		Name: &newName,
	})
	return err
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*db.Session, error) {
	return m.db.GetSession(id)
}

// List returns all sessions.
func (m *Manager) List(ctx context.Context) ([]*db.Session, error) {
	return m.db.ListSessions(ctx)
}

// ListProfiles returns the names of all profile directories that contain
// a hooks/ subdirectory under {stateDir}/profiles/.
func (m *Manager) ListProfiles() ([]string, error) {
	profilesDir := filepath.Join(m.stateDir, "profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("read profiles dir: %w", err)
	}
	names := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if ValidateProfileName(name) != nil {
			continue
		}
		hooksDir := filepath.Join(profilesDir, name, "hooks")
		info, statErr := os.Stat(hooksDir)
		if statErr != nil {
			if !os.IsNotExist(statErr) {
				log.Printf("ListProfiles: stat %s: %v", hooksDir, statErr)
			}
			continue
		}
		if info.IsDir() {
			names = append(names, name)
		}
	}
	return names, nil
}

// getSession looks up a session by ID and checks whether its tmux
// process is still alive. Returns ErrNotFound if the session doesn't exist
// in the database. The alive flag indicates whether the tmux session is running.
func (m *Manager) getSession(id string) (sess *db.Session, alive bool, err error) {
	sess, err = m.db.GetSession(id)
	if err != nil {
		return nil, false, fmt.Errorf("lookup session: %w", err)
	}
	if sess == nil {
		return nil, false, fmt.Errorf("%w: %s", db.ErrNotFound, id)
	}
	return sess, HasSession(sess.TmuxName), nil
}

// EnsureSession guarantees the tmux session for the given session ID is running.
// If the session is already alive, this is a no-op. If it was killed, it is
// recreated from the DB record (agent type, model, auto-approve, resume ID).
// Returns the tmux session name, or an error if the session doesn't exist in the DB.
func (m *Manager) EnsureSession(id string) (string, error) {
	// Fast path: if tmux is already alive, return without locking.
	session, alive, err := m.getSession(id)
	if err != nil {
		return "", err
	}
	if alive {
		return session.TmuxName, nil
	}

	// Slow path: acquire per-session lock and recreate.
	l := m.sessionLock(id)
	l.Lock()
	defer l.Unlock()

	// Re-check — session may have been deleted or recreated while we waited.
	session, alive, err = m.getSession(id)
	if err != nil {
		return "", err
	}
	if alive {
		return session.TmuxName, nil
	}

	tmuxName := session.TmuxName

	cwd, err := shared.ExpandPath(session.WorkingDirectory)
	if err != nil {
		return "", fmt.Errorf("expand path: %w", err)
	}
	if cwd == "" {
		home, err := shared.ExpandPath("~")
		if err != nil {
			return "", fmt.Errorf("expand home directory: %w", err)
		}
		cwd = home
	}
	// Verify the working directory still exists on disk. For worktree-backed
	// sessions the directory may have been removed externally; without this
	// check tmux would silently fall back to the home directory.
	if _, statErr := os.Stat(cwd); statErr != nil {
		return "", fmt.Errorf("working directory no longer exists: %s", cwd)
	}

	agentCmd, err := provider.BuildCommand(provider.ProviderType(session.ProviderType), provider.BuildCommandOptions{
		AutoApprove: session.AutoApprove,
		SessionID:   ptrStr(session.ProviderSessionID),
		Model:       ptrStr(session.Model),
	})
	if err != nil {
		return "", fmt.Errorf("build command: %w", err)
	}

	// Resolve post_create hooks (empty profile falls back to "default" in HookRunner)
	profileName := ptrStr(session.Profile)
	projectKey := ProjectKeyForSession(session)
	postCreatePaths := m.hooks.ResolvePostCreateHookPaths(profileName, projectKey)

	var tmuxCmd string
	if agentCmd != "" {
		pattern := provider.GetSessionIDPattern(provider.ProviderType(session.ProviderType))
		scriptPath, err := WriteInitScript(session.ID, agentCmd, pattern, postCreatePaths)
		if err != nil {
			return "", fmt.Errorf("write init script: %w", err)
		}
		tmuxCmd = "bash " + scriptPath
	} else if len(postCreatePaths) > 0 {
		scriptPath, err := WriteShellInitScript(session.ID, postCreatePaths)
		if err != nil {
			return "", fmt.Errorf("write shell init script: %w", err)
		}
		if scriptPath != "" {
			tmuxCmd = "bash " + scriptPath
		}
	}

	if err := NewSession(tmuxName, cwd, tmuxCmd); err != nil {
		return "", fmt.Errorf("spawn tmux: %w", err)
	}
	configDir := session.WorkingDirectory
	if session.GitParentDir != nil {
		configDir = *session.GitParentDir
	}
	configBranch := ""
	if session.WorktreeBranch != nil {
		configBranch = *session.WorktreeBranch
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("resolve home dir: %v", err)
	}
	ConfigureSession(tmuxName, session.ID, configDir, configBranch, home)

	return tmuxName, nil
}

// Update updates session fields and returns the updated session.
func (m *Manager) Update(id string, u db.SessionUpdate) (*db.Session, error) {
	return m.db.UpdateSession(id, u)
}

// TouchSession sets updated_at to the given Unix timestamp to reflect
// the most recent tmux activity for the session.
func (m *Manager) TouchSession(ctx context.Context, id string, unixTS int64) error {
	return m.db.TouchSession(ctx, id, unixTS)
}

// BackfillGitParentDir populates git_parent_dir for existing worktree
// sessions that were created before the column was added. This is
// best-effort: sessions whose working directories no longer exist or
// aren't inside a git repo are silently skipped.
func (m *Manager) BackfillGitParentDir() {
	sessions, err := m.db.ListSessionsForBackfill()
	if err != nil {
		log.Printf("backfill git_parent_dir: list sessions: %v", err)
		return
	}
	for _, s := range sessions {
		if dir, err := git.FindMainRepo(s.WorkingDirectory); err == nil {
			if err := m.db.SetGitParentDir(s.ID, dir); err != nil {
				log.Printf("backfill git_parent_dir: set %s: %v", s.ID, err)
			}
		}
	}
}

// resolveProfile validates and resolves the profile name.
// If profileOpt is non-nil, it validates the name and checks the profile dir exists.
// If profileOpt is nil, returns "" — the HookRunner will fall back to "default"
// at resolution time if the default profile directory exists.
// Only explicitly specified profiles are stored in the DB.
func (m *Manager) resolveProfile(profileOpt *string) (string, error) {
	if profileOpt == nil {
		return "", nil
	}
	name := *profileOpt
	if err := ValidateProfileName(name); err != nil {
		return "", err
	}
	dir := filepath.Join(m.stateDir, "profiles", name, "hooks")
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("profile %q not found", name)
		}
		return "", fmt.Errorf("check profile %q: %w", name, err)
	}
	return name, nil
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
