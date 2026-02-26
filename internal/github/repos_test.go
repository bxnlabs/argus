package github

import (
	"testing"
)

func TestFuzzySearch(t *testing.T) {
	repos := []string{
		"bxnlabs/argus",
		"bxnlabs/infra",
		"bxnlabs/sdk",
		"myorg/backend",
		"myorg/frontend",
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"arg", []string{"bxnlabs/argus"}},
		{"bxn", []string{"bxnlabs/argus", "bxnlabs/infra", "bxnlabs/sdk"}},
		{"front", []string{"myorg/frontend"}},
		{"", nil}, // empty query returns nil (caller should return full list)
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := fuzzyFilterRepos(repos, tt.query)
			if len(got) != len(tt.want) {
				t.Errorf("fuzzyFilterRepos(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}
