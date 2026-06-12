package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/bxnlabs/argus/internal/node/search"
	"github.com/bxnlabs/argus/internal/shared"
)

type searchHandler struct{}

// GET /code-search?query=...&path=...&maxResults=100
// The query is a regular expression (ripgrep syntax). maxResults is capped server-side.
func (h *searchHandler) search(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		respondError(w, http.StatusBadRequest, "query parameter is required")
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		respondError(w, http.StatusBadRequest, "path parameter is required")
		return
	}

	expandedPath, err := shared.ExpandPath(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	info, err := os.Stat(expandedPath)
	if err != nil {
		respondError(w, http.StatusBadRequest, "path does not exist")
		return
	}
	if !info.IsDir() {
		respondError(w, http.StatusBadRequest, "path must be a directory")
		return
	}

	maxResults := 100
	if s := r.URL.Query().Get("maxResults"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			maxResults = v
		}
	}

	result, err := search.Search(expandedPath, query, maxResults)
	if err != nil && result == nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Return partial results even on parse errors (e.g., scanner buffer exceeded).
	respondJSON(w, http.StatusOK, result)
}

// GET /code-search/available
func (h *searchHandler) available(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]bool{
		"available": search.IsAvailable(),
	})
}
