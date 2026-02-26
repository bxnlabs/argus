package github

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sahilm/fuzzy"
)

// RepoService lists and searches GitHub repositories using the gh CLI.
// If gh is not installed or not authenticated, operations return empty results.
type RepoService struct {
	mu        sync.Mutex
	cached    []string
	fetchedAt time.Time
}

const cacheTTL = 5 * time.Minute

// NewRepoService creates a new RepoService.
func NewRepoService() *RepoService {
	return &RepoService{}
}

// Search returns repos matching the query. If query is empty, returns all repos.
// Results are cached for 5 minutes.
func (s *RepoService) Search(ctx context.Context, query string) ([]string, error) {
	repos, err := s.listAll(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return repos, nil
	}
	return fuzzyFilterRepos(repos, query), nil
}

func (s *RepoService) listAll(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	if s.cached != nil && time.Since(s.fetchedAt) < cacheTTL {
		cached := s.cached
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, nil
	}

	// Detach from the HTTP request context so client disconnects don't
	// kill the gh subprocess mid-pagination. The cache means subsequent
	// requests are instant.
	ctx = context.WithoutCancel(ctx)

	var allRepos []string

	// Fetch user's repos
	userRepos, err := ghAPILines(ctx, "user/repos", ".[].full_name")
	if err != nil {
		return nil, fmt.Errorf("listing user repos: %w", err)
	}
	allRepos = append(allRepos, userRepos...)

	// Fetch orgs, then each org's repos
	orgs, err := ghAPILines(ctx, "user/orgs", ".[].login")
	if err != nil {
		return nil, fmt.Errorf("listing orgs: %w", err)
	}
	for _, org := range orgs {
		orgRepos, err := ghAPILines(ctx, fmt.Sprintf("orgs/%s/repos", org), ".[].full_name")
		if err != nil {
			return nil, fmt.Errorf("listing repos for org %s: %w", org, err)
		}
		allRepos = append(allRepos, orgRepos...)
	}

	// Deduplicate (user repos may overlap with org repos)
	seen := make(map[string]bool, len(allRepos))
	deduped := allRepos[:0]
	for _, r := range allRepos {
		if !seen[r] {
			seen[r] = true
			deduped = append(deduped, r)
		}
	}
	sort.Strings(deduped)

	s.mu.Lock()
	s.cached = deduped
	s.fetchedAt = time.Now()
	s.mu.Unlock()

	return deduped, nil
}

// ghAPILines calls `gh api --paginate <endpoint> --jq <jqExpr>` and returns
// the non-empty lines of output.
func ghAPILines(ctx context.Context, endpoint, jqExpr string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "gh", "api", "--paginate", endpoint, "--jq", jqExpr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w (stderr: %s)", endpoint, err, strings.TrimSpace(stderr.String()))
	}
	var lines []string
	for _, l := range strings.Split(stdout.String(), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

// fuzzyFilterRepos applies fuzzy matching on repo full names.
func fuzzyFilterRepos(repos []string, query string) []string {
	if query == "" {
		return nil
	}
	src := repoSource(repos)
	matches := fuzzy.FindFromNoSort(query, src)
	sort.SliceStable(matches, func(i, j int) bool {
		ii, jj := matches[i].Index, matches[j].Index
		// Tier 1: prefer matches where the repo name (after /) matches
		ti := repoNameTier(repos[ii], query)
		tj := repoNameTier(repos[jj], query)
		if ti != tj {
			return ti < tj
		}
		// Tier 2: fuzzy score (descending)
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		// Tier 3: lexical
		return repos[ii] < repos[jj]
	})
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = repos[m.Index]
	}
	return result
}

type repoSource []string

func (s repoSource) String(i int) string { return s[i] }
func (s repoSource) Len() int            { return len(s) }

// repoNameTier returns a ranking tier based on how the query matches the repo
// name (the part after the /). Lower is better.
func repoNameTier(fullName, query string) int {
	q := strings.ToLower(query)
	_, name, _ := strings.Cut(fullName, "/")
	name = strings.ToLower(name)
	switch {
	case name == q:
		return 0 // exact
	case strings.HasPrefix(name, q):
		return 1 // prefix
	case strings.Contains(name, q):
		return 2 // substring
	default:
		return 3 // path-only
	}
}
