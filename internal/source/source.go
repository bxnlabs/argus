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
	//
	// URL-shaped inputs and "org/repo" shorthands are passed verbatim so
	// that filepath.Abs does not mangle their scheme or interpret them as
	// relative paths. For all other inputs (bare names, absolute paths that
	// don't exist), pass abs so that a relative path whose leading segment
	// has no dots cannot be mistaken for a shorthand org.
	if looksLikeRemote(input) {
		return parseRemote(input)
	}
	return parseRemote(abs)
}

// looksLikeRemote reports whether input is clearly a remote reference (SSH
// URL, HTTPS/HTTP URL, or "org/repo" shorthand) rather than a local path.
// Shorthand is identified as two slash-separated segments where the first
// contains no dots (dots are present in hostnames and relative path segments
// like "..").
func looksLikeRemote(input string) bool {
	if strings.HasPrefix(input, "git@") ||
		strings.HasPrefix(input, "https://") ||
		strings.HasPrefix(input, "http://") {
		return true
	}
	// Inputs that look like local paths are never shorthand.
	if strings.HasPrefix(input, "~") ||
		strings.HasPrefix(input, ".") ||
		filepath.IsAbs(input) {
		return false
	}
	// org/repo shorthand: exactly one "/", first segment non-empty and dot-free.
	parts := strings.SplitN(input, "/", 2)
	return len(parts) == 2 &&
		parts[0] != "" &&
		!strings.Contains(parts[0], ".") &&
		parts[1] != ""
}

func parseRemote(input string) (*Source, error) {
	// SSH: git@host:org/repo[.git]
	if rest, ok := strings.CutPrefix(input, "git@"); ok {
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		host := parts[0]
		if err := validateHost(host, input); err != nil {
			return nil, err
		}
		orgRepo := strings.TrimSuffix(parts[1], ".git")
		pr := strings.SplitN(orgRepo, "/", 2)
		if len(pr) != 2 || pr[0] == "" || pr[1] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		if err := validateOrgRepo(pr[0], pr[1], input); err != nil {
			return nil, err
		}
		return &Source{
			RemoteURL: "https://" + host + "/" + pr[0] + "/" + pr[1] + ".git",
			Host:      host,
			Org:       pr[0],
			Repo:      pr[1],
		}, nil
	}

	// HTTPS: https://host/org/repo[.git]
	var trimmed string
	isHTTP := false
	switch {
	case strings.HasPrefix(input, "https://"):
		trimmed = input[len("https://"):]
		isHTTP = true
	case strings.HasPrefix(input, "http://"):
		trimmed = input[len("http://"):]
		isHTTP = true
	}
	if isHTTP {
		parts := strings.SplitN(trimmed, "/", 3)
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		repo := strings.TrimSuffix(parts[2], ".git")
		if repo == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		if err := validateOrgRepo(parts[1], repo, input); err != nil {
			return nil, err
		}
		return &Source{
			RemoteURL: "https://" + parts[0] + "/" + parts[1] + "/" + repo + ".git",
			Host:      parts[0],
			Org:       parts[1],
			Repo:      repo,
		}, nil
	}

	// Shorthand: org/repo (implies github.com)
	parts := strings.SplitN(input, "/", 2)
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.Contains(parts[0], ".") {
		repo := strings.TrimSuffix(parts[1], ".git")
		if repo == "" {
			return nil, fmt.Errorf("not a valid path or git URL: %s", input)
		}
		if err := validateOrgRepo(parts[0], repo, input); err != nil {
			return nil, err
		}
		return &Source{
			RemoteURL: "https://github.com/" + parts[0] + "/" + repo + ".git",
			Host:      "github.com",
			Org:       parts[0],
			Repo:      repo,
		}, nil
	}

	return nil, fmt.Errorf("not a valid path or git URL: %s", input)
}

// validateHost returns an error if host contains characters that could cause
// path traversal when used in filepath.Join (e.g. "/" or "..").
func validateHost(host, input string) error {
	if strings.Contains(host, "/") || strings.Contains(host, "..") {
		return fmt.Errorf("not a valid path or git URL: %s", input)
	}
	return nil
}

// validateOrgRepo returns an error if org or repo contain path-traversal
// characters (".", "..", or "/") that could escape the intended storage dir.
func validateOrgRepo(org, repo, input string) error {
	for _, segment := range []string{org, repo} {
		if segment == "." || segment == ".." || strings.Contains(segment, "/") {
			return fmt.Errorf("not a valid path or git URL: %s", input)
		}
	}
	return nil
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
