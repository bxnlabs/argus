package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchHandler_MissingQuery(t *testing.T) {
	handler := &searchHandler{}
	req := httptest.NewRequest("GET", "/code-search?path=/tmp", nil)
	w := httptest.NewRecorder()
	handler.search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "query") {
		t.Error("expected query error message")
	}
}

func TestSearchHandler_MissingPath(t *testing.T) {
	handler := &searchHandler{}
	req := httptest.NewRequest("GET", "/code-search?query=test", nil)
	w := httptest.NewRecorder()
	handler.search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "path") {
		t.Error("expected path error message")
	}
}

func TestSearchHandler_WhitespaceQuery(t *testing.T) {
	handler := &searchHandler{}
	req := httptest.NewRequest("GET", "/code-search?query=%20%20&path=/tmp", nil)
	w := httptest.NewRecorder()
	handler.search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSearchHandler_NonExistentPath(t *testing.T) {
	handler := &searchHandler{}
	req := httptest.NewRequest("GET", "/code-search?query=test&path=/nonexistent/path/xyz", nil)
	w := httptest.NewRecorder()
	handler.search(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSearchHandler_Available(t *testing.T) {
	handler := &searchHandler{}
	req := httptest.NewRequest("GET", "/code-search/available", nil)
	w := httptest.NewRecorder()
	handler.available(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "available") {
		t.Errorf("expected 'available' field, got %s", body)
	}
}

func TestSearchHandlerViaRouter(t *testing.T) {
	router := NewRouter(Deps{})

	t.Run("GET /code-search missing query", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/code-search?path=/tmp", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", w.Code)
		}
	})

	t.Run("GET /code-search/available", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/code-search/available", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
	})
}
