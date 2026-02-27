package session

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/provider"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/source"
	"github.com/bxnlabs/argus/internal/worktree"
)

// ErrNotFound is returned when a session ID does not exist in the database.
var ErrNotFound = db.ErrNotFound

// ErrInvalidInput is returned when a create/update request contains
// user-fixable validation errors (bad source, unknown agent type, etc.).
var ErrInvalidInput = errors.New("invalid input")

// Manager handles session lifecycle (create, delete, rename, etc.).
type Manager struct {
	db      *db.DB
	wt      *worktree.Manager
	mu      sync.Mutex
	sessLks map[string]*sync.Mutex
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
func NewManager(database *db.DB, wt *worktree.Manager) *Manager {
	return &Manager{db: database, wt: wt}
}

// CreateOptions are the options for creating a new session.
type CreateOptions struct {
	Name            string  `json:"name"`
	AgentType       string  `json:"agent_type"`
	Source          string  `json:"source"`
	Model           *string `json:"model,omitempty"`
	SystemPrompt    *string `json:"system_prompt,omitempty"`
	AutoApprove     bool    `json:"auto_approve"`
	ResumeSessionID string  `json:"resume_session_id,omitempty"`
}

// Create creates a new session: generates ID, builds CLI command, spawns tmux, inserts DB.
func (m *Manager) Create(opts CreateOptions) (*db.Session, error) {
	if !provider.IsValid(provider.AgentType(opts.AgentType)) {
		return nil, fmt.Errorf("%w: invalid agent type: %s", ErrInvalidInput, opts.AgentType)
	}

	// Generate session ID (UUID format for tmux name)
	sessionID := shared.GenerateID("sess")
	tmuxName := fmt.Sprintf("%s-%s", opts.AgentType, sessionID)

	// Resolve source → working directory (and optional worktree branch).
	// cleanup removes the git worktree if a later step fails; it is a no-op
	// for non-worktree sessions.
	cwd, worktreeBranch, cleanup, err := m.resolveSourceToCWD(opts.Source, opts.Name, provider.AgentType(opts.AgentType))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}

	// If anything below fails before the DB commit, clean up the worktree.
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	// Build the agent command
	agentCmd, err := provider.BuildCommand(provider.AgentType(opts.AgentType), provider.BuildCommandOptions{
		AutoApprove: opts.AutoApprove,
		SessionID:   opts.ResumeSessionID,
		Model:       ptrStr(opts.Model),
	})
	if err != nil {
		return nil, fmt.Errorf("build command: %w", err)
	}

	// For shell sessions (no command), start tmux with default shell
	// For agent sessions, wrap with init script
	var tmuxCmd string
	if agentCmd != "" {
		scriptPath, err := WriteInitScript(sessionID, agentCmd)
		if err != nil {
			return nil, fmt.Errorf("write init script: %w", err)
		}
		tmuxCmd = "bash " + scriptPath
	}

	// Spawn tmux session and apply standard styling
	if err := NewSession(tmuxName, cwd, tmuxCmd); err != nil {
		return nil, fmt.Errorf("spawn tmux: %w", err)
	}
	ConfigureSession(tmuxName)

	// Insert into database
	var providerSessionID *string
	if opts.ResumeSessionID != "" {
		providerSessionID = &opts.ResumeSessionID
	}
	session := &db.Session{
		ID:                sessionID,
		Name:              opts.Name,
		TmuxName:          tmuxName,
		WorkingDirectory:  cwd,
		AgentType:         opts.AgentType,
		Model:             opts.Model,
		SystemPrompt:      opts.SystemPrompt,
		AutoApprove:       opts.AutoApprove,
		ProviderSessionID: providerSessionID,
		WorktreeBranch:    worktreeBranch,
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
// worktree if a subsequent step fails. For non-worktree sessions, cleanup is
// a no-op. If source is empty, defaults to home dir.
func (m *Manager) resolveSourceToCWD(src, sessionName string, agentType provider.AgentType) (cwd string, worktreeBranch *string, cleanup func(), err error) {
	noop := func() {}

	if src == "" {
		home, err := shared.ExpandPath("~")
		if err != nil {
			return "", nil, noop, fmt.Errorf("expand home directory: %w", err)
		}
		return home, nil, noop, nil
	}

	resolved, err := source.Resolve(src)
	if err != nil {
		return "", nil, noop, err
	}

	if resolved.IsRemote() {
		if agentType == provider.AgentShell {
			// Shell sessions clone but don't create a worktree.
			cloneDir, err := m.wt.EnsureClone(resolved)
			if err != nil {
				return "", nil, noop, err
			}
			return cloneDir, nil, noop, nil
		}
		wtPath, branch, err := m.wt.CreateForRemoteRepo(resolved, sessionName)
		if err != nil {
			return "", nil, noop, err
		}
		return wtPath, &branch, func() { m.wt.Cleanup(wtPath) }, nil
	}

	// Local path: check if it's inside a git repo.
	gitRoot, err := findGitRoot(resolved.LocalPath)
	if err != nil {
		// Not a git repo — use the path directly.
		return resolved.LocalPath, nil, noop, nil
	}

	if agentType == provider.AgentShell {
		// Shell sessions use the local path directly, no worktree.
		return resolved.LocalPath, nil, noop, nil
	}

	wtPath, branch, err := m.wt.CreateForLocalRepo(gitRoot, sessionName)
	if err != nil {
		return "", nil, noop, err
	}
	return wtPath, &branch, func() { m.wt.Cleanup(wtPath) }, nil
}

// findGitRoot returns the git root for the given directory, or an error if
// the directory is not inside a git repository.
func findGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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

	// Remove worktree FIRST so a dirty-check failure leaves the session
	// fully intact (tmux still alive, DB row untouched).
	if session.WorktreeBranch != nil {
		if _, statErr := os.Stat(session.WorkingDirectory); os.IsNotExist(statErr) {
			// Worktree was removed externally; skip git cleanup and
			// proceed with deletion. The orphaned git worktree ref
			// will be cleaned up by "git worktree prune".
		} else if err := m.wt.RemoveWorktree(session.WorkingDirectory, force); err != nil {
			return err
		}
	}

	// Kill tmux (ignore error if already dead)
	if HasSession(session.TmuxName) {
		KillSession(session.TmuxName)
	}

	return m.db.DeleteSession(id)
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

	return m.db.UpdateSession(id, db.SessionUpdate{
		Name: &newName,
	})
}

// Get returns a session by ID.
func (m *Manager) Get(id string) (*db.Session, error) {
	return m.db.GetSession(id)
}

// List returns all sessions.
func (m *Manager) List() ([]*db.Session, error) {
	return m.db.ListSessions()
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

	agentCmd, err := provider.BuildCommand(provider.AgentType(session.AgentType), provider.BuildCommandOptions{
		AutoApprove: session.AutoApprove,
		SessionID:   ptrStr(session.ProviderSessionID),
		Model:       ptrStr(session.Model),
	})
	if err != nil {
		return "", fmt.Errorf("build command: %w", err)
	}

	var tmuxCmd string
	if agentCmd != "" {
		scriptPath, err := WriteInitScript(session.ID, agentCmd)
		if err != nil {
			return "", fmt.Errorf("write init script: %w", err)
		}
		tmuxCmd = "bash " + scriptPath
	}

	if err := NewSession(tmuxName, cwd, tmuxCmd); err != nil {
		return "", fmt.Errorf("spawn tmux: %w", err)
	}
	ConfigureSession(tmuxName)

	return tmuxName, nil
}

// Update updates session fields.
func (m *Manager) Update(id string, u db.SessionUpdate) error {
	return m.db.UpdateSession(id, u)
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
