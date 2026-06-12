package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/bxnlabs/argus/internal/node/db"
)

type fakeHeartbeatDB struct {
	mu           sync.Mutex
	lastViewedAt map[string]bool
	acknowledged map[string]bool
	markedUnread map[string]bool
	read         map[string]bool
	// notFound makes every method report db.ErrNotFound, simulating a session
	// that does not exist.
	notFound bool
}

func newFakeHeartbeatDB() *fakeHeartbeatDB {
	return &fakeHeartbeatDB{
		lastViewedAt: make(map[string]bool),
		acknowledged: make(map[string]bool),
		markedUnread: make(map[string]bool),
		read:         make(map[string]bool),
	}
}

func (f *fakeHeartbeatDB) MarkSessionUnread(ctx context.Context, id string) error {
	if f.notFound {
		return db.ErrNotFound
	}
	f.mu.Lock()
	f.markedUnread[id] = true
	f.mu.Unlock()
	return nil
}

func (f *fakeHeartbeatDB) TouchLastViewedAt(ctx context.Context, id string) error {
	if f.notFound {
		return db.ErrNotFound
	}
	f.mu.Lock()
	f.lastViewedAt[id] = true
	f.mu.Unlock()
	return nil
}

func (f *fakeHeartbeatDB) AcknowledgeSession(ctx context.Context, id string) error {
	if f.notFound {
		return db.ErrNotFound
	}
	f.mu.Lock()
	f.acknowledged[id] = true
	f.mu.Unlock()
	return nil
}

func (f *fakeHeartbeatDB) MarkSessionRead(ctx context.Context, id string) error {
	if f.notFound {
		return db.ErrNotFound
	}
	f.mu.Lock()
	f.read[id] = true
	f.mu.Unlock()
	return nil
}

func TestHeartbeatHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/sessions/sess-1/heartbeat", nil)
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

	req := httptest.NewRequest("POST", "/sessions/sess-2/acknowledge", nil)
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

func TestMarkUnreadHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/sessions/sess-3/unread", nil)
	req.SetPathValue("id", "sess-3")
	w := httptest.NewRecorder()

	h.markUnread(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	db.mu.Lock()
	if !db.markedUnread["sess-3"] {
		t.Error("expected session to be marked unread")
	}
	db.mu.Unlock()
}

func TestMarkReadHandler(t *testing.T) {
	db := newFakeHeartbeatDB()
	h := &heartbeatHandler{db: db}

	req := httptest.NewRequest("POST", "/sessions/sess-4/read", nil)
	req.SetPathValue("id", "sess-4")
	w := httptest.NewRecorder()

	h.markRead(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}

	db.mu.Lock()
	if !db.read["sess-4"] {
		t.Error("expected session to be marked read")
	}
	db.mu.Unlock()
}

// All four endpoints must return 404 when the underlying DB reports the session
// does not exist, rather than a misleading 204.
func TestHeartbeatHandlersMissingSession(t *testing.T) {
	database := &fakeHeartbeatDB{notFound: true}
	h := &heartbeatHandler{db: database}

	cases := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"heartbeat", h.heartbeat, "/sessions/missing/heartbeat"},
		{"acknowledge", h.acknowledge, "/sessions/missing/acknowledge"},
		{"unread", h.markUnread, "/sessions/missing/unread"},
		{"read", h.markRead, "/sessions/missing/read"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", tc.path, nil)
			req.SetPathValue("id", "missing")
			w := httptest.NewRecorder()

			tc.handler(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for missing session, got %d", w.Code)
			}
		})
	}
}
