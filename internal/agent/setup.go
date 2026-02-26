package agent

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/agent/api"
	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/session"
	"github.com/bxnlabs/argus/internal/agent/status"
	"github.com/bxnlabs/argus/internal/config"
	ghsvc "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/worktree"
)

// Config holds the configuration for the agent.
type Config struct {
	DBPath  string
	Address string
}

// Setup initializes the agent: opens the database, runs migrations, and
// returns an HTTP handler with all agent API routes. The returned cleanup
// function closes the database and should be called on shutdown.
func Setup(cfg Config) (http.Handler, func(), error) {
	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.RunMigrations(); err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("migrations: %w", err)
	}

	// Determine state dir from DB path (~/.argus)
	expandedDBPath, expandErr := shared.ExpandPath(cfg.DBPath)
	if expandErr != nil {
		expandedDBPath = cfg.DBPath // fall back to literal path
	}
	stateDir := filepath.Dir(expandedDBPath)
	if stateDir == "." {
		home, err := os.UserHomeDir()
		if err != nil {
			database.Close()
			return nil, nil, fmt.Errorf("home dir: %w", err)
		}
		stateDir = filepath.Join(home, ".argus")
	}

	userCfg, err := config.Load()
	if err != nil || userCfg == nil {
		userCfg = &config.Config{}
	}
	wtMgr := worktree.NewManager(stateDir, userCfg)

	mgr := session.NewManager(database, wtMgr)
	detector := status.NewDetector()

	var repoService *ghsvc.RepoService
	if userCfg.GitHubToken != "" {
		repoService = ghsvc.NewRepoService(userCfg.GitHubToken)
	}

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		StatusDetector: detector,
		RepoService:    repoService,
	})

	cleanup := func() { database.Close() }
	return handler, cleanup, nil
}
