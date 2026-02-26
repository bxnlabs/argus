package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestSnapshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	idx := NewRepoIndexer(dir)

	repos := []string{"org/alpha", "org/beta", "org/gamma"}
	idx.saveSnapshot(repos)

	idx2 := NewRepoIndexer(dir)
	idx2.loadSnapshot()

	got := idx2.Search("")
	if len(got) != len(repos) {
		t.Fatalf("got %d repos, want %d", len(got), len(repos))
	}
	for i, r := range repos {
		if got[i] != r {
			t.Errorf("got[%d] = %q, want %q", i, got[i], r)
		}
	}
}

func TestSnapshotFileFormat(t *testing.T) {
	dir := t.TempDir()
	idx := NewRepoIndexer(dir)
	idx.saveSnapshot([]string{"a/b", "c/d"})

	data, err := os.ReadFile(filepath.Join(dir, "data", "repos.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snap repoSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(snap.Repos) != 2 {
		t.Errorf("got %d repos in snapshot, want 2", len(snap.Repos))
	}
	if snap.FetchedAt.IsZero() {
		t.Error("fetched_at should not be zero")
	}
}

func TestSnapshotCorrupt(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "repos.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	idx := NewRepoIndexer(dir)
	idx.loadSnapshot()

	got := idx.Search("")
	if len(got) != 0 {
		t.Errorf("expected empty repos after corrupt snapshot, got %d", len(got))
	}
}

func TestSnapshotMissing(t *testing.T) {
	dir := t.TempDir()
	idx := NewRepoIndexer(dir)
	idx.loadSnapshot() // should be a no-op

	got := idx.Search("")
	if len(got) != 0 {
		t.Errorf("expected empty repos when no snapshot, got %d", len(got))
	}
}

func TestSearchNeverBlocks(t *testing.T) {
	idx := NewRepoIndexer(t.TempDir())
	// Don't call Start(). Search must return immediately.
	got := idx.Search("anything")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestSearchFromSnapshot(t *testing.T) {
	dir := t.TempDir()
	idx := NewRepoIndexer(dir)
	idx.saveSnapshot([]string{"bxnlabs/argus", "bxnlabs/infra", "myorg/backend"})

	idx2 := NewRepoIndexer(dir)
	idx2.loadSnapshot()

	got := idx2.Search("argus")
	if len(got) != 1 || got[0] != "bxnlabs/argus" {
		t.Errorf("expected [bxnlabs/argus], got %v", got)
	}
}

func TestCloseWithoutStart(t *testing.T) {
	idx := NewRepoIndexer(t.TempDir())
	idx.Close() // must not panic
}

func TestStartAndClose(t *testing.T) {
	idx := NewRepoIndexer(t.TempDir())
	idx.fetchFunc = func(ctx context.Context) ([]string, error) {
		return []string{"a/b"}, nil
	}
	idx.Start(context.Background())
	time.Sleep(50 * time.Millisecond)
	idx.Close() // goroutine must exit cleanly
}

func TestDoubleStartIsSafe(t *testing.T) {
	calls := 0
	idx := NewRepoIndexer(t.TempDir())
	idx.fetchFunc = func(ctx context.Context) ([]string, error) {
		calls++
		return []string{"a/b"}, nil
	}
	ctx := context.Background()
	idx.Start(ctx)
	idx.Start(ctx) // second call must be a no-op
	time.Sleep(100 * time.Millisecond)
	idx.Close()
	if calls != 1 {
		t.Errorf("expected fetchFunc called once, got %d", calls)
	}
}

func TestRefreshPreservesDataOnFailure(t *testing.T) {
	dir := t.TempDir()
	idx := NewRepoIndexer(dir)

	// Seed with known data.
	idx.mu.Lock()
	idx.repos = []string{"org/alpha", "org/beta"}
	idx.mu.Unlock()
	idx.saveSnapshot([]string{"org/alpha", "org/beta"})

	// Override fetch to fail.
	idx.fetchFunc = func(ctx context.Context) ([]string, error) {
		return nil, fmt.Errorf("gh CLI not found")
	}

	// Trigger refresh — should preserve old data.
	idx.refresh(context.Background())

	got := idx.Search("")
	if len(got) != 2 {
		t.Fatalf("expected 2 repos preserved after failed refresh, got %d", len(got))
	}
	if got[0] != "org/alpha" || got[1] != "org/beta" {
		t.Errorf("unexpected repos: %v", got)
	}
}

func TestRefreshUpdatesDataOnSuccess(t *testing.T) {
	dir := t.TempDir()
	idx := NewRepoIndexer(dir)

	// Seed with old data.
	idx.mu.Lock()
	idx.repos = []string{"old/repo"}
	idx.mu.Unlock()

	// Override fetch to succeed with new data.
	idx.fetchFunc = func(ctx context.Context) ([]string, error) {
		return []string{"new/alpha", "new/beta"}, nil
	}

	idx.refresh(context.Background())

	got := idx.Search("")
	if len(got) != 2 || got[0] != "new/alpha" || got[1] != "new/beta" {
		t.Errorf("expected [new/alpha new/beta], got %v", got)
	}
}

func TestSearchReturnsDefensiveCopy(t *testing.T) {
	idx := NewRepoIndexer(t.TempDir())
	idx.mu.Lock()
	idx.repos = []string{"a/b", "c/d"}
	idx.mu.Unlock()

	got := idx.Search("")
	got[0] = "mutated"

	// Internal state must be unchanged.
	got2 := idx.Search("")
	if got2[0] != "a/b" {
		t.Errorf("internal state was mutated: got %q, want %q", got2[0], "a/b")
	}
}
