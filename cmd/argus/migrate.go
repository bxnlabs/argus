package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/git/worktree"
	"github.com/bxnlabs/argus/internal/node/db"
	"github.com/bxnlabs/argus/internal/node/session"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/spf13/cobra"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations and backfills",
		Long:  "Runs schema migrations and backfills session data (git_parent_dir, git_remote_url). Safe to run multiple times.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMigrate(cfg)
		},
	}

	return cmd
}

func runMigrate(c *config.Config) error {
	database, err := db.Open(c.Database.Path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer database.Close()

	if err := database.RunMigrations(); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	log.Println("schema migrations complete")

	expandedDBPath, err := shared.ExpandPath(c.Database.Path)
	if err != nil {
		return fmt.Errorf("expand db path: %w", err)
	}
	absDBPath, err := filepath.Abs(expandedDBPath)
	if err != nil {
		return fmt.Errorf("abs db path: %w", err)
	}
	stateDir := filepath.Dir(absDBPath)

	wtMgr := worktree.NewManager(stateDir, c)
	mgr := session.NewManager(database, wtMgr, stateDir)

	n, err := mgr.BackfillGitParentDir()
	if err != nil {
		return fmt.Errorf("backfill git_parent_dir: %w", err)
	}
	log.Printf("backfill git_parent_dir complete (%d sessions updated)", n)

	n, err = mgr.BackfillGitRemoteURL()
	if err != nil {
		return fmt.Errorf("backfill git_remote_url: %w", err)
	}
	log.Printf("backfill git_remote_url complete (%d sessions updated)", n)

	return nil
}
