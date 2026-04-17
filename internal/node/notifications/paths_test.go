package notifications

import "testing"

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"https with .git", "https://github.com/bxnlabs/argus.git", "bxnlabs/argus"},
		{"https without .git", "https://github.com/flyteorg/flyte-sdk", "flyteorg/flyte-sdk"},
		{"ssh url", "git@github.com:bxnlabs/argus.git", "bxnlabs/argus"},
		{"single segment", "https://github.com/argus.git", "argus"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoName(tt.input)
			if got != tt.expected {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCompressPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"home path", "/home/jeev/repos/myproject", "~/repos/myproject"},
		{"Users path", "/Users/jeev/Workspace/repos/foo", "~/Workspace/repos/foo"},
		{"no home prefix", "/tmp/foo", "/tmp/foo"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressHomePath(tt.input)
			if got != tt.expected {
				t.Errorf("compressHomePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildLocationLine(t *testing.T) {
	s := func(v string) *string { return &v }

	tests := []struct {
		name                             string
		remoteURL, parentDir, workingDir *string
		branch                           *string
		wantRepo, wantLocal, wantBranch  string
	}{
		{
			name:       "full git metadata",
			remoteURL:  s("https://github.com/bxnlabs/argus.git"),
			parentDir:  s("/home/jeev/repos/argus"),
			workingDir: s("/home/jeev/.argus/projects/foo/worktrees/bar"),
			branch:     s("jeev/feature"),
			wantRepo:   "bxnlabs/argus",
			wantLocal:  "~/repos/argus",
			wantBranch: "jeev/feature",
		},
		{
			name:       "no remote, has parent dir",
			remoteURL:  nil,
			parentDir:  s("/Users/jeev/Workspace/repos/myproject"),
			workingDir: s("/tmp/foo"),
			branch:     nil,
			wantRepo:   "~/Workspace/repos/myproject",
			wantLocal:  "",
			wantBranch: "",
		},
		{
			name:       "only working dir",
			remoteURL:  nil,
			parentDir:  nil,
			workingDir: s("/tmp/project"),
			branch:     nil,
			wantRepo:   "/tmp/project",
			wantLocal:  "",
			wantBranch: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, local, branch := buildLocationLine(tt.remoteURL, tt.parentDir, tt.workingDir, tt.branch)
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if local != tt.wantLocal {
				t.Errorf("local = %q, want %q", local, tt.wantLocal)
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
		})
	}
}
