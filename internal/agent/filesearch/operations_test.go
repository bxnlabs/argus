package filesearch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ignore "github.com/sabhiram/go-gitignore"
)

// createTree creates a directory tree from a map. Keys ending in "/" are
// directories; all others are files whose content is the map value.
// Returns the absolute path to the root temp directory.
func createTree(t *testing.T, tree map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range tree {
		p := filepath.Join(root, name)
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSearch_BasicFuzzyMatch(t *testing.T) {
	root := createTree(t, map[string]string{
		"controller.go":      "",
		"controller_test.go": "",
		"model.go":           "",
		"readme.md":          "",
	})

	resp, err := searchInDir(context.Background(), root, "ctrl", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected at least one result for fuzzy query 'ctrl'")
	}
	// "controller.go" should rank higher than "controller_test.go" (shorter path)
	// Both should match; model.go and readme.md should not.
	for _, r := range resp.Results {
		if !strings.Contains(r.Name, "controller") {
			t.Errorf("unexpected result %q for query 'ctrl'", r.Name)
		}
	}
}

func TestSearch_TypeFilter(t *testing.T) {
	root := createTree(t, map[string]string{
		"src/":         "",
		"src/main.go":  "",
		"src/utils.go": "",
		"docs/":        "",
		"lib/":         "",
	})

	t.Run("file only", func(t *testing.T) {
		resp, err := searchInDir(context.Background(), root, "main", "file", 20, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range resp.Results {
			if r.Type != "file" {
				t.Errorf("expected type 'file', got %q for %s", r.Type, r.Path)
			}
		}
	})

	t.Run("directory only", func(t *testing.T) {
		resp, err := searchInDir(context.Background(), root, "src", "directory", 20, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Count == 0 {
			t.Fatal("expected at least one directory result")
		}
		for _, r := range resp.Results {
			if r.Type != "directory" {
				t.Errorf("expected type 'directory', got %q for %s", r.Type, r.Path)
			}
		}
	})
}

func TestSearch_IgnorePatterns(t *testing.T) {
	root := createTree(t, map[string]string{
		"src/main.go":                "",
		"node_modules/pkg/index.js":  "",
		".git/config":               "",
		"vendor/lib.go":             "",
	})

	matcher := ignore.CompileIgnoreLines("node_modules/", ".git/")

	resp, err := searchInDir(context.Background(), root, "index", "", 20, matcher, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if strings.Contains(r.Path, "node_modules") {
			t.Errorf("node_modules should be ignored, got result: %s", r.Path)
		}
		if strings.Contains(r.Path, ".git") {
			t.Errorf(".git should be ignored, got result: %s", r.Path)
		}
	}

	// "main.go" or "vendor/lib.go" should still be findable.
	resp2, err := searchInDir(context.Background(), root, "main", "", 20, matcher, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Count == 0 {
		t.Fatal("expected main.go to be found")
	}
}

func TestSearch_NegationPatterns(t *testing.T) {
	root := createTree(t, map[string]string{
		"allowed/keep.txt":    "",
		"blocked/remove.txt":  "",
		"other/data.txt":      "",
	})

	// Ignore everything, then un-ignore allowed/
	matcher := ignore.CompileIgnoreLines("*", "!allowed/")

	resp, err := searchInDir(context.Background(), root, "keep", "", 20, matcher, "")
	if err != nil {
		t.Fatal(err)
	}

	foundKeep := false
	for _, r := range resp.Results {
		if strings.Contains(r.Name, "keep") {
			foundKeep = true
		}
		if strings.Contains(r.Path, "blocked") {
			t.Errorf("blocked/ should be ignored, got result: %s", r.Path)
		}
		if strings.Contains(r.Path, "other") {
			t.Errorf("other/ should be ignored, got result: %s", r.Path)
		}
	}
	if !foundKeep {
		t.Error("expected 'keep.txt' to be found via negation pattern")
	}
}

func TestSearch_MaxDepth(t *testing.T) {
	// Create a file nested 10 levels deep (beyond maxDepth=8).
	tree := map[string]string{}
	deepPath := "a/b/c/d/e/f/g/h/i/j/deep.txt"
	tree[deepPath] = "deep"
	// Also create a shallow file to ensure search works.
	tree["shallow.txt"] = "shallow"

	root := createTree(t, tree)

	resp, err := searchInDir(context.Background(), root, "deep", "file", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if r.Name == "deep.txt" {
			t.Errorf("deep.txt at depth 10 should not be found (maxDepth=%d)", maxDepth)
		}
	}

	// Shallow file should be found.
	resp2, err := searchInDir(context.Background(), root, "shallow", "file", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp2.Count == 0 {
		t.Fatal("expected shallow.txt to be found")
	}
}

func TestSearch_Limit(t *testing.T) {
	tree := map[string]string{}
	for i := 0; i < 30; i++ {
		tree[filepath.Join("files", strings.Repeat("a", i+1)+".txt")] = ""
	}
	root := createTree(t, tree)

	resp, err := searchInDir(context.Background(), root, "a", "file", 5, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count > 5 {
		t.Errorf("expected at most 5 results, got %d", resp.Count)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	root := createTree(t, map[string]string{"file.txt": ""})

	_, err := searchInDir(context.Background(), root, "", "", 20, nil, "")
	if err == nil {
		t.Error("expected error for empty query")
	}

	_, err = searchInDir(context.Background(), root, "   ", "", 20, nil, "")
	if err == nil {
		t.Error("expected error for whitespace-only query")
	}
}

func TestSearch_AbsolutePaths(t *testing.T) {
	root := createTree(t, map[string]string{
		"src/main.go":  "",
		"src/utils.go": "",
		"readme.md":    "",
	})

	resp, err := searchInDir(context.Background(), root, "main", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range resp.Results {
		if !filepath.IsAbs(r.Path) {
			t.Errorf("expected absolute path, got %q", r.Path)
		}
	}
}

func TestEnsureIgnoreFile(t *testing.T) {
	t.Run("creates default file when missing", func(t *testing.T) {
		home := t.TempDir()
		path, err := ensureIgnoreFile(home)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".argus", "ignore")
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		content := string(data)
		for _, want := range []string{".git/", "node_modules/", "__pycache__/", "target/", ".venv/"} {
			if !strings.Contains(content, want) {
				t.Errorf("default file should contain %q", want)
			}
		}
	})

	t.Run("preserves existing file", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".argus")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		custom := "custom-pattern/\n"
		if err := os.WriteFile(filepath.Join(dir, "ignore"), []byte(custom), 0o600); err != nil {
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
	})
}

func TestSearch_IgnoreRootAnchor(t *testing.T) {
	// Simulate the real scenario: ignore file has * + !Workspace/ patterns
	// and we're searching from a subdirectory of Workspace/.
	root := createTree(t, map[string]string{
		"Workspace/project/main.go":    "",
		"Workspace/project/lib.go":     "",
		"Workspace/project/.git/HEAD":  "",
		"Downloads/file.txt":           "",
	})

	// Patterns relative to root (like ~/.argus/ignore relative to $HOME).
	matcher := ignore.CompileIgnoreLines("*", "!Workspace/", ".git/")

	// Searching from the subdirectory with ignoreRoot set to parent.
	projectDir := filepath.Join(root, "Workspace", "project")
	resp, err := searchInDir(context.Background(), projectDir, "main", "file", 20, matcher, root)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected results when searching subdirectory with ignoreRoot set to parent")
	}

	// .git/ should still be excluded.
	for _, r := range resp.Results {
		if strings.Contains(r.Path, ".git") {
			t.Errorf(".git should be ignored, got result: %s", r.Path)
		}
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	root := createTree(t, map[string]string{
		"README.md":      "",
		"Makefile":        "",
		"src/AppMain.go":  "",
	})

	// Lowercase query should match uppercase filenames.
	resp, err := searchInDir(context.Background(), root, "readme", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected 'readme' to match 'README.md' (case-insensitive)")
	}
	if resp.Results[0].Name != "README.md" {
		t.Errorf("expected README.md, got %s", resp.Results[0].Name)
	}

	// Uppercase query should match lowercase filenames.
	resp, err = searchInDir(context.Background(), root, "MAKEFILE", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected 'MAKEFILE' to match 'Makefile' (case-insensitive)")
	}

	// Mixed case query.
	resp, err = searchInDir(context.Background(), root, "AppMain", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected 'AppMain' to match 'AppMain.go' (case-insensitive)")
	}
}

func TestSearch_SortTiebreaker(t *testing.T) {
	t.Run("shallower depth wins", func(t *testing.T) {
		root := createTree(t, map[string]string{
			"a/main.go":     "",
			"a/b/c/main.go": "",
		})

		resp, err := searchInDir(context.Background(), root, "main", "file", 20, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Count < 2 {
			t.Fatalf("expected at least 2 results, got %d", resp.Count)
		}
		// Shallower file (depth 1) should rank above deeper file (depth 3).
		if strings.Count(resp.Results[0].Path, string(filepath.Separator)) >
			strings.Count(resp.Results[1].Path, string(filepath.Separator)) {
			t.Errorf("expected shallower path first: %q vs %q",
				resp.Results[0].Path, resp.Results[1].Path)
		}
	})

	t.Run("depth beats path length", func(t *testing.T) {
		// Both paths are 14 chars with "main" at the same index, so fuzzy
		// scores are equal. Directory chars (x) don't overlap with query
		// chars (m,a,i,n), preventing the fuzzy algorithm from finding
		// alternate match paths. Depth 1 should beat depth 2.
		root := createTree(t, map[string]string{
			"xxxxxx/main.go": "", // depth 1
			"xx/xxx/main.go": "", // depth 2
		})

		resp, err := searchInDir(context.Background(), root, "main", "file", 20, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Count < 2 {
			t.Fatalf("expected at least 2 results, got %d", resp.Count)
		}
		first := resp.Results[0].Path
		if !strings.Contains(first, "xxxxxx") {
			t.Errorf("expected depth-1 result first, got %q", first)
		}
	})

	t.Run("lexical tiebreak for determinism", func(t *testing.T) {
		// Same depth, same rel-path length, "main" at the same index.
		// Directory chars (x, z) don't overlap with query chars.
		// Lexical ordering of rel path should break the tie.
		root := createTree(t, map[string]string{
			"zzzz/main.go": "",
			"xxxx/main.go": "",
		})

		resp, err := searchInDir(context.Background(), root, "main", "file", 20, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Count < 2 {
			t.Fatalf("expected at least 2 results, got %d", resp.Count)
		}
		// "xxxx/main.go" < "zzzz/main.go" lexically.
		if !strings.Contains(resp.Results[0].Path, "xxxx") {
			t.Errorf("expected lexically-first path (xxxx/) first, got %q", resp.Results[0].Path)
		}
	})
}

func TestSearch_BasenameMatchRanking(t *testing.T) {
	// Regression: searching for "argus" from $HOME should rank the project
	// directory (basename "argus") above config directories like ".argus",
	// even though the fuzzy library's greedy matcher mis-aligns characters
	// across path components in longer paths (e.g. Workspace/repos/bxnlabs/argus).
	root := createTree(t, map[string]string{
		".argus/":                                  "",
		".argus/ignore":                            "",
		"Workspace/repos/bxnlabs/argus/":           "",
		"Workspace/repos/bxnlabs/argus/go.mod":     "",
		"Workspace/repos/bxnlabs/argus/main.go":    "",
		"Workspace/repos/bxnlabs/argus/cmd/argus/": "",
	})

	resp, err := searchInDir(context.Background(), root, "argus", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected results")
	}

	// The first result must be an exact basename match for "argus".
	if resp.Results[0].Name != "argus" {
		t.Errorf("expected first result basename to be %q, got %q (%s)",
			"argus", resp.Results[0].Name, resp.Results[0].Path)
	}

	// The project root should rank above the nested cmd/argus/ directory
	// (both are exact matches, but the project root has a shorter path).
	var projectIdx, cmdIdx int = -1, -1
	for i, r := range resp.Results {
		if r.Name == "argus" && !strings.Contains(r.Path, "cmd") {
			projectIdx = i
		}
		if r.Name == "argus" && strings.Contains(r.Path, "cmd") {
			cmdIdx = i
		}
	}
	if projectIdx == -1 || cmdIdx == -1 {
		t.Fatalf("expected both argus directories; projectIdx=%d cmdIdx=%d, results=%+v",
			projectIdx, cmdIdx, resp.Results)
	}
	if projectIdx > cmdIdx {
		t.Errorf("project root (idx %d) should rank above cmd/argus/ (idx %d)",
			projectIdx, cmdIdx)
	}

	// ".argus" (substring match in basename) should rank below exact matches.
	var dotArgusIdx int = -1
	for i, r := range resp.Results {
		if r.Name == ".argus" {
			dotArgusIdx = i
		}
	}
	if dotArgusIdx != -1 && dotArgusIdx < projectIdx {
		t.Errorf(".argus (idx %d) should rank below exact match argus (idx %d)",
			dotArgusIdx, projectIdx)
	}
}

func TestBasenameMatchTier(t *testing.T) {
	tests := []struct {
		name       string
		lowerQuery string
		want       int
	}{
		{"argus", "argus", 0},         // exact
		{"Argus", "argus", 0},         // exact (case-insensitive)
		{"main.go", "main", 1},        // prefix
		{"Main.go", "main", 1},        // prefix (case-insensitive)
		{".argus", "argus", 2},        // substring
		{"my-argus-lib", "argus", 2},  // substring
		{"operations.go", "argus", 3}, // no match
		{"ignore", "argus", 3},        // no match
	}
	for _, tt := range tests {
		got := basenameMatchTier(tt.name, tt.lowerQuery)
		if got != tt.want {
			t.Errorf("basenameMatchTier(%q, %q) = %d, want %d", tt.name, tt.lowerQuery, got, tt.want)
		}
	}
}

func TestSearch_RelativePathsCorrect(t *testing.T) {
	// Ensure results have correct relative paths used for fuzzy matching,
	// even with nested search roots.
	root := createTree(t, map[string]string{
		"src/main.go":            "",
		"src/lib/utils.go":      "",
		"src/lib/deep/inner.go": "",
	})

	resp, err := searchInDir(context.Background(), root, "utils", "file", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected results")
	}
	// The result path should be absolute and contain the expected file.
	found := false
	for _, r := range resp.Results {
		if r.Name == "utils.go" {
			found = true
			if !strings.HasPrefix(r.Path, root) {
				t.Errorf("result path %q should start with root %q", r.Path, root)
			}
		}
	}
	if !found {
		t.Error("expected to find utils.go")
	}
}

func TestSearch_CrossPathComponents(t *testing.T) {
	root := createTree(t, map[string]string{
		"internal/agent/filesearch/operations.go": "",
		"internal/agent/filesearch/types.go":      "",
		"internal/server/handler.go":              "",
		"web/src/components/Terminal/index.tsx":    "",
	})

	// Query spanning directory + filename components.
	resp, err := searchInDir(context.Background(), root, "internalsearch", "", 20, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count == 0 {
		t.Fatal("expected results for query spanning path components 'internalsearch'")
	}
	// Files under internal/agent/filesearch/ should rank highest.
	top := resp.Results[0]
	if !strings.Contains(top.Path, "filesearch") {
		t.Errorf("expected top result under filesearch/, got %s", top.Path)
	}
}

func TestSearch_RespectsContextCancellation(t *testing.T) {
	// Create enough files to make the walk non-trivial.
	tree := map[string]string{}
	for i := 0; i < 100; i++ {
		tree[filepath.Join("dir", fmt.Sprintf("file%03d.txt", i))] = ""
	}
	root := createTree(t, tree)

	// Run a baseline search to know how many results a full walk returns.
	baseline, err := searchInDir(context.Background(), root, "file", "", 100, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Count == 0 {
		t.Fatal("expected baseline results")
	}

	// Cancel context immediately, then search again.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	resp, err := searchInDir(ctx, root, "file", "", 100, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// A pre-cancelled context should return strictly fewer results than a
	// full walk. The exact count depends on timing, but it must not return
	// the complete set.
	if resp.Count >= baseline.Count {
		t.Errorf("cancelled search returned %d results (same as baseline %d); cancellation had no effect",
			resp.Count, baseline.Count)
	}
}

func TestCachedIgnoreMatcher(t *testing.T) {
	t.Cleanup(resetMatcherCache)

	home := t.TempDir()
	path, err := ensureIgnoreFile(home)
	if err != nil {
		t.Fatal(err)
	}

	// First load — should compile and cache.
	m1 := loadIgnoreMatcher(path)
	if m1 == nil {
		t.Fatal("expected non-nil matcher")
	}

	// Second load — should return cached (same pointer).
	m2 := loadIgnoreMatcher(path)
	if m2 != m1 {
		t.Error("expected cached matcher (same pointer) on second load")
	}

	// Modify the file — should recompile.
	if err := os.WriteFile(path, []byte("newpattern/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Force a different mtime (filesystem precision may not capture sub-second changes).
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	m3 := loadIgnoreMatcher(path)
	if m3 == m1 {
		t.Error("expected new matcher after file modification")
	}
}
