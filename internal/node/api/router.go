package api

import (
	"net/http"

	"github.com/bxnlabs/argus/internal/git/worktree"
	ghservice "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/node/status"
	"github.com/bxnlabs/argus/internal/node/terminal"
)

// Deps holds the dependencies injected into API handlers.
type Deps struct {
	SessionManager    *session.Manager
	WatcherManager    *status.WatcherManager
	Database          *db.DB
	RepoIndexer       *ghservice.RepoIndexer
	UploadDirOverride string // override upload directory (for testing)
	StateDir          string
	// WorktreeManager backs the /git/worktree(s) routes. If nil, those routes
	// are not registered.
	WorktreeManager *worktree.Manager
	// AllowOrigin reports whether a cross-origin request's Origin is permitted.
	// If nil, only loopback origins are allowed (the safe default — e.g. an
	// instance with Tailscale disabled has no remote peers anyway).
	AllowOrigin func(string) bool
	// AllowHost reports whether a request's Host hostname is one of this node's
	// own names (the anti-DNS-rebinding gate). If nil, every Host is allowed —
	// a test/embedding convenience; production always supplies a real policy via
	// node.Setup (mirrors registry.NewHandlers' nil-cors handling).
	AllowHost func(string) bool
}

// NewRouter creates the HTTP router with all node API routes.
func NewRouter(deps Deps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /info", handleInfo)

	// Session routes
	sh := &sessionHandler{manager: deps.SessionManager, watcherManager: deps.WatcherManager}
	mux.HandleFunc("GET /sessions", sh.list)
	mux.HandleFunc("POST /sessions", sh.create)
	mux.HandleFunc("POST /sessions/{id}/clone", sh.clone)
	mux.HandleFunc("GET /sessions/{id}", sh.get)
	mux.HandleFunc("PATCH /sessions/{id}", sh.update)
	mux.HandleFunc("DELETE /sessions/{id}", sh.delete)
	mux.HandleFunc("PUT /sessions/{id}/profile", sh.setProfile)
	mux.HandleFunc("DELETE /sessions/{id}/profile", sh.detachProfile)

	// Profile routes
	mux.HandleFunc("GET /profiles", sh.listProfiles)
	mux.HandleFunc("POST /profiles/{name}/up", sh.profileUp)
	mux.HandleFunc("POST /profiles/{name}/down", sh.profileDown)

	// Git routes. The ones carrying diffs or file bodies are compressed; the
	// rest answer with too little to be worth the framing.
	gh := &gitHandler{stateDir: deps.StateDir}
	mux.HandleFunc("GET /git/status", gh.status)
	mux.HandleFunc("GET /git/diff", gzipped(gh.diff))
	mux.HandleFunc("GET /git/working-diff", gzipped(gh.workingDiff))
	mux.HandleFunc("GET /git/history", gh.history)
	mux.HandleFunc("GET /git/history/{hash}", gh.commitDetail)
	mux.HandleFunc("GET /git/history/{hash}/full-diff", gzipped(gh.commitFullDiff))
	mux.HandleFunc("GET /git/compare/branches", gh.compareBranches)
	mux.HandleFunc("GET /git/compare", gzipped(gh.compare))
	mux.HandleFunc("GET /git/file-content", gzipped(gh.fileContent))
	mux.HandleFunc("GET /git/file-lines", gh.fileLines)
	mux.HandleFunc("GET /git/check", gh.check)
	mux.HandleFunc("GET /git/branches", gh.branches)
	mux.HandleFunc("POST /git/fetch", gh.fetch)

	// Review routes
	rh := &reviewHandler{}
	mux.HandleFunc("GET /git/review", rh.get)
	mux.HandleFunc("POST /git/review", rh.post)
	mux.HandleFunc("DELETE /git/review", rh.delete)

	// Worktree routes
	if deps.WorktreeManager != nil {
		wh := &worktreeHandler{mgr: deps.WorktreeManager, db: deps.Database}
		mux.HandleFunc("POST /git/worktree", wh.create)
		mux.HandleFunc("DELETE /git/worktree", wh.delete)
		mux.HandleFunc("GET /git/worktrees", wh.list)
	}

	// File routes
	fh := &filesHandler{uploadDirOverride: deps.UploadDirOverride}
	mux.HandleFunc("GET /files", fh.list)
	mux.HandleFunc("GET /files/meta", fh.meta)
	mux.HandleFunc("GET /files/content", fh.readContent)
	mux.HandleFunc("GET /files/search", fh.search)
	mux.HandleFunc("POST /files/upload", fh.upload)

	// Code search routes
	srch := &searchHandler{}
	mux.HandleFunc("GET /code-search", srch.search)
	mux.HandleFunc("GET /code-search/available", srch.available)

	// GitHub routes
	ghub := &githubHandler{repoIndexer: deps.RepoIndexer}
	mux.HandleFunc("GET /github/repos", ghub.listRepos)

	// Status route
	if deps.WatcherManager != nil && deps.Database != nil {
		mux.HandleFunc("GET /sessions/status", handleStatus(deps.WatcherManager, deps.Database))
		mux.HandleFunc("GET /summary", handleSummary(deps.WatcherManager, deps.Database))
	}

	// Heartbeat/acknowledge routes
	if deps.Database != nil {
		hb := &heartbeatHandler{db: deps.Database}
		mux.HandleFunc("POST /sessions/{id}/heartbeat", hb.heartbeat)
		mux.HandleFunc("POST /sessions/{id}/acknowledge", hb.acknowledge)
		mux.HandleFunc("POST /sessions/{id}/unread", hb.markUnread)
		mux.HandleFunc("POST /sessions/{id}/read", hb.markRead)
	}

	// Terminal WebSocket
	var onSessionReady terminal.SessionReadyFunc
	if deps.WatcherManager != nil && deps.SessionManager != nil {
		sm := deps.SessionManager
		wm := deps.WatcherManager
		onSessionReady = func(sessionID, tmuxName string) {
			sess, err := sm.Get(sessionID)
			if err != nil || sess == nil {
				return
			}
			wm.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
		}
	}
	mux.HandleFunc("/ws/sessions/{id}", terminal.HandleSessionWebSocket(deps.SessionManager, onSessionReady))
	mux.HandleFunc("/ws/terminal", terminal.HandleTerminalWebSocket)

	allowOrigin := deps.AllowOrigin
	if allowOrigin == nil {
		allowOrigin = NewCORSPolicy("") // loopback-only
	}
	allowHost := deps.AllowHost
	if allowHost == nil {
		allowHost = func(string) bool { return true } // test/embedding convenience
	}
	return CORS(allowHost, allowOrigin, mux)
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"name":    "argus-node",
		"version": "2.0.0",
	})
}
