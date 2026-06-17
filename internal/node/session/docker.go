package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/bxnlabs/argus/internal/node/docker"
	"github.com/bxnlabs/argus/internal/node/provider"
)

// stackOpTimeout bounds a single compose up/down so a hung Docker daemon can't
// hold a profile's mutex forever and wedge every future create/revival for that
// profile. It is deliberately generous — it must never fire mid-legitimate
// build/pull; its only job is to release the lock if Docker is truly dead.
const stackOpTimeout = 20 * time.Minute

// composeRunner is the subset of docker.CLICompose the Manager depends on, so
// tests can substitute a fake.
type composeRunner interface {
	Up(ctx context.Context, project, file string, env []string, opts docker.ComposeUpOpts) error
	Down(ctx context.Context, project, file string, env []string) error
}

// ProfileInfo describes a profile for the listing API/CLI.
type ProfileInfo struct {
	Name       string `json:"name"`
	Dockerized bool   `json:"dockerized"`
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

// composeArgs resolves the project name and interpolation env for a profile's
// stack. The compose file path comes from docker.ProfileComposeFile at the call
// site, so it is not bundled here.
func (m *Manager) composeArgs(profile string) (project string, env []string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, fmt.Errorf("resolve home: %w", err)
	}
	return docker.ProjectName(profile), docker.Env(home, m.stateDir), nil
}

// bringStackUp runs `docker compose up -d` (with opts) for the profile's stack,
// serialized per profile and bounded by stackOpTimeout. `up -d` is idempotent:
// it reconciles to the compose file's desired state, leaves unchanged running
// containers alone, and restarts a dead service — so it is safe to always run.
func (m *Manager) bringStackUp(profile, composeFile string, opts docker.ComposeUpOpts) error {
	l := m.profileLock(profile)
	l.Lock()
	defer l.Unlock()

	project, env, err := m.composeArgs(profile)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), stackOpTimeout)
	defer cancel()
	return m.compose.Up(ctx, project, composeFile, env, opts)
}

// ensureStackUp is the lazy path used on session create/revival: a fast
// `up -d` that ensures the stack is running and auto-applies compose-config
// changes, without rebuilding or re-pulling images.
func (m *Manager) ensureStackUp(profile, composeFile string) error {
	return m.bringStackUp(profile, composeFile, docker.ComposeUpOpts{})
}

// ProfileUp is the explicit refresh path (`argus profile up`): `up -d --build
// --pull always`, rebuilding local images and re-pulling registry images. It
// only acts on real, usable (hooks-bearing) dockerized profiles. Returns
// ErrInvalidInput if the profile is missing, invalid, or not dockerized.
func (m *Manager) ProfileUp(name string) error {
	file, err := m.dockerizedProfileFile(name)
	if err != nil {
		return err
	}
	return m.bringStackUp(name, file, docker.ComposeUpOpts{Build: true, Pull: true})
}

// ProfileDown tears a dockerized profile's stack down. Like ProfileUp it only
// acts on real, usable dockerized profiles.
func (m *Manager) ProfileDown(name string) error {
	file, err := m.dockerizedProfileFile(name)
	if err != nil {
		return err
	}
	l := m.profileLock(name)
	l.Lock()
	defer l.Unlock()
	project, env, err := m.composeArgs(name)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), stackOpTimeout)
	defer cancel()
	return m.compose.Down(ctx, project, file, env)
}

// dockerizedProfileFile validates that name is a real, usable profile (via
// resolveProfile, which checks the name and requires hooks/) and that it is
// dockerized, returning its compose file. All failures map to ErrInvalidInput
// so the API returns 400.
func (m *Manager) dockerizedProfileFile(name string) (string, error) {
	if _, err := m.resolveProfile(&name); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidInput, err)
	}
	file, ok := docker.ProfileComposeFile(m.stateDir, name)
	if !ok {
		return "", fmt.Errorf("%w: profile %q is not dockerized", ErrInvalidInput, name)
	}
	return file, nil
}

// ListProfilesDetailed returns every profile annotated with whether it is
// dockerized. It is a pure directory-stat loop and does no Docker subprocess
// work, so GET /profiles is cheap. Operators who want live stack status use
// `docker compose -p argus-<name> ps` directly.
func (m *Manager) ListProfilesDetailed() ([]ProfileInfo, error) {
	names, err := m.ListProfiles()
	if err != nil {
		return nil, err
	}
	out := make([]ProfileInfo, 0, len(names))
	for _, name := range names {
		_, dockerized := docker.ProfileComposeFile(m.stateDir, name)
		out = append(out, ProfileInfo{Name: name, Dockerized: dockerized})
	}
	return out, nil
}

// buildTmuxCmd constructs the command tmux runs for a session: the init script
// that launches the agent (or shell) and sources post_create hooks. For a
// dockerized profile the agent/shell runs inside the profile's container via
// `docker compose exec`; otherwise it runs directly on the host. An empty
// return means "start tmux's default shell with no init script".
func (m *Manager) buildTmuxCmd(sessionID, providerType, profile, cwd, agentCmd string, postCreatePaths []string) (string, error) {
	// Normalize "" → "default" so dockerization is consistent with the hooks
	// layer: a no-profile session already sources profiles/default/'s hooks, so
	// a profiles/default/ compose file must dockerize it too. The normalized
	// name flows downstream so detection, ProjectName, ensureStackUp, and the
	// exec all agree on the same stack (argus-default).
	dockerProfile := effectiveProfileName(profile)
	if composeFile, ok := docker.ProfileComposeFile(m.stateDir, dockerProfile); ok {
		return m.buildDockerTmuxCmd(sessionID, providerType, dockerProfile, composeFile, cwd, agentCmd, postCreatePaths)
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

	var content string
	pattern := ""
	if agentCmd != "" {
		content = GenerateContainerInitScript(agentCmd, postCreatePaths)
		pattern = provider.GetSessionIDPattern(provider.ProviderType(providerType))
	} else {
		content = GenerateContainerShellInitScript(postCreatePaths)
	}
	innerPath, err := writeContainerScript(sessionID, m.stateDir, content)
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
		// docker compose exec re-interpolates the whole compose file at exec
		// time, so it needs the same ARGUS_* vars `up` saw or ${ARGUS_*} blanks.
		Env:     docker.Env(home, m.stateDir),
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
