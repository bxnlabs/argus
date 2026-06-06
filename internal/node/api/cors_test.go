package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_ReflectsOrigin(t *testing.T) {
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	req.Header.Set("Origin", "http://pane.example:80")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://pane.example:80" {
		t.Errorf("Allow-Origin = %q, want reflected origin", got)
	}
}

func TestCORS_PreflightShortCircuits(t *testing.T) {
	called := false
	h := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	req := httptest.NewRequest(http.MethodOptions, "/sessions", nil)
	req.Header.Set("Origin", "http://pane.example")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if called {
		t.Error("preflight must not reach the wrapped handler")
	}
}
