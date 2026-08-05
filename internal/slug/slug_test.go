package slug

import "testing"

func TestMake(t *testing.T) {
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
		got := Make(tc.input)
		if got != tc.want {
			t.Errorf("Make(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
