package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// Profile execution types reported by the listing API/CLI. A profile is
// "docker" when its directory holds a compose file (its sessions run via
// `docker compose exec`), otherwise "host".
const (
	ProfileTypeHost   = "host"
	ProfileTypeDocker = "docker"
)

// ProfileInfo describes a profile for the listing API/CLI.
type ProfileInfo struct {
	Name string `json:"name"`
	Type string `json:"type"` // ProfileTypeHost or ProfileTypeDocker
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

// profileHasHooks reports whether dockerProfile is a configured profile: a
// directory under profiles/ holding a hooks/ subdirectory. This is the same bar
// ListProfiles and resolveProfile apply, so it defines what counts as a profile.
func (m *Manager) profileHasHooks(dockerProfile string) bool {
	info, err := os.Stat(filepath.Join(m.stateDir, "profiles", dockerProfile, "hooks"))
	return err == nil && info.IsDir()
}

// dockerComposeFile returns a profile's compose file and true only when the
// profile is real (hooks-bearing). Keying detection on the compose file alone
// would dockerize a profiles/<name>/ dir that has a compose file but no hooks/
// — a "profile" that ListProfiles, resolveProfile, and `profile up/down` all
// refuse to recognize, silently running its sessions in a stack nobody can list
// or stop. Gating on hooks keeps implicit dockerization consistent with explicit
// management.
func (m *Manager) dockerComposeFile(dockerProfile string) (string, bool) {
	if !m.profileHasHooks(dockerProfile) {
		return "", false
	}
	return docker.ProfileComposeFile(m.stateDir, dockerProfile)
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

// prepareDockerTarget runs the deterministic docker preconditions for a profile
// — verifying the cwd is visible in the container and bringing the stack up. It
// is a no-op for non-dockerized (host) profiles. profile is normalized with
// effectiveProfileName so the implicit-default and explicit "default" cases
// resolve to the same dockerized stack as buildTmuxCmd.
//
// ChangeProfile calls this BEFORE killing the live session so a predictable
// docker failure (cwd not mounted, unstartable stack) leaves the original
// session alive and untouched — the same "validate before teardown" contract
// the cwd and pre_create preflights already follow. buildDockerTmuxCmd re-runs
// it on respawn, which is safe: PathVisible is pure and `up -d` is idempotent.
func (m *Manager) prepareDockerTarget(profile, cwd string) error {
	dockerProfile := effectiveProfileName(profile)
	composeFile, ok := m.dockerComposeFile(dockerProfile)
	if !ok {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	if !docker.PathVisible(cwd, home, m.stateDir) {
		return fmt.Errorf("%w: working directory %q is not under the home or state directory mounted into the %q profile container", ErrInvalidInput, cwd, dockerProfile)
	}
	if err := m.ensureStackUp(dockerProfile, composeFile); err != nil {
		return fmt.Errorf("%w: profile %q: %w", ErrStackStart, dockerProfile, err)
	}
	return nil
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
	if err := m.bringStackUp(name, file, docker.ComposeUpOpts{Build: true, Pull: true}); err != nil {
		return fmt.Errorf("%w: profile %q: %w", ErrStackStart, name, err)
	}
	return nil
}

// ProfileDown tears a dockerized profile's stack down. Like ProfileUp it only
// acts on real, usable dockerized profiles. It refuses when live sessions are
// still running in the profile's shared stack, since `down` would kill their
// containers out from under them (returns ErrProfileInUse).
func (m *Manager) ProfileDown(name string) error {
	file, err := m.dockerizedProfileFile(name)
	if err != nil {
		return err
	}
	l := m.profileLock(name)
	l.Lock()
	defer l.Unlock()

	// Checked under the profile lock so it serializes against concurrent
	// up/down for this profile. Known limitation: session create brings the
	// stack up (ensureStackUp) and only inserts the DB row afterwards, so a
	// session mid-creation in this profile is briefly invisible here and a
	// racing `down` could stop the stack under it. The window is small,
	// `profile down` is a deliberate manual operation, and the worst case is a
	// failed `docker compose exec` in the in-flight session — recoverable by
	// retry — so we accept it rather than locking across the whole create path.
	inUse, err := m.profileInUse(name)
	if err != nil {
		return err
	}
	if inUse {
		return fmt.Errorf("%w: profile %q has live sessions; stop them before bringing the stack down", ErrProfileInUse, name)
	}

	project, env, err := m.composeArgs(name)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), stackOpTimeout)
	defer cancel()
	if err := m.compose.Down(ctx, project, file, env); err != nil {
		return fmt.Errorf("%w: profile %q: %w", ErrStackStop, name, err)
	}
	return nil
}

// profileInUse reports whether any live tmux session resolves to the given
// profile. Sessions are matched on their effective profile name, so the
// implicit-default ("") and explicit "default" sessions both count against the
// "default" profile, consistent with how the stack is named and brought up.
func (m *Manager) profileInUse(name string) (bool, error) {
	sessions, err := m.db.ListSessions(context.Background())
	if err != nil {
		return false, fmt.Errorf("list sessions: %w", err)
	}
	target := effectiveProfileName(name)
	for _, s := range sessions {
		if effectiveProfileName(ptrStr(s.Profile)) != target {
			continue
		}
		if HasSession(s.TmuxName) {
			return true, nil
		}
	}
	return false, nil
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
		typ := ProfileTypeHost
		if _, dockerized := docker.ProfileComposeFile(m.stateDir, name); dockerized {
			typ = ProfileTypeDocker
		}
		out = append(out, ProfileInfo{Name: name, Type: typ})
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
	if composeFile, ok := m.dockerComposeFile(dockerProfile); ok {
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
	// Validate cwd visibility and bring the stack up. Idempotent with the
	// preflight ChangeProfile runs before teardown (see prepareDockerTarget).
	if err := m.prepareDockerTarget(profile, cwd); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
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

	// No -u: the agent runs as the image's default user. The profile's image is
	// responsible for baking a user that matches the host uid/gid and making it
	// the default (USER directive), so the agent lands in a real account with the
	// right HOME and file ownership. See GenerateContainerInitScript and the
	// dockerized-profiles design doc.
	execCmd := docker.ExecCommand(docker.ExecOptions{
		Project: docker.ProjectName(profile),
		File:    composeFile,
		Workdir: cwd,
		Service: "agent",
		// docker compose exec re-interpolates the whole compose file at exec
		// time, so it needs the same ARGUS_* vars `up` saw or ${ARGUS_*} blanks.
		Env:  docker.Env(home, m.stateDir),
		Args: []string{"bash", innerPath},
	})

	// The host wrapper is the standard init script with the agent command set
	// to the docker-exec string and no host-side hooks (hooks are sourced
	// inside the container by the inner script). An EXIT trap removes the inner
	// script as a safety net: it normally self-deletes on start, but if
	// `docker compose exec` fails before it runs (e.g. the stack died), the trap
	// still cleans it out of <stateDir>/tmp instead of leaving it behind.
	hostAgentCmd := "__argus_inner=" + shellQuote(innerPath) + "\n" +
		`trap 'rm -f -- "$__argus_inner"' EXIT` + "\n" +
		execCmd
	hostPath, err := WriteInitScript(sessionID, hostAgentCmd, pattern, nil)
	if err != nil {
		return "", fmt.Errorf("write host wrapper script: %w", err)
	}
	return "bash " + hostPath, nil
}
