package cli

import "testing"

func TestParseRepo(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"scp-ssh", "git@github.com:bxnlabs/argus.git", "bxnlabs/argus"},
		{"scp-ssh no suffix", "git@github.com:bxnlabs/argus", "bxnlabs/argus"},
		{"https", "https://github.com/bxnlabs/argus.git", "bxnlabs/argus"},
		{"https no suffix", "https://github.com/bxnlabs/argus", "bxnlabs/argus"},
		{"subgroup", "https://gitlab.com/group/sub/proj.git", "group/sub/proj"},
		{"empty", "", ""},
		{"garbage", "not a url", ""},
		{"ssh scheme", "ssh://git@github.com/bxnlabs/argus.git", "bxnlabs/argus"},
		{"scp empty path", "git@github.com:", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRepo(tt.url); got != tt.want {
				t.Errorf("parseRepo(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
