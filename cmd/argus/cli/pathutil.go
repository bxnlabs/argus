package cli

import (
	"path/filepath"
	"strings"
)

// compressPath shortens a path for display:
// 1. Replace home prefix with ~
// 2. If longer than threshold, keep first + last 2 segments joined by /.../
func compressPath(path, home string, threshold int) string {
	// Step 1: tilde-shorten
	display := path
	if home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			display = "~" + path[len(home):]
		}
	}

	// Step 2: compress if over threshold
	if len(display) <= threshold {
		return display
	}

	// Split into prefix (~ or empty) and segments
	var prefix string
	rest := display
	if strings.HasPrefix(display, "~/") {
		prefix = "~"
		rest = display[1:] // "/Workspace/repos/..."
	}

	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) <= 3 {
		// Can't compress further — already first + 2 tail segments
		return display
	}

	// Keep first segment + last 2 segments
	first := segments[0]
	tail := segments[len(segments)-2:]
	return prefix + "/" + first + "/.../" + filepath.Join(tail[0], tail[1])
}
