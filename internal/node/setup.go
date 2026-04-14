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

// prodTmuxOps is the production implementation of TmuxWatcherOps.
type prodTmuxOps struct{}

func (prodTmuxOps) CapturePaneContent(ctx context.Context, name string) (string, error) {
	return session.CapturePaneContext(ctx, name)
}

func (prodTmuxOps) GetPaneDimensions(ctx context.Context, name string) (session.PaneDimensions, error) {
	return session.GetPaneDimensionsContext(ctx, name)
}

func (prodTmuxOps) HasSession(ctx context.Context, name string) (bool, error) {
	return session.HasSessionContext(ctx, name)
}

// prodWatcherDB adapts *db.DB to the WatcherDB interface.
type prodWatcherDB struct {
	db *db.DB
}

func (p *prodWatcherDB) SetUnreadSince(ctx context.Context, id string, ts *string) error {
	return p.db.SetUnreadSince(ctx, id, ts)
}

func (p *prodWatcherDB) TouchSession(ctx context.Context, id string, unixTS int64) error {
	return p.db.TouchSession(ctx, id, unixTS)
}

func (p *prodWatcherDB) GetSession(id string) (unreadSince, lastViewedAt *string, err error) {
	s, err := p.db.GetSession(id)
	if err != nil {
		return nil, nil, err
	}
	if s == nil {
		return nil, nil, nil
	}
	return s.UnreadSince, s.LastViewedAt, nil
}

// Setup initializes the node: opens the database, verifies migrations are
// current, and returns an HTTP handler with all node API routes.
func Setup(cfg *config.Config) (http.Handler, func(), error) {
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.CheckMigrations(); err != nil {
		database.Close()
		return nil, nil, err
	}

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

	watcherDB := &prodWatcherDB{db: database}
	watcherMgr := status.NewWatcherManager(mgr, watcherDB, prodTmuxOps{})
	watcherMgr.Start(context.Background())

	repoIndexer := ghsvc.NewRepoIndexer(stateDir)
	repoIndexer.Start(context.Background())

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		WatcherManager: watcherMgr,
		Database:       database,
		RepoIndexer:    repoIndexer,
		StateDir:       stateDir,
	})

	cleanup := func() {
		watcherMgr.Close()
		repoIndexer.Close()
		database.Close()
	}
	return handler, cleanup, nil
}
