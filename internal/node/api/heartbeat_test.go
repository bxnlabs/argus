package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type fakeHeartbeatDB struct {
	mu           sync.Mutex
	lastViewedAt map[string]bool
	acknowledged map[string]bool
}

func newFakeHeartbeatDB() *fakeHeartbeatDB {
	return &fakeHeartbeatDB{
		lastViewedAt: make(map[string]bool),
		acknowledged: make(map[string]bool),
	}
}

func (f *fakeHeartbeatDB) TouchLastViewedAt(ctx context.Context, id string) error {
	f.mu.Lock()
	f.lastViewedAt[id] = true
	f.mu.Unlock()
	return nil
}

func (f *fakeHeartbeatDB) AcknowledgeSession(ctx context.Context, id string) error {
	f.mu.Lock()
	f.acknowledged[id] = true
	f.mu.Unlock()
	return nil
}

func TestHeartbeatHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/api/sessions/sess-1/heartbeat", nil)
	req.SetPathValue("id", "sess-1")
	w := httptest.NewRecorder()

	h.heartbeat(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	db.mu.Lock()
	if !db.lastViewedAt["sess-1"] {
		t.Error("expected last_viewed_at to be touched")
	}
	db.mu.Unlock()
}

func TestAcknowledgeHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/api/sessions/sess-2/acknowledge", nil)
	req.SetPathValue("id", "sess-2")
	w := httptest.NewRecorder()

	h.acknowledge(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	db.mu.Lock()
	if !db.acknowledged["sess-2"] {
		t.Error("expected session to be acknowledged")
	}
	db.mu.Unlock()
}
