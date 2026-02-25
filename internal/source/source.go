package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Source represents a resolved session source: either a local directory
// that exists on disk, or a remote git repository.
type Source struct {
	// Exactly one of LocalPath or RemoteURL is non-empty.
	LocalPath string // absolute local dir path (exists on disk)
	RemoteURL string // normalized HTTPS git URL

	// Set only when RemoteURL is non-empty.
	Host string // e.g. "github.com"
	Org  string // e.g. "bxnlabs"
	Repo string // e.g. "argus"
}

// IsRemote reports whether this source is a remote git repo.
func (s *Source) IsRemote() bool { return s.RemoteURL != "" }

// ParentKey returns the directory name used under ~/.argus/projects/.
//
//   - Local:  "--" + abspath with "/" replaced by "--"
//     e.g. /Users/jeevb/repos/argus → --Users--jeevb--repos--argus
//   - Remote: "host--org--repo"
//     e.g. github.com/bxnlabs/argus → github.com--bxnlabs--argus
func (s *Source) ParentKey() string {
	if s.IsRemote() {
		return s.Host + "--" + s.Org + "--" + s.Repo
	}
	// LocalPath is absolute so it starts with "/". Skip the leading slash
	// before replacing so we get "--seg1--seg2" rather than "----seg1--seg2".
	stripped := strings.TrimPrefix(s.LocalPath, "/")
	return "--" + strings.ReplaceAll(stripped, "/", "--")
}

// Resolve resolves input into a Source. It first checks whether input is an
// existing local directory; otherwise it attempts to parse it as a git URL
// or "org/repo" GitHub shorthand. Returns an error if neither interpretation
// is valid.
func Resolve(input string) (*Source, error) {
	expanded, err := expandTilde(input)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return nil, err
	}

	// Prefer local path if it exists as a directory.
	if info, statErr := os.Stat(abs); statErr == nil && info.IsDir() {
		return &Source{LocalPath: abs}, nil
	}

	// Otherwise try as remote.
	return parseRemote(input)
}

func parseRemote(input string) (*Source, error) {
	// SSH: git@host:org/repo[.git]
	if strings.HasPrefix(input, "git@") {
		rest := strings.TrimPrefix(input, "git@")
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		host := parts[0]
		orgRepo := strings.TrimSuffix(parts[1], ".git")
		pr := strings.SplitN(orgRepo, "/", 2)
		if len(pr) != 2 || pr[0] == "" || pr[1] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		return &Source{
			RemoteURL: "https://" + host + "/" + pr[0] + "/" + pr[1] + ".git",
			Host:      host,
			Org:       pr[0],
			Repo:      pr[1],
		}, nil
	}

	// HTTPS: https://host/org/repo[.git]
	if strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://") {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(input, "https://"), "http://")
		trimmed = strings.TrimSuffix(trimmed, ".git")
		parts := strings.SplitN(trimmed, "/", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		return &Source{
			RemoteURL: "https://" + parts[0] + "/" + parts[1] + "/" + parts[2] + ".git",
			Host:      parts[0],
			Org:       parts[1],
			Repo:      parts[2],
		}, nil
	}

	// Shorthand: org/repo (implies github.com)
	parts := strings.SplitN(input, "/", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(parts[0], ".") {
		repo := strings.TrimSuffix(parts[1], ".git")
		return &Source{
			RemoteURL: "https://github.com/" + parts[0] + "/" + repo + ".git",
			Host:      "github.com",
			Org:       parts[0],
			Repo:      repo,
		}, nil
	}

	return nil, fmt.Errorf("not a valid path or git URL: %s", input)
}

func expandTilde(p string) (string, error) {
	if !strings.HasPrefix(p, "~") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[1:]), nil
}
