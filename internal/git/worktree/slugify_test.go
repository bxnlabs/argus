package worktree

import "testing"

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
