package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilesSearch_MissingQuery(t *testing.T) {
	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/api/files/search", nil)
	w := httptest.NewRecorder()
	handler.search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "q parameter") {
		t.Error("expected 'q parameter' in error message")
	}
}

func TestFilesSearch_InvalidType(t *testing.T) {
	handler := &filesHandler{}
	req := httptest.NewRequest("GET", "/api/files/search?q=test&type=invalid", nil)
	w := httptest.NewRecorder()
	handler.search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestFilesSearch_ViaRouter(t *testing.T) {
	deps := Deps{} // filesHandler has no deps
	router := NewRouter(deps)

	req := httptest.NewRequest("GET", "/api/files/search?q=test&type=directory&limit=5", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should be 200 (even if 0 results) or 500 (if fd not installed)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 200 or 500", w.Code)
	}

	if w.Code == http.StatusOK {
		var resp struct {
			Results []json.RawMessage `json:"results"`
			Query   string            `json:"query"`
			Count   int               `json:"count"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if resp.Query != "test" {
			t.Errorf("query = %q, want 'test'", resp.Query)
		}
	}
}
