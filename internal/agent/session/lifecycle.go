package session

import (
	"fmt"
	"sync"

	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/provider"
	"github.com/bxnlabs/argus/internal/shared"
)

// ErrNotFound is returned when a session ID does not exist in the database.
var ErrNotFound = db.ErrNotFound

// Manager handles session lifecycle (create, delete, rename, etc.).
type Manager struct {
	db      *db.DB
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
func NewManager(database *db.DB) *Manager {
	return &Manager{db: database}
}

// CreateOptions are the options for creating a new session.
type CreateOptions struct {
	Name             string  `json:"name"`
	AgentType        string  `json:"agent_type"`
	WorkingDirectory string  `json:"working_directory"`
	Model            *string `json:"model,omitempty"`
	SystemPrompt     *string `json:"system_prompt,omitempty"`
	AutoApprove      bool    `json:"auto_approve"`
	ResumeSessionID  string  `json:"resume_session_id,omitempty"`
}

// Create creates a new session: generates ID, builds CLI command, spawns tmux, inserts DB.
func (m *Manager) Create(opts CreateOptions) (*db.Session, error) {
	if !provider.IsValid(opts.AgentType) {
		return nil, fmt.Errorf("invalid agent type: %s", opts.AgentType)
	}

	// Generate session ID (UUID format for tmux name)
	sessionID := shared.GenerateID("sess")
	tmuxName := fmt.Sprintf("%s-%s", opts.AgentType, sessionID)

	// Expand working directory
	cwd, err := shared.ExpandPath(opts.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("expand path: %w", err)
	}
	if cwd == "" {
		home, err := shared.ExpandPath("~")
		if err != nil {
			return nil, fmt.Errorf("expand home directory: %w", err)
		}
		cwd = home
	}

	// Build the agent command
	agentCmd, err := provider.BuildCommand(opts.AgentType, provider.BuildCommandOptions{
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
		WorkingDirectory:  opts.WorkingDirectory,
		AgentType:         opts.AgentType,
		Model:             opts.Model,
		SystemPrompt:      opts.SystemPrompt,
		AutoApprove:       opts.AutoApprove,
		ProviderSessionID: providerSessionID,
	}

	if err := m.db.CreateSession(session); err != nil {
		// Clean up tmux on DB error
		KillSession(tmuxName)
		return nil, fmt.Errorf("insert session: %w", err)
	}

	// Re-fetch to get defaults (created_at, updated_at, status)
	created, err := m.db.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	return created, nil
}

// Delete kills the tmux session and removes from DB.
func (m *Manager) Delete(id string) error {
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

	agentCmd, err := provider.BuildCommand(session.AgentType, provider.BuildCommandOptions{
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
