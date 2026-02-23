package search

import (
	"strings"
	"testing"
)

func TestParseRgJSON(t *testing.T) {
	t.Run("match entries", func(t *testing.T) {
		output := `{"type":"begin","data":{"path":{"text":"/tmp/proj/test.go"}}}
{"type":"match","data":{"path":{"text":"/tmp/proj/test.go"},"lines":{"text":"func main() {\n"},"line_number":1,"absolute_offset":0,"submatches":[{"match":{"text":"main"},"start":5,"end":9}]}}
{"type":"match","data":{"path":{"text":"/tmp/proj/other.go"},"lines":{"text":"package main\n"},"line_number":1,"absolute_offset":0,"submatches":[{"match":{"text":"main"},"start":8,"end":12}]}}
{"type":"end","data":{"path":{"text":"/tmp/proj/test.go"}}}`

		matches, err := parseRgJSON(output, "/tmp/proj", 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 2 {
			t.Fatalf("matches = %d, want 2", len(matches))
		}

		// First match
		m := matches[0]
		if m.File != "test.go" {
			t.Errorf("file = %q, want %q", m.File, "test.go")
		}
		if m.Line != 1 {
			t.Errorf("line = %d, want 1", m.Line)
		}
		if m.Column != 5 {
			t.Errorf("column = %d, want 5", m.Column)
		}
		if m.MatchText != "main" {
			t.Errorf("matchText = %q, want %q", m.MatchText, "main")
		}
		if m.LineText != "func main() {" {
			t.Errorf("lineText = %q, want %q", m.LineText, "func main() {")
		}

		// Second match — different file
		if matches[1].File != "other.go" {
			t.Errorf("file = %q, want %q", matches[1].File, "other.go")
		}
	})

	t.Run("empty output", func(t *testing.T) {
		matches, err := parseRgJSON("", "/tmp", 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Errorf("matches = %d, want 0", len(matches))
		}
	})

	t.Run("malformed line skipped", func(t *testing.T) {
		output := `{"type":"match","bad json
{"type":"match","data":{"path":{"text":"/tmp/test.go"},"lines":{"text":"valid line\n"},"line_number":5,"submatches":[{"match":{"text":"valid"},"start":0,"end":5}]}}`

		matches, err := parseRgJSON(output, "/tmp", 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Errorf("matches = %d, want 1 (skip bad line)", len(matches))
		}
	})

	t.Run("caps at maxResults", func(t *testing.T) {
		lines := []string{}
		for i := 0; i < 10; i++ {
			lines = append(lines, `{"type":"match","data":{"path":{"text":"/tmp/f.go"},"lines":{"text":"line\n"},"line_number":1,"submatches":[]}}`)
		}
		output := strings.Join(lines, "\n")

		matches, err := parseRgJSON(output, "/tmp", 3)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 3 {
			t.Errorf("matches = %d, want 3 (capped)", len(matches))
		}
	})

	t.Run("returns partial results on scanner buffer overflow", func(t *testing.T) {
		// First line is a valid match, second line exceeds the 1MB scanner buffer.
		validLine := `{"type":"match","data":{"path":{"text":"/tmp/f.go"},"lines":{"text":"good line\n"},"line_number":1,"submatches":[{"match":{"text":"good"},"start":0,"end":4}]}}`
		longLine := `{"type":"match","data":{"path":{"text":"/tmp/f.go"},"lines":{"text":"` + strings.Repeat("x", 1<<20) + `\n"},"line_number":2,"submatches":[]}}`
		output := validLine + "\n" + longLine

		matches, err := parseRgJSON(output, "/tmp", 100)
		if err == nil {
			t.Error("expected scanner error for oversized line")
		}
		// Partial results from before the overflow should still be returned.
		if len(matches) != 1 {
			t.Errorf("matches = %d, want 1 (partial results before overflow)", len(matches))
		}
	})

	t.Run("no submatches", func(t *testing.T) {
		output := `{"type":"match","data":{"path":{"text":"/tmp/f.go"},"lines":{"text":"hello\n"},"line_number":1,"submatches":[]}}`
		matches, err := parseRgJSON(output, "/tmp", 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Fatal("expected 1 match")
		}
		if matches[0].Column != 0 {
			t.Errorf("column = %d, want 0 for no submatches", matches[0].Column)
		}
		if matches[0].MatchText != "" {
			t.Errorf("matchText = %q, want empty for no submatches", matches[0].MatchText)
		}
	})
}

func TestMakeRelative(t *testing.T) {
	tests := []struct {
		name, path, base, want string
	}{
		{"simple", "/tmp/proj/file.go", "/tmp/proj", "file.go"},
		{"nested", "/tmp/proj/src/main.go", "/tmp/proj", "src/main.go"},
		{"unrelated fallback", "/other/file.go", "/tmp/proj", "../../other/file.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeRelative(tt.path, tt.base)
			if got != tt.want {
				t.Errorf("makeRelative(%q, %q) = %q, want %q", tt.path, tt.base, got, tt.want)
			}
		})
	}
}

func TestIsAvailable(t *testing.T) {
	// Just verify it returns a boolean without panicking.
	available := IsAvailable()
	t.Logf("ripgrep available: %v", available)
}

func TestSearch(t *testing.T) {
	if !IsAvailable() {
		t.Skip("ripgrep not installed")
	}

	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "package main\n\nfunc hello() {}\nfunc world() {}\n")
	writeTestFile(t, dir, "lib.go", "package main\n\nfunc helper() {}\n")

	t.Run("finds matches", func(t *testing.T) {
		result, err := Search(dir, "func", 100)
		if err != nil {
			t.Fatal(err)
		}
		if result.Count == 0 {
			t.Error("expected matches")
		}
		if result.Query != "func" {
			t.Errorf("query = %q, want %q", result.Query, "func")
		}
		if result.Path != dir {
			t.Errorf("path = %q, want %q", result.Path, dir)
		}
		if result.Count != len(result.Results) {
			t.Errorf("count %d != len(results) %d", result.Count, len(result.Results))
		}
		// Check a match has expected fields
		m := result.Results[0]
		if m.File == "" {
			t.Error("expected non-empty file")
		}
		if m.Line == 0 {
			t.Error("expected non-zero line number")
		}
	})

	t.Run("no matches", func(t *testing.T) {
		result, err := Search(dir, "xyznonexistent999", 100)
		if err != nil {
			t.Fatal(err)
		}
		if result.Count != 0 {
			t.Errorf("count = %d, want 0", result.Count)
		}
		if result.Results == nil {
			t.Error("expected non-nil empty slice")
		}
	})

	t.Run("maxResults cap", func(t *testing.T) {
		result, err := Search(dir, "func", 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Results) > 2 {
			t.Errorf("results = %d, want <= 2", len(result.Results))
		}
	})

	t.Run("relative paths", func(t *testing.T) {
		result, err := Search(dir, "func", 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range result.Results {
			if strings.HasPrefix(m.File, "/") {
				t.Errorf("file path should be relative, got %q", m.File)
			}
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		result, err := Search(dir, "FUNC", 100)
		if err != nil {
			t.Fatal(err)
		}
		if result.Count == 0 {
			t.Error("expected case-insensitive matches")
		}
	})
}
