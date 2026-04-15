package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/status"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// emptyLister returns no sessions so WatcherManager starts with no watchers.
type emptyLister struct{}

func (e *emptyLister) List(ctx context.Context) ([]*db.Session, error) {
	return nil, nil
}

// noopWatcherDB satisfies status.WatcherDB with no-op implementations.
type noopWatcherDB struct{}

func (n *noopWatcherDB) SetUnreadSince(ctx context.Context, id string, ts *string) error {
	return nil
}
func (n *noopWatcherDB) TouchSession(ctx context.Context, id string, unixTS int64) error {
	return nil
}
func (n *noopWatcherDB) GetSession(id string) (unreadSince, lastViewedAt *string, err error) {
	return nil, nil, nil
}

func TestHandleStatus_SessionWithoutSnapshotDefaultsToIdle(t *testing.T) {
	database := testDB(t)

	// Create a session in the DB — no watcher will exist for it.
	err := database.CreateSession(&db.Session{
		ID:           "sess-no-watcher",
		Name:         "No Watcher Session",
		TmuxName:     "claude-no-watcher",
		ProviderType: "claude",
	})
	if err != nil {
		t.Fatal(err)
	}

	// WatcherManager with no watchers running.
	mgr := status.NewWatcherManager(&emptyLister{}, &noopWatcherDB{}, nil)
	mgr.Start(context.Background())
	defer mgr.Close()

	handler := handleStatus(mgr, database)
	req := httptest.NewRequest("GET", "/api/sessions/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Statuses map[string]struct {
			SessionName  string `json:"sessionName"`
			Status       string `json:"status"`
			ProviderType string `json:"providerType"`
		} `json:"statuses"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	entry, ok := resp.Statuses["sess-no-watcher"]
	if !ok {
		t.Fatal("expected sess-no-watcher in response — session exists in DB but has no watcher")
	}
	if entry.Status != "idle" {
		t.Errorf("expected status=idle for session without snapshot, got %q", entry.Status)
	}
	if entry.SessionName != "claude-no-watcher" {
		t.Errorf("expected sessionName from DB fallback, got %q", entry.SessionName)
	}
	if entry.ProviderType != "claude" {
		t.Errorf("expected providerType from DB fallback, got %q", entry.ProviderType)
	}
}

func TestHandleStatus_AllDBSessionsAppearInResponse(t *testing.T) {
	database := testDB(t)

	// Create two sessions — neither will have watchers.
	for _, s := range []*db.Session{
		{ID: "sess-a", Name: "A", TmuxName: "claude-a", ProviderType: "claude"},
		{ID: "sess-b", Name: "B", TmuxName: "claude-b", ProviderType: "shell"},
	} {
		if err := database.CreateSession(s); err != nil {
			t.Fatal(err)
		}
	}

	mgr := status.NewWatcherManager(&emptyLister{}, &noopWatcherDB{}, nil)
	mgr.Start(context.Background())
	defer mgr.Close()

	handler := handleStatus(mgr, database)
	req := httptest.NewRequest("GET", "/api/sessions/status", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var resp struct {
		Statuses map[string]struct {
			Status string `json:"status"`
		} `json:"statuses"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if len(resp.Statuses) != 2 {
		t.Errorf("expected 2 statuses (all DB sessions), got %d", len(resp.Statuses))
	}
	for _, id := range []string{"sess-a", "sess-b"} {
		if _, ok := resp.Statuses[id]; !ok {
			t.Errorf("expected %s in response", id)
		}
	}
}
