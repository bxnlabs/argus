package files

import "path/filepath"

// defaultExcludePatterns lists file/directory names and glob patterns
// to filter from directory listings. Matches V1's DEFAULT_EXCLUDES
// (lib/files.ts:16-40) with a few universal additions.
//
// All patterns are matched with filepath.Match against the base name only.
var defaultExcludePatterns = []string{
	// JavaScript/TypeScript
	"node_modules",
	".next",
	".vercel",
	".turbo",
	".cache",
	".parcel-cache",

	// Python
	"__pycache__",
	".pytest_cache",
	".mypy_cache",
	".ruff_cache",
	".venv",
	"venv",
	"*.pyc",

	// Build outputs
	"dist",
	"build",
	"out",
	"coverage",

	// Version control
	".git",
	".svn",
	".hg",

	// OS files
	".DS_Store",
	"Thumbs.db",

	// Environment files
	".env",
	".env.local",
	".env.*.local",

	// Database files
	"*.db",
	"*.db-wal",
	"*.db-shm",

	// Logs
	"*.log",
}

// shouldExclude checks if a file/directory name should be filtered out.
// Uses filepath.Match for both exact names and glob patterns.
func shouldExclude(name string) bool {
	for _, pattern := range defaultExcludePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}
