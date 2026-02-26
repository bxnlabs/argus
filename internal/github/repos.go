package github

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v69/github"
	"github.com/sahilm/fuzzy"
)

// RepoService lists and searches GitHub repositories for the authenticated user.
type RepoService struct {
	token string

	mu        sync.Mutex
	cached    []string
	fetchedAt time.Time
}

const cacheTTL = 5 * time.Minute

// NewRepoService creates a new RepoService with the given GitHub token.
func NewRepoService(token string) *RepoService {
	return &RepoService{token: token}
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

	client := gh.NewClient(nil).WithAuthToken(s.token)

	var allRepos []string

	// Fetch user's repos (includes personal + repos with push access)
	opts := &gh.RepositoryListByAuthenticatedUserOptions{
		Sort:        "full_name",
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	for {
		repos, resp, err := client.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("listing user repos: %w", err)
		}
		for _, r := range repos {
			allRepos = append(allRepos, r.GetFullName())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Fetch org repos
	orgs, _, err := client.Organizations.List(ctx, "", &gh.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("listing orgs: %w", err)
	}
	for _, org := range orgs {
		orgOpts := &gh.RepositoryListByOrgOptions{
			Sort:        "full_name",
			ListOptions: gh.ListOptions{PerPage: 100},
		}
		for {
			repos, resp, err := client.Repositories.ListByOrg(ctx, org.GetLogin(), orgOpts)
			if err != nil {
				return nil, fmt.Errorf("listing repos for org %s: %w", org.GetLogin(), err)
			}
			for _, r := range repos {
				allRepos = append(allRepos, r.GetFullName())
			}
			if resp.NextPage == 0 {
				break
			}
			orgOpts.Page = resp.NextPage
		}
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
