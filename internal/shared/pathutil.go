package shared

import "strings"

// TruncateRight right-truncates s to max runes, suffixing with "…" if truncated.
func TruncateRight(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// CompressPath shortens a path for display, prioritizing the basename (last
// segment) over parent directories. Compression stages:
//  1. Replace home prefix with ~
//  2. If over threshold with ≤3 segments, truncate the basename
//  3. Keep first + last 2 segments: ~/first/.../parent/basename
//  4. Drop first segment: ~/.../parent/basename
//  5. Drop parent segment: ~/.../basename
//  6. Truncate basename as last resort: ~/.../basen…
func CompressPath(path, home string, threshold int) string {
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
	basename := segments[len(segments)-1]

	if len(segments) <= 3 {
		// For exactly 3 segments, try dropping the middle to preserve basename
		if len(segments) == 3 {
			result := prefix + "/.../" + basename
			if len(result) <= threshold {
				return result
			}
		}
		return TruncateRight(display, threshold)
	}

	// Try: ~/first/.../parent/basename
	first := segments[0]
	parent := segments[len(segments)-2]
	result := prefix + "/" + first + "/.../" + parent + "/" + basename
	if len(result) <= threshold {
		return result
	}

	// Try: ~/.../parent/basename
	result = prefix + "/.../" + parent + "/" + basename
	if len(result) <= threshold {
		return result
	}

	// Try: ~/.../basename (drop parent, prioritize basename)
	ellipsis := prefix + "/.../"
	result = ellipsis + basename
	if len(result) <= threshold {
		return result
	}

	// Last resort: truncate the basename itself
	return TruncateRight(result, threshold)
}
