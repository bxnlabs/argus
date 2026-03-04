package shared

import "testing"

func TestCompressPath(t *testing.T) {
	home := "/Users/jeevb"
	tests := []struct {
		name      string
		path      string
		home      string
		threshold int
		want      string
	}{
		{
			name:      "short path unchanged",
			path:      "/tmp/project",
			home:      home,
			threshold: 40,
			want:      "/tmp/project",
		},
		{
			name:      "tilde replaces home prefix",
			path:      "/Users/jeevb/project",
			home:      home,
			threshold: 40,
			want:      "~/project",
		},
		{
			name:      "home dir itself",
			path:      "/Users/jeevb",
			home:      home,
			threshold: 40,
			want:      "~",
		},
		{
			name:      "long path compressed",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			home:      home,
			threshold: 30,
			want:      "~/Workspace/.../bxnlabs/argus",
		},
		{
			name:      "non-home long path compressed",
			path:      "/opt/data/very/deep/nested/project",
			home:      home,
			threshold: 25,
			want:      "/opt/.../nested/project",
		},
		{
			name:      "second stage drops first segment",
			path:      "/opt/data/very/deep/nested/project",
			home:      home,
			threshold: 20,
			want:      "/.../nested/project",
		},
		{
			name:      "drops parent to preserve basename",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/very-long-project-name",
			home:      home,
			threshold: 20,
			want:      "~/.../very-long-pro…",
		},
		{
			name:      "three_segment_drops_middle_to_preserve_basename",
			path:      "/Users/jeevb/Workspace/long-parent/project",
			home:      home,
			threshold: 20,
			want:      "~/.../project",
		},
		{
			name:      "three_segment_fallback_when_basename_too_long",
			path:      "/Users/jeevb/a/b/very-long-basename",
			home:      home,
			threshold: 15,
			want:      "~/a/b/very-lon…",
		},
		{
			name:      "three segments no compression needed",
			path:      "/Users/jeevb/project",
			home:      home,
			threshold: 10,
			want:      "~/project",
		},
		{
			name:      "exactly at threshold no compression",
			path:      "/Users/jeevb/short",
			home:      home,
			threshold: 7,
			want:      "~/short",
		},
		{
			name:      "deep path with long tail preserves basename",
			path:      "/Users/jeevb/a/b/very-very-very-long-segment/another-very-very-long-segment",
			home:      home,
			threshold: 30,
			want:      "~/.../another-very-very-long-…",
		},
		{
			name:      "shallow path truncated when over threshold",
			path:      "/Users/jeevb/very-long-directory-name",
			home:      home,
			threshold: 15,
			want:      "~/very-long-di…",
		},
		{
			name:      "empty home falls back",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			home:      "",
			threshold: 30,
			want:      "/Users/.../bxnlabs/argus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompressPath(tt.path, tt.home, tt.threshold)
			if got != tt.want {
				t.Errorf("CompressPath(%q, %q, %d) = %q, want %q",
					tt.path, tt.home, tt.threshold, got, tt.want)
			}
		})
	}
}

func TestTruncateRight(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"within limit", "main", 20, "main"},
		{"at limit", "abcde", 5, "abcde"},
		{"over limit", "abcdefghij", 5, "abcd…"},
		{"max 1", "abcde", 1, "…"},
		{"max 0 returns empty", "abcde", 0, ""},
		{"negative max returns empty", "abcde", -1, ""},
		{"unicode runes", "日本語テスト", 4, "日本語…"},
		{"empty", "", 10, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TruncateRight(tt.s, tt.max); got != tt.want {
				t.Errorf("TruncateRight(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}
