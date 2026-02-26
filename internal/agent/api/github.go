package api

import (
	"net/http"

	ghservice "github.com/bxnlabs/argus/internal/github"
)

type githubHandler struct {
	repoService *ghservice.RepoService
}

// GET /api/github/repos?q=...
func (h *githubHandler) listRepos(w http.ResponseWriter, r *http.Request) {
	if h.repoService == nil {
		respondJSON(w, http.StatusOK, map[string]any{"repos": []string{}})
		return
	}

	query := r.URL.Query().Get("q")
	repos, err := h.repoService.Search(r.Context(), query)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"repos": repos})
}
