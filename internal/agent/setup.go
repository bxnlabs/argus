package agent

import (
	"fmt"
	"net/http"

	"github.com/bxnlabs/argus/internal/agent/api"
	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/session"
	"github.com/bxnlabs/argus/internal/agent/status"
)

// Config holds the configuration for the agent.
type Config struct {
	DBPath string
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

	mgr := session.NewManager(database)
	detector := status.NewDetector()

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		StatusDetector: detector,
	})

	cleanup := func() {
		database.Close()
	}

	return handler, cleanup, nil
}
