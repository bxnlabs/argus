package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sahilm/fuzzy"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// RepoIndexer maintains an in-memory snapshot of the user's GitHub repos,
// refreshed periodically in the background. Search() reads from the snapshot
// without blocking on network calls.
type RepoIndexer struct {
	stateDir string

	mu        sync.RWMutex
	repos     []string
	fetchedAt time.Time

	group     singleflight.Group
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once

	// fetchFunc is the function used to fetch repos. Defaults to fetchAll
	// and can be overridden for testing.
	fetchFunc func(ctx context.Context) ([]string, error)
	retryCfg  retryConfig // retry policy; tests override delay
}

const (
	refreshInterval = 5 * time.Minute
	snapshotSubdir  = "data"
	snapshotFile    = "repos.json"
)

// repoSnapshot is the on-disk JSON format.
type repoSnapshot struct {
	Repos     []string  `json:"repos"`
	FetchedAt time.Time `json:"fetched_at"`
}

// NewRepoIndexer creates a new RepoIndexer. Call Start() to begin background
// refreshes and Close() to stop.
func NewRepoIndexer(stateDir string) *RepoIndexer {
	return &RepoIndexer{
		stateDir:  stateDir,
		fetchFunc: fetchAll,
	}
}

// Start loads the disk snapshot (if any) and begins periodic background
// refreshes. It is safe to call multiple times; only the first call has
// any effect.
func (idx *RepoIndexer) Start(ctx context.Context) {
	idx.startOnce.Do(func() {
		idx.loadSnapshot()

		ctx, idx.cancel = context.WithCancel(ctx)
		idx.wg.Add(1)
		go idx.loop(ctx)
	})
}

// Close cancels background refreshes and waits for the goroutine to exit.
func (idx *RepoIndexer) Close() {
	if idx.cancel != nil {
		idx.cancel()
	}
	idx.wg.Wait()
}

// Search returns repos matching the query from the current in-memory snapshot.
// It never blocks on a network fetch.
func (idx *RepoIndexer) Search(query string) []string {
	idx.mu.RLock()
	repos := idx.repos
	idx.mu.RUnlock()

	if len(repos) == 0 {
		return []string{}
	}
	if query == "" {
		return append([]string(nil), repos...)
	}
	return fuzzyFilterRepos(repos, query)
}

func (idx *RepoIndexer) loop(ctx context.Context) {
	defer idx.wg.Done()

	// Refresh immediately on start.
	idx.refresh(ctx)

	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			idx.refresh(ctx)
		}
	}
}

// retryConfig holds parameters for the retry loop.
type retryConfig struct {
	attempts int
	delay    time.Duration
	maxDelay time.Duration
}

// defaultRetryCfg is the baseline retry policy for refresh.
var defaultRetryCfg = retryConfig{
	attempts: 3,
	delay:    2 * time.Second,
	maxDelay: 10 * time.Second,
}

// permanentError wraps an error that should not be retried.
type permanentError struct{ error }

func (e permanentError) Unwrap() error { return e.error }

// retryWithBackoff runs fn up to cfg.attempts times with exponential backoff.
// It stops immediately on context cancellation or permanentError.
func retryWithBackoff(ctx context.Context, cfg retryConfig, fn func() error) error {
	c := cfg
	if c.attempts <= 0 {
		c.attempts = defaultRetryCfg.attempts
	}
	if c.delay <= 0 {
		c.delay = defaultRetryCfg.delay
	}
	if c.maxDelay <= 0 {
		c.maxDelay = defaultRetryCfg.maxDelay
	}

	var lastErr error
	for i := range c.attempts {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		var perm permanentError
		if errors.As(lastErr, &perm) {
			return perm.error
		}

		// Don't sleep after the last attempt.
		if i < c.attempts-1 {
			backoff := c.delay * time.Duration(1<<uint(i))
			if backoff > c.maxDelay {
				backoff = c.maxDelay
			}
			select {
			case <-ctx.Done():
				return lastErr
			case <-time.After(backoff):
			}
		}
	}
	return lastErr
}

func (idx *RepoIndexer) refresh(ctx context.Context) {
	_, _, _ = idx.group.Do("refresh", func() (any, error) {
		cfg := idx.retryCfg

		var repos []string
		err := retryWithBackoff(ctx, cfg, func() error {
			var fetchErr error
			repos, fetchErr = idx.fetchFunc(ctx)
			return fetchErr
		})

		if err != nil {
			log.Printf("repo indexer: refresh failed: %v", err)
			return nil, nil
		}

		idx.mu.Lock()
		idx.repos = repos
		idx.fetchedAt = time.Now()
		idx.mu.Unlock()

		idx.saveSnapshot(repos)
		return nil, nil
	})
}

