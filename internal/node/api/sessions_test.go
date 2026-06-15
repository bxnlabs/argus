package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
)

func newTestSessionHandler(t *testing.T) (*sessionHandler, *db.DB) {
	t.Helper()
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := session.NewManager(database, wt, stateDir)
	return &sessionHandler{manager: mgr}, database
}

func TestSetProfileHandler_InvalidName(t *testing.T) {
	h, _ := newTestSessionHandler(t)

	req := httptest.NewRequest("PUT", "/sessions/whatever/profile",
		strings.NewReader(`{"profile":"../evil"}`))
	req.SetPathValue("id", "whatever")
	w := httptest.NewRecorder()

	h.setProfile(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid profile name, got %d", w.Code)
	}
}

func TestSetProfileHandler_RequiresProfile(t *testing.T) {
	h, _ := newTestSessionHandler(t)

	// PUT sets a named profile; absent, null, and "" are all rejected. Detach
	// is a separate operation (DELETE). The handler rejects before touching the
	// manager, so the session need not exist.
	for _, body := range []string{`{}`, `{"profile":null}`, `{"profile":""}`} {
		req := httptest.NewRequest("PUT", "/sessions/whatever/profile",
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
	h, _ := newTestSessionHandler(t)

	// DELETE detaches; the session itself is missing.
	req := httptest.NewRequest("DELETE", "/sessions/missing/profile", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()

	h.detachProfile(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing session, got %d", w.Code)
	}
}

func TestCloneHandler_SessionNotFound(t *testing.T) {
	h, _ := newTestSessionHandler(t)

	req := httptest.NewRequest("POST", "/api/sessions/missing/clone", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()

	h.clone(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing session, got %d", w.Code)
	}
}

// newProfileTestHandler builds a sessionHandler backed by a real Manager over a
// temp state dir, returning the state dir so tests can seed profile fixtures.
func newProfileTestHandler(t *testing.T) (*sessionHandler, string) {
	t.Helper()
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	wt := worktree.NewManager(stateDir, &config.Config{})
	mgr := session.NewManager(database, wt, stateDir)
	return &sessionHandler{manager: mgr}, stateDir
}

func TestProfileUpHandler_NotDockerized(t *testing.T) {
	h, stateDir := newProfileTestHandler(t)
	// Create a non-docker profile so ProfileUp returns ErrInvalidInput.
	if err := os.MkdirAll(filepath.Join(stateDir, "profiles", "plain", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/profiles/plain/up", nil)
	req.SetPathValue("name", "plain")
	w := httptest.NewRecorder()
	h.profileUp(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-docker profile, got %d", w.Code)
	}
}

func TestDetachProfileHandler_AlreadyDetachedIsNoop(t *testing.T) {
	h, database := newTestSessionHandler(t)

	// A session that already has no profile. Detaching again is an idempotent
	// no-op: ChangeProfile returns early (profile unchanged) without killing or
	// respawning tmux, so the handler responds 200.
	if err := database.CreateSession(&db.Session{
		ID: "sess-detached", Name: "d", TmuxName: "shell-sess-detached",
		WorkingDirectory: t.TempDir(), ProviderType: "shell", Profile: nil,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/sessions/sess-detached/profile", nil)
	req.SetPathValue("id", "sess-detached")
	w := httptest.NewRecorder()

	h.detachProfile(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for idempotent detach, got %d", w.Code)
	}
}
