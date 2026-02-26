package worktree

import "testing"

func TestSlugify(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Fix Auth Bug!", "fix-auth-bug"},
		{"  my feature  ", "my-feature"},
		{"already-valid", "already-valid"},
		{"123abc", "123abc"},
		{"UPPER CASE", "upper-case"},
		{"multiple   spaces", "multiple-spaces"},
		{"a--b", "a-b"},
		{"!!!!", "session"},
		{"", "session"},
	}
	for _, tc := range cases {
		got := slugify(tc.input)
		if got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestWorktreeDirName(t *testing.T) {
	cases := []struct {
		branch string
		want   string
	}{
		{"jeev/fix-auth-bug", "jeev--fix-auth-bug"},
		{"fix-auth-bug", "fix-auth-bug"},
		{"prefix/org/feature", "prefix--org--feature"},
	}
	for _, tc := range cases {
		got := worktreeDirName(tc.branch)
		if got != tc.want {
			t.Errorf("worktreeDirName(%q) = %q, want %q", tc.branch, got, tc.want)
		}
	}
}
