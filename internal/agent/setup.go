package agent

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/agent/api"
	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/session"
	"github.com/bxnlabs/argus/internal/agent/status"
	"github.com/bxnlabs/argus/internal/config"
	ghsvc "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/git/worktree"
)

// Setup initializes the agent: opens the database, runs migrations, and
// returns an HTTP handler with all agent API routes. The returned cleanup
// function closes the database and should be called on shutdown.
func Setup(cfg *config.Config) (http.Handler, func(), error) {
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.RunMigrations(); err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("migrations: %w", err)
	}

	// Determine state dir from DB path (~/.argus).
	// Canonicalize with ExpandPath + filepath.Abs so relative paths
	// (e.g. "agent.db") resolve to the same directory as sqlite.Open.
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
	mgr.BackfillGitParentDir()
	detector := status.NewDetector()

	statusMon := status.NewMonitor(mgr, mgr, detector)
	statusMon.Start(context.Background())

	repoIndexer := ghsvc.NewRepoIndexer(stateDir)
	repoIndexer.Start(context.Background())

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		StatusMonitor:  statusMon,
		RepoIndexer:    repoIndexer,
	})

	cleanup := func() {
		statusMon.Close()
		repoIndexer.Close()
		database.Close()
	}
	return handler, cleanup, nil
}
