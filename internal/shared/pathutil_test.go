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
			name:      "second stage with tilde prefix",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/very-long-project-name",
			home:      home,
			threshold: 20,
			want:      "~/.../bxnlabs/very-long-project-name",
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
