// Package docker runs profile agent sessions inside docker-compose stacks.
// A profile is "dockerized" when its directory contains a compose file; the
// agent then runs via `docker compose exec` into a shared, per-profile stack.
package docker

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// composeFileNames are the compose file names recognized in a profile
// directory, in priority order.
var composeFileNames = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// ProfileComposeFile returns the path to a profile's compose file and true when
// the profile is dockerized. It returns "" and false otherwise.
func ProfileComposeFile(stateDir, profile string) (string, bool) {
	for _, name := range composeFileNames {
		p := filepath.Join(stateDir, "profiles", profile, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, true
		}
	}
	return "", false
}

// ProjectName returns the stable docker-compose project name for a profile.
// All sessions of a profile share this project (one stack per profile).
func ProjectName(profile string) string {
	return "argus-" + profile
}

// Env returns the environment passed to compose invocations so the compose
// file can mount the host home and state dir at identical paths and run the
// agent as the host user.
func Env(home, stateDir string) []string {
	return []string{
		"ARGUS_HOST_HOME=" + home,
		"ARGUS_STATE_DIR=" + stateDir,
		"ARGUS_UID=" + strconv.Itoa(os.Getuid()),
		"ARGUS_GID=" + strconv.Itoa(os.Getgid()),
	}
}

// PathVisible reports whether path is under the host home or the state dir,
// the two roots mounted into the container at identical paths. A session whose
// working directory is outside both cannot be seen inside the container.
func PathVisible(path, home, stateDir string) bool {
	return underRoot(path, home) || underRoot(path, stateDir)
}

func underRoot(path, root string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel))
}
