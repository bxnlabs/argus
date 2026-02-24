package filesearch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFdArgs_Hidden(t *testing.T) {
	args := buildFdArgs("test", "", 20)

	hasHidden := false
	for _, a := range args {
		if a == "--hidden" {
			hasHidden = true
		}
	}
	if !hasHidden {
		t.Error("expected --hidden flag")
	}

	// Verify at least some exclusions are present
	hasExclude := false
	for i, a := range args {
		if a == "--exclude" && i+1 < len(args) && args[i+1] == ".cache" {
			hasExclude = true
		}
	}
	if !hasExclude {
		t.Error("expected --exclude .cache")
	}
}

func TestBuildFdArgs_Directory(t *testing.T) {
	args := buildFdArgs("test", "directory", 20)

	hasType := false
	for i, a := range args {
		if a == "-t" && i+1 < len(args) && args[i+1] == "d" {
			hasType = true
		}
	}
	if !hasType {
		t.Error("expected -t d for directory filter")
	}
}

func TestBuildFdArgs_File(t *testing.T) {
	args := buildFdArgs("test", "file", 20)

	hasType := false
	for i, a := range args {
		if a == "-t" && i+1 < len(args) && args[i+1] == "f" {
			hasType = true
		}
	}
	if !hasType {
		t.Error("expected -t f for file filter")
	}
}

func TestBuildFdArgs_Both(t *testing.T) {
	args := buildFdArgs("test", "", 20)

	for _, a := range args {
		if a == "-t" {
			t.Error("should not have -t flag when type is empty")
		}
	}
}

func TestRelevanceScore(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"argus", "argus", 3},        // exact
		{"argus-v2", "argus", 2},     // prefix
		{"my-argus-app", "argus", 1}, // contains
		{"other", "argus", 0},        // no match
	}
	for _, tt := range tests {
		got := relevanceScore(tt.name, tt.query)
		if got != tt.want {
			t.Errorf("relevanceScore(%q, %q) = %d, want %d", tt.name, tt.query, got, tt.want)
		}
	}
}

func TestSortByRelevance(t *testing.T) {
	results := []FileSearchResult{
		{Name: "contains-argus", Path: "/long/path/contains-argus"},
		{Name: "argus", Path: "/short/argus"},
		{Name: "argus-prefix", Path: "/medium/argus-prefix"},
	}
	sortByRelevance(results, "argus")

	if results[0].Name != "argus" {
		t.Errorf("first result should be exact match, got %s", results[0].Name)
	}
	if results[1].Name != "argus-prefix" {
		t.Errorf("second result should be prefix match, got %s", results[1].Name)
	}
	if results[2].Name != "contains-argus" {
		t.Errorf("third result should be contains match, got %s", results[2].Name)
	}
}

func TestSortByRelevance_TieBreaker(t *testing.T) {
	results := []FileSearchResult{
		{Name: "argus", Path: "/very/long/deep/path/argus"},
		{Name: "argus", Path: "/short/argus"},
	}
	sortByRelevance(results, "argus")

	if results[0].Path != "/short/argus" {
		t.Errorf("shorter path should come first, got %s", results[0].Path)
	}
}

func TestParseOutput_Empty(t *testing.T) {
	results := parseOutput("", "test", "directory", 20)
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestEnsureIgnoreFile(t *testing.T) {
	t.Run("creates default file when missing", func(t *testing.T) {
		home := t.TempDir()
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".argus", "search-ignore")
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		if !strings.Contains(content, ".git/") {
			t.Error("default file should contain .git/")
		}
		if !strings.Contains(content, "Library/") {
			t.Error("default file should contain Library/")
		}
		if !strings.Contains(content, "node_modules/") {
			t.Error("default file should contain node_modules/")
		}
	})

	t.Run("preserves existing file", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".argus")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		custom := "custom-pattern/\n"
		if err := os.WriteFile(filepath.Join(dir, "search-ignore"), []byte(custom), 0o600); err != nil {
			t.Fatal(err)
		}
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != custom {
			t.Errorf("existing file was overwritten: got %q, want %q", string(data), custom)
		}
		_ = path
	})
}
