package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/status"
)

type fakeSnapshotter struct{ snap status.Snapshot }

func (f fakeSnapshotter) Snapshot() status.Snapshot { return f.snap }

func TestSummary_CountsAttentionAndBusy(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	if err := database.CreateSession(&db.Session{ID: "s-active", Name: "Active", TmuxName: "claude-active", ProviderType: "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(&db.Session{ID: "s-unread", Name: "Unread", TmuxName: "claude-unread", ProviderType: "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(&db.Session{ID: "s-clean", Name: "Clean", TmuxName: "claude-clean", ProviderType: "claude"}); err != nil {
		t.Fatal(err)
	}
	ts := "2026-01-01T00:00:00Z"
	if err := database.SetUnreadSince(ctx, "s-unread", &ts); err != nil {
		t.Fatal(err)
	}

	snap := status.Snapshot{Statuses: map[string]status.SnapshotEntry{
		"s-active": {SessionName: "claude-active", State: status.StateActive, ProviderType: "claude"},
	}}

	req := httptest.NewRequest("GET", "/summary", nil)
	rec := httptest.NewRecorder()
	handleSummary(fakeSnapshotter{snap: snap}, database).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var got struct {
		Attention int `json:"attention"`
		Busy      int `json:"busy"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 3 || got.Busy != 1 || got.Attention != 1 {
		t.Errorf("summary = %+v, want total=3 busy=1 attention=1", got)
	}
}

func TestSummary_DeadSessionWithManualUnreadCountsAsAttention(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	if err := database.CreateSession(&db.Session{ID: "s-dead-unread", Name: "Dead", TmuxName: "dead-tmux", ProviderType: "claude"}); err != nil {
		t.Fatal(err)
	}
	if err := database.MarkSessionUnread(ctx, "s-dead-unread"); err != nil {
		t.Fatal(err)
	}

	snap := status.Snapshot{Statuses: map[string]status.SnapshotEntry{
		"s-dead-unread": {State: status.StateDead, ProviderType: "claude"},
	}}

	req := httptest.NewRequest("GET", "/summary", nil)
	rec := httptest.NewRecorder()
	handleSummary(fakeSnapshotter{snap: snap}, database).ServeHTTP(rec, req)

	var got struct {
		Attention int `json:"attention"`
		Busy      int `json:"busy"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Attention != 1 || got.Busy != 0 || got.Total != 1 {
		t.Errorf("dead+unread summary = %+v, want attention=1 busy=0 total=1", got)
	}
}

func TestSummary_EmptyDBReturnsZeros(t *testing.T) {
	database := testDB(t)
	snap := status.Snapshot{Statuses: map[string]status.SnapshotEntry{}}

	req := httptest.NewRequest("GET", "/summary", nil)
	rec := httptest.NewRecorder()
	handleSummary(fakeSnapshotter{snap: snap}, database).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var got struct {
		Attention int `json:"attention"`
		Busy      int `json:"busy"`
		Total     int `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Attention != 0 || got.Busy != 0 || got.Total != 0 {
		t.Errorf("empty summary = %+v, want all zeros", got)
	}
}
