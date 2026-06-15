package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/bxnlabs/argus/internal/node/docker"
	"github.com/bxnlabs/argus/internal/node/provider"
)

// composeRunner is the subset of docker.CLICompose the Manager depends on, so
// tests can substitute a fake.
type composeRunner interface {
	Up(ctx context.Context, project, file string, env []string) error
	Down(ctx context.Context, project, file string, env []string) error
	IsUp(ctx context.Context, project, file string, env []string) (bool, error)
}

// ProfileInfo describes a profile for the listing API/CLI.
type ProfileInfo struct {
	Name       string `json:"name"`
	Dockerized bool   `json:"dockerized"`
	Stack      string `json:"stack"` // "up" | "down" | "-" (host) | "?" (daemon error)
}

// profileLock returns a per-profile mutex, creating it if needed. It serializes
// stack up/down for a profile so concurrent session creates bring the shared
// stack up exactly once.
func (m *Manager) profileLock(name string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profileLks == nil {
		m.profileLks = make(map[string]*sync.Mutex)
	}
	l, ok := m.profileLks[name]
	if !ok {
		l = &sync.Mutex{}
		m.profileLks[name] = l
	}
	return l
}

// ensureStackUp brings the profile's compose stack up if it is not already
// running. Serialized per profile.
func (m *Manager) ensureStackUp(profile, composeFile string) error {
	l := m.profileLock(profile)
	l.Lock()
	defer l.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	project := docker.ProjectName(profile)
	env := docker.Env(home, m.stateDir)
	up, err := m.compose.IsUp(context.Background(), project, composeFile, env)
	if err != nil {
		return err
	}
	if up {
		return nil
	}
	return m.compose.Up(context.Background(), project, composeFile, env)
}

// ProfileUp brings a dockerized profile's stack up. Returns ErrInvalidInput if
// the profile is not dockerized.
func (m *Manager) ProfileUp(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	file, ok := docker.ProfileComposeFile(m.stateDir, name)
	if !ok {
		return fmt.Errorf("%w: profile %q is not dockerized", ErrInvalidInput, name)
	}
	return m.ensureStackUp(name, file)
}

// ProfileDown tears a dockerized profile's stack down.
func (m *Manager) ProfileDown(name string) error {
	if err := ValidateProfileName(name); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	file, ok := docker.ProfileComposeFile(m.stateDir, name)
	if !ok {
		return fmt.Errorf("%w: profile %q is not dockerized", ErrInvalidInput, name)
	}
	l := m.profileLock(name)
	l.Lock()
	defer l.Unlock()
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	return m.compose.Down(context.Background(), docker.ProjectName(name), file, docker.Env(home, m.stateDir))
}

// ListProfilesDetailed returns every profile annotated with its type and, for
// dockerized profiles, its stack status. Status lookups are best-effort: a
// Docker daemon error yields "?" rather than failing the listing.
func (m *Manager) ListProfilesDetailed() ([]ProfileInfo, error) {
	names, err := m.ListProfiles()
	if err != nil {
		return nil, err
	}
	home, _ := os.UserHomeDir()
	out := make([]ProfileInfo, 0, len(names))
	for _, name := range names {
		info := ProfileInfo{Name: name, Stack: "-"}
		if file, ok := docker.ProfileComposeFile(m.stateDir, name); ok {
			info.Dockerized = true
			info.Stack = "?"
			if up, err := m.compose.IsUp(context.Background(), docker.ProjectName(name), file, docker.Env(home, m.stateDir)); err == nil {
				if up {
					info.Stack = "up"
				} else {
					info.Stack = "down"
				}
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// buildTmuxCmd constructs the command tmux runs for a session: the init script
// that launches the agent (or shell) and sources post_create hooks. For a
// dockerized profile the agent/shell runs inside the profile's container via
// `docker compose exec`; otherwise it runs directly on the host. An empty
// return means "start tmux's default shell with no init script".
func (m *Manager) buildTmuxCmd(sessionID, providerType, profile, cwd, agentCmd string, postCreatePaths []string) (string, error) {
	if composeFile, ok := docker.ProfileComposeFile(m.stateDir, profile); ok {
		return m.buildDockerTmuxCmd(sessionID, providerType, profile, composeFile, cwd, agentCmd, postCreatePaths)
	}
	// Host (non-docker) path — unchanged behavior.
	if agentCmd != "" {
		pattern := provider.GetSessionIDPattern(provider.ProviderType(providerType))
		scriptPath, err := WriteInitScript(sessionID, agentCmd, pattern, postCreatePaths)
		if err != nil {
			return "", fmt.Errorf("write init script: %w", err)
		}
		return "bash " + scriptPath, nil
	}
	if len(postCreatePaths) > 0 {
		scriptPath, err := WriteShellInitScript(sessionID, postCreatePaths)
		if err != nil {
			return "", fmt.Errorf("write shell init script: %w", err)
		}
		if scriptPath != "" {
			return "bash " + scriptPath, nil
		}
	}
	return "", nil
}

// buildDockerTmuxCmd builds the host wrapper command for a dockerized profile.
// It validates the cwd is visible in the container, ensures the stack is up,
// writes the inner init script under the mounted state tmp dir, and wraps it in
// a `docker compose exec` invocation embedded in the standard host init script
// (which provides the banner and, for resume-capable providers, the
// provider-session-ID capture).
func (m *Manager) buildDockerTmuxCmd(sessionID, providerType, profile, composeFile, cwd, agentCmd string, postCreatePaths []string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	if !docker.PathVisible(cwd, home, m.stateDir) {
		return "", fmt.Errorf("%w: working directory %q is not under the home or state directory mounted into the %q profile container", ErrInvalidInput, cwd, profile)
	}
	if err := m.ensureStackUp(profile, composeFile); err != nil {
		return "", fmt.Errorf("start profile %q stack: %w", profile, err)
	}

	var innerPath string
	pattern := ""
	if agentCmd != "" {
		innerPath, err = WriteContainerInitScript(sessionID, m.stateDir, agentCmd, postCreatePaths)
		pattern = provider.GetSessionIDPattern(provider.ProviderType(providerType))
	} else {
		innerPath, err = WriteContainerShellInitScript(sessionID, m.stateDir, postCreatePaths)
	}
	if err != nil {
		return "", err
	}

	uid, gid := dockerIdentity()
	execCmd := docker.ExecCommand(docker.ExecOptions{
		Project: docker.ProjectName(profile),
		File:    composeFile,
		Workdir: cwd,
		UID:     uid,
		GID:     gid,
		Service: "agent",
		Command: "bash " + shellQuote(innerPath),
	})

	// The host wrapper is the standard init script with the agent command set
	// to the docker-exec string and no host-side hooks (hooks are sourced
	// inside the container by the inner script).
	hostPath, err := WriteInitScript(sessionID, execCmd, pattern, nil)
	if err != nil {
		return "", fmt.Errorf("write host wrapper script: %w", err)
	}
	return "bash " + hostPath, nil
}

// dockerIdentity returns the host UID/GID as strings for `exec --user`.
func dockerIdentity() (uid, gid string) {
	return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
}
