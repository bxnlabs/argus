package cli

import (
	neturl "net/url"
	"strings"
)

// parseRepo extracts "org/repo" (or a deeper subgroup path) from a git remote
// URL. Supports scp-style SSH (git@host:path) and https://host/path. Returns ""
// when the URL doesn't match an expected shape.
func parseRepo(url string) string {
	if url == "" {
		return ""
	}

	var path string

	// scp-style SSH (user@host:path) has no scheme. Scheme-based URLs
	// (https://, ssh://, git://) are handled by net/url below.
	if !strings.Contains(url, "://") {
		if at := strings.Index(url, "@"); at >= 0 {
			if colon := strings.Index(url[at:], ":"); colon >= 0 {
				path = url[at+colon+1:]
			}
		}
	}

	if path == "" {
		if u, err := neturl.Parse(url); err == nil && len(u.Path) > 1 {
			path = u.Path
		}
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if !strings.Contains(path, "/") {
		return ""
	}
	return path
}
