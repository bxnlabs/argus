package api

import (
	"net/http"

	ghservice "github.com/bxnlabs/argus/internal/github"
)

type githubHandler struct {
	repoIndexer *ghservice.RepoIndexer
}

// GET /api/github/repos?q=...
func (h *githubHandler) listRepos(w http.ResponseWriter, r *http.Request) {
	if h.repoIndexer == nil {
		respondJSON(w, http.StatusOK, map[string]any{"repos": []string{}})
		return
	}

	query := r.URL.Query().Get("q")
	repos := h.repoIndexer.Search(query)
	respondJSON(w, http.StatusOK, map[string]any{"repos": repos})
}
