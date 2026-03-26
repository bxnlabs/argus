package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/node/status"
	"github.com/bxnlabs/argus/internal/node/terminal"
	ghservice "github.com/bxnlabs/argus/internal/github"
)

// Deps holds the dependencies injected into API handlers.
type Deps struct {
	SessionManager     *session.Manager
	StatusMonitor      *status.Monitor
	RepoIndexer        *ghservice.RepoIndexer
	UploadDirOverride  string // override upload directory (for testing)
}

// NewRouter creates the HTTP router with all node API routes.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/info", handleInfo)

	// Session routes
	sh := &sessionHandler{manager: deps.SessionManager}
	mux.HandleFunc("GET /api/sessions", sh.list)
	mux.HandleFunc("POST /api/sessions", sh.create)
	mux.HandleFunc("GET /api/sessions/{id}", sh.get)
	mux.HandleFunc("PATCH /api/sessions/{id}", sh.update)
	mux.HandleFunc("DELETE /api/sessions/{id}", sh.delete)

	// Profile routes
	mux.HandleFunc("GET /api/profiles", sh.listProfiles)

	// Git routes (read-only)
	gh := &gitHandler{}
	mux.HandleFunc("GET /api/git/status", gh.status)
	mux.HandleFunc("GET /api/git/diff", gh.diff)
	mux.HandleFunc("GET /api/git/working-diff", gh.workingDiff)
	mux.HandleFunc("GET /api/git/history", gh.history)
	mux.HandleFunc("GET /api/git/history/{hash}", gh.commitDetail)
	mux.HandleFunc("GET /api/git/history/{hash}/full-diff", gh.commitFullDiff)
	mux.HandleFunc("GET /api/git/compare/branches", gh.compareBranches)
	mux.HandleFunc("GET /api/git/compare", gh.compare)
	mux.HandleFunc("GET /api/git/file-content", gh.fileContent)
	mux.HandleFunc("GET /api/git/check", gh.check)

	// Review routes
	rh := &reviewHandler{}
	mux.HandleFunc("GET /api/git/review", rh.get)
	mux.HandleFunc("POST /api/git/review", rh.post)
	mux.HandleFunc("DELETE /api/git/review", rh.delete)

	// File routes
	fh := &filesHandler{uploadDirOverride: deps.UploadDirOverride}
	mux.HandleFunc("GET /api/files", fh.list)
	mux.HandleFunc("GET /api/files/meta", fh.meta)
	mux.HandleFunc("GET /api/files/content", fh.readContent)
	mux.HandleFunc("PUT /api/files/content", fh.writeContent)
	mux.HandleFunc("GET /api/files/search", fh.search)
	mux.HandleFunc("POST /api/files/upload", fh.upload)

	// Code search routes
	srch := &searchHandler{}
	mux.HandleFunc("GET /api/code-search", srch.search)
	mux.HandleFunc("GET /api/code-search/available", srch.available)

	// GitHub routes
	ghub := &githubHandler{repoIndexer: deps.RepoIndexer}
	mux.HandleFunc("GET /api/github/repos", ghub.listRepos)

	// Status route
	if deps.StatusMonitor != nil {
		mux.HandleFunc("GET /api/sessions/status", handleStatus(deps.StatusMonitor))
	}

	// Terminal WebSocket
	mux.HandleFunc("/ws/sessions/{id}", terminal.HandleSessionWebSocket(deps.SessionManager))
	mux.HandleFunc("/ws/terminal", terminal.HandleTerminalWebSocket)

	return corsMiddleware(mux)
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"name":    "argus-node",
		"version": "2.0.0",
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
