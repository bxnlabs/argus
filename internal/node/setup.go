package node

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/node/api"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/node/status"
	"github.com/bxnlabs/argus/internal/config"
	ghsvc "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/git/worktree"
)

// Setup initializes the node: opens the database, verifies migrations are
// current, and returns an HTTP handler with all node API routes. The returned
// cleanup function closes the database and should be called on shutdown.
func Setup(cfg *config.Config) (http.Handler, func(), error) {
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.CheckMigrations(); err != nil {
		database.Close()
		return nil, nil, err
	}

	// Determine state dir from DB path (~/.argus).
	// Canonicalize with ExpandPath + filepath.Abs so relative paths
	// (e.g. "node.db") resolve to the same directory as sqlite.Open.
	expandedDBPath, err := shared.ExpandPath(cfg.Database.Path)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("expand db path: %w", err)
	}
	absDBPath, err := filepath.Abs(expandedDBPath)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("abs db path: %w", err)
	}
	stateDir := filepath.Dir(absDBPath)

	wtMgr := worktree.NewManager(stateDir, cfg)

	mgr := session.NewManager(database, wtMgr, stateDir)
	detector := status.NewDetector()

	statusMon := status.NewMonitor(mgr, mgr, detector)
	statusMon.Start(context.Background())

	repoIndexer := ghsvc.NewRepoIndexer(stateDir)
	repoIndexer.Start(context.Background())

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		StatusMonitor:  statusMon,
		RepoIndexer:    repoIndexer,
		StateDir:       stateDir,
	})

	cleanup := func() {
		statusMon.Close()
		repoIndexer.Close()
		database.Close()
	}
	return handler, cleanup, nil
}
