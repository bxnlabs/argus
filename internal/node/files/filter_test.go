package files

import "testing"

func TestShouldExclude(t *testing.T) {
	// Exact name matches (from V1 DEFAULT_EXCLUDES)
	excluded := []string{
		"node_modules", ".git", ".next", "dist", "build", "out",
		"coverage", ".cache", ".vercel", ".turbo",
		"__pycache__", ".pytest_cache", ".mypy_cache", ".venv", "venv",
		".DS_Store",
	}
	for _, name := range excluded {
		t.Run("excluded_"+name, func(t *testing.T) {
			if !shouldExclude(name) {
				t.Errorf("expected %q to be excluded", name)
			}
		})
	}

	// Glob pattern matches
	globTests := []struct {
		name string
		want bool
	}{
		{"debug.log", true},                // *.log
		{"error.log", true},                // *.log
		{"module.pyc", true},               // *.pyc
		{".env", true},                     // .env (exact)
		{".env.local", true},               // .env.local (exact)
		{".env.development.local", true},   // .env.*.local
		{".env.production.local", true},    // .env.*.local
		{"data.db", true},                  // *.db
		{"data.db-wal", true},              // *.db-wal
		{"data.db-shm", true},              // *.db-shm
	}
	for _, tt := range globTests {
		t.Run("glob_"+tt.name, func(t *testing.T) {
			if got := shouldExclude(tt.name); got != tt.want {
				t.Errorf("shouldExclude(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}

	// Should NOT be excluded
	allowed := []string{
		"main.go", "README.md", "src", "package.json",
		".editorconfig", ".gitignore", "Makefile",
		"node_modules.txt", // substring match should NOT trigger
		"my.env",           // not exact ".env"
		"data.log.bak",     // not *.log
	}
	for _, name := range allowed {
		t.Run("allowed_"+name, func(t *testing.T) {
			if shouldExclude(name) {
				t.Errorf("expected %q to NOT be excluded", name)
			}
		})
	}
}
