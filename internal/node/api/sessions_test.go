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

func TestSetProfileHandler_SessionNotFound(t *testing.T) {
	h := newTestSessionHandler(t)

	// profile=null is a valid detach request; the session itself is missing.
	req := httptest.NewRequest("PUT", "/api/sessions/missing/profile",
		strings.NewReader(`{"profile":null}`))
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()

	h.setProfile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing session, got %d", w.Code)
	}
}
