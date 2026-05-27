package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
)

func newTestSessionHandler(t *testing.T) *sessionHandler {
	t.Helper()
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := session.NewManager(database, wt, stateDir)
	return &sessionHandler{manager: mgr}
}

func TestSetProfileHandler_InvalidName(t *testing.T) {
	h := newTestSessionHandler(t)

	req := httptest.NewRequest("PUT", "/api/sessions/whatever/profile",
		strings.NewReader(`{"profile":"../evil"}`))
	req.SetPathValue("id", "whatever")
	w := httptest.NewRecorder()

	h.setProfile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid profile name, got %d", w.Code)
	}
}

func TestSetProfileHandler_RequiresProfile(t *testing.T) {
	h := newTestSessionHandler(t)

	// PUT sets a named profile; absent, null, and "" are all rejected. Detach
	// is a separate operation (DELETE). The handler rejects before touching the
	// manager, so the session need not exist.
	for _, body := range []string{`{}`, `{"profile":null}`, `{"profile":""}`} {
		req := httptest.NewRequest("PUT", "/api/sessions/whatever/profile",
			strings.NewReader(body))
		req.SetPathValue("id", "whatever")
		w := httptest.NewRecorder()

		h.setProfile(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, w.Code)
		}
	}
}

func TestDetachProfileHandler_SessionNotFound(t *testing.T) {
	h := newTestSessionHandler(t)

	// DELETE detaches; the session itself is missing.
	req := httptest.NewRequest("DELETE", "/api/sessions/missing/profile", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()

	h.detachProfile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing session, got %d", w.Code)
	}
}
