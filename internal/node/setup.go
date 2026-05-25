package node

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	ghsvc "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/node/api"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/notifications"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/node/status"
	"github.com/bxnlabs/argus/internal/shared"
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
func Setup(cfg *config.Config, baseURL string) (http.Handler, func(), error) {
	// Resolve Argus's state root once (ARGUS_HOME, else ~/.argus) and share it
	// with every component below, so state never splits across locations.
	stateDir, err := shared.StateDir()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve state dir: %w", err)
	}

	// Bootstrap the dedicated tmux server state before anything else: the
	// directory holds the server socket and the seeded config is read once at
	// first server start. Both are required, so a failure here is fatal rather
	// than leaving the server permanently misconfigured.
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		return nil, nil, fmt.Errorf("ensure tmux dir: %w", err)
	}
	if _, err := shared.SeedTmuxConfig(); err != nil {
		return nil, nil, fmt.Errorf("seed tmux config: %w", err)
	}

	dbPath, err := shared.DBPath()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve db path: %w", err)
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.CheckMigrations(); err != nil {
		database.Close()
		return nil, nil, err
	}

	wtMgr := worktree.NewManager(stateDir, cfg)

	mgr := session.NewManager(database, wtMgr, stateDir)

	watcherDB := &prodWatcherDB{db: database}
	watcherMgr := status.NewWatcherManager(mgr, watcherDB, prodTmuxOps{})
	watcherMgr.Start(context.Background())

	repoIndexer := ghsvc.NewRepoIndexer(stateDir)
	repoIndexer.Start(context.Background())

	// Notification service (optional — only started when a channel is configured).
	var notifService *notifications.Service
	if cfg.Notifications.Channel != "" {
		threshold, err := time.ParseDuration(cfg.Notifications.NotifyAfterUnreadFor)
		if err != nil {
			// Should not happen — validated in config.Load
			watcherMgr.Close()
			repoIndexer.Close()
			database.Close()
			return nil, nil, fmt.Errorf("parse notification threshold: %w", err)
		}

		var sender notifications.Sender
		switch cfg.Notifications.Channel {
		case "slack":
			sender = notifications.NewSlackSender(
				cfg.Notifications.Slack.BotToken,
				cfg.Notifications.Slack.ChannelID,
				baseURL,
			)
		default:
			watcherMgr.Close()
			repoIndexer.Close()
			database.Close()
			return nil, nil, fmt.Errorf("notification channel %q is not implemented", cfg.Notifications.Channel)
		}

		notifService = notifications.NewService(sender, database, threshold)
		notifService.Start(context.Background())
		log.Printf("notification service started (channel=%s, threshold=%s)",
			cfg.Notifications.Channel, cfg.Notifications.NotifyAfterUnreadFor)
	}

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		WatcherManager: watcherMgr,
		Database:       database,
		RepoIndexer:    repoIndexer,
		StateDir:       stateDir,
	})

	cleanup := func() {
		if notifService != nil {
			notifService.Close()
		}
		watcherMgr.Close()
		repoIndexer.Close()
		database.Close()
	}
	return handler, cleanup, nil
}
