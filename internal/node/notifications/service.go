package notifications

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
)

const pollInterval = 30 * time.Second
const gcInterval = 10 * time.Minute

const sqliteDatetimeFormat = "2006-01-02 15:04:05"

// NotificationDB abstracts database operations for the notification service.
type NotificationDB interface {
	UnreadSessions(ctx context.Context) ([]db.UnreadSession, error)
	HasNotification(ctx context.Context, sessionID, after string) (bool, error)
	InsertNotification(ctx context.Context, sessionID, sentAt string) error
	GCNotifications(ctx context.Context) (int64, error)
}

// Service polls for unread sessions and sends notifications via the configured Sender.
type Service struct {
	sender    Sender
	db        NotificationDB
	threshold time.Duration

	cancel    context.CancelFunc
	wg        sync.WaitGroup
	startOnce sync.Once

	// nowFn for testing
	nowFn func() time.Time
}

// NewService creates a notification service.
func NewService(sender Sender, db NotificationDB, threshold time.Duration) *Service {
	return &Service{
		sender:    sender,
		db:        db,
		threshold: threshold,
		nowFn:     time.Now,
	}
}

// Start launches the polling goroutine. Safe to call multiple times.
func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		ctx, s.cancel = context.WithCancel(ctx)
		s.wg.Add(1)
		go s.loop(ctx)
	})
}

// Close cancels the polling goroutine and waits for it to exit.
func (s *Service) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Service) loop(ctx context.Context) {
	defer s.wg.Done()

	// Poll immediately on start.
	s.poll(ctx)

	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

	gcTicker := time.NewTicker(gcInterval)
	defer gcTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			s.poll(ctx)
		case <-gcTicker.C:
			s.gc(ctx)
		}
	}
}

func (s *Service) gc(ctx context.Context) {
	deleted, err := s.db.GCNotifications(ctx)
	if err != nil {
		log.Printf("notifications: gc: %v", err)
		return
	}
	if deleted > 0 {
		log.Printf("notifications: gc cleaned up %d old notification(s)", deleted)
	}
}

func (s *Service) poll(ctx context.Context) {
	now := s.nowFn()

	sessions, err := s.db.UnreadSessions(ctx)
	if err != nil {
		log.Printf("notifications: query unread sessions: %v", err)
		return
	}

	for _, sess := range sessions {
		unreadSince, err := time.Parse(sqliteDatetimeFormat, sess.UnreadSince)
		if err != nil {
			log.Printf("notifications: parse unread_since for %s: %v", sess.ID, err)
			continue
		}

		unreadFor := now.Sub(unreadSince)
		if unreadFor < s.threshold {
			continue
		}

		// Deduplication: skip if already notified for this unread event
		has, err := s.db.HasNotification(ctx, sess.ID, sess.UnreadSince)
		if err != nil {
			log.Printf("notifications: check notification for %s: %v", sess.ID, err)
			continue
		}
		if has {
			continue
		}

		msg := Message{
			SessionID:      sess.ID,
			SessionName:    sess.Name,
			WorkingDir:     sess.WorkingDirectory,
			UnreadSince:    unreadSince,
			UnreadFor:      unreadFor,
			WorktreeBranch: sess.WorktreeBranch,
			GitParentDir:   sess.GitParentDir,
			GitRemoteURL:   sess.GitRemoteURL,
		}

		if err := s.sender.Send(ctx, msg); err != nil {
			log.Printf("notifications: send for session %s: %v", sess.ID, err)
			continue
		}

		sentAt := now.UTC().Format(sqliteDatetimeFormat)
		if err := s.db.InsertNotification(ctx, sess.ID, sentAt); err != nil {
			log.Printf("notifications: insert notification for %s: %v", sess.ID, err)
		}
	}
}
