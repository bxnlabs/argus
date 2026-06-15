package session

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/bxnlabs/argus/internal/node/docker"
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

// dockerIdentity returns the host UID/GID as strings for `exec --user`.
func dockerIdentity() (uid, gid string) {
	return strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
}