// fetchAll fetches user repos and org repos in parallel via the gh CLI.
func fetchAll(ctx context.Context) ([]string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, permanentError{fmt.Errorf("gh CLI not found: %w", err)}
	}

	// Fetch user repos and org list concurrently.
	// errgroup cancels gctx on first error so the other call is cleaned up.
	var userLines, orgNames []string
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		lines, err := ghAPILines(gctx, "user/repos?per_page=100", ".[].full_name")
		if err != nil {
			return fmt.Errorf("listing user repos: %w", err)
		}
		userLines = lines
		return nil
	})
	g.Go(func() error {
		lines, err := ghAPILines(gctx, "user/orgs?per_page=100", ".[].login")
		if err != nil {
			return fmt.Errorf("listing orgs: %w", err)
		}
		orgNames = lines
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	allRepos := userLines

	// Fan out org repo fetches with bounded concurrency.
	if len(orgNames) > 0 {
		type result struct {
			lines []string
			err   error
		}
		orgResults := make([]result, len(orgNames))
		og, ogctx := errgroup.WithContext(ctx)
		og.SetLimit(8)
		for i, org := range orgNames {
			og.Go(func() error {
				lines, err := ghAPILines(ogctx, fmt.Sprintf("orgs/%s/repos?per_page=100", org), ".[].full_name")
				orgResults[i] = result{lines, err}
				return nil // errors collected per-org
			})
		}
		og.Wait()

		for i, r := range orgResults {
			if r.err != nil {
				return nil, fmt.Errorf("listing repos for org %s: %w", orgNames[i], r.err)
			}
			allRepos = append(allRepos, r.lines...)
		}
	}

	// Deduplicate and sort.
	seen := make(map[string]bool, len(allRepos))
	deduped := make([]string, 0, len(allRepos))
	for _, r := range allRepos {
		if !seen[r] {
			seen[r] = true
			deduped = append(deduped, r)
		}
	}
	sort.Strings(deduped)
	return deduped, nil
}

// --- Disk snapshot ---

func (idx *RepoIndexer) snapshotPath() string {
	return filepath.Join(idx.stateDir, snapshotSubdir, snapshotFile)
}

func (idx *RepoIndexer) loadSnapshot() {
	data, err := os.ReadFile(idx.snapshotPath())
	if err != nil {
		return
	}
	var snap repoSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("repo indexer: corrupt snapshot, ignoring: %v", err)
		return
	}
	idx.mu.Lock()
	idx.repos = snap.Repos
	idx.fetchedAt = snap.FetchedAt
	idx.mu.Unlock()
	log.Printf("repo indexer: loaded %d repos from snapshot (age %s)",
		len(snap.Repos), time.Since(snap.FetchedAt).Round(time.Second))
}

func (idx *RepoIndexer) saveSnapshot(repos []string) {
	snap := repoSnapshot{Repos: repos, FetchedAt: time.Now()}
	data, err := json.Marshal(snap)
	if err != nil {
		log.Printf("repo indexer: snapshot marshal: %v", err)
		return
	}
	dir := filepath.Dir(idx.snapshotPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("repo indexer: mkdir %s: %v", dir, err)
		return
	}
	if err := os.WriteFile(idx.snapshotPath(), data, 0o600); err != nil {
		log.Printf("repo indexer: write snapshot: %v", err)
	}
}

// --- gh CLI helper ---

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

// --- Fuzzy search ---

// fuzzyFilterRepos applies fuzzy matching on repo full names.
func fuzzyFilterRepos(repos []string, query string) []string {
	if query == "" {
		return nil
	}
	src := repoSource(repos)
	matches := fuzzy.FindFromNoSort(query, src)
	sort.SliceStable(matches, func(i, j int) bool {
		ii, jj := matches[i].Index, matches[j].Index
		ti := repoNameTier(repos[ii], query)
		tj := repoNameTier(repos[jj], query)
		if ti != tj {
			return ti < tj
		}
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
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
		return 0
	case strings.HasPrefix(name, q):
		return 1
	case strings.Contains(name, q):
		return 2
	default:
		return 3
	}
}
