package notifications

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	nodedb "github.com/bxnlabs/argus/internal/node/db"
)

// mockNotificationDB implements NotificationDB for testing.
type mockNotificationDB struct {
	mu            sync.Mutex
	sessions      []nodedb.UnreadSession
	notifications map[string][]string // sessionID -> []sentAt
}

func newMockNotificationDB() *mockNotificationDB {
	return &mockNotificationDB{
		notifications: make(map[string][]string),
	}
}

func (m *mockNotificationDB) UnreadSessions(ctx context.Context) ([]nodedb.UnreadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]nodedb.UnreadSession(nil), m.sessions...), nil
}

func (m *mockNotificationDB) HasNotification(ctx context.Context, sessionID, after string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sentAt := range m.notifications[sessionID] {
		if sentAt > after {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockNotificationDB) InsertNotification(ctx context.Context, sessionID, sentAt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications[sessionID] = append(m.notifications[sessionID], sentAt)
	return nil
}

func (m *mockNotificationDB) setUnreadSessions(sessions []nodedb.UnreadSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions = sessions
}

func (m *mockNotificationDB) notificationCount(sessionID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications[sessionID])
}

// mockSender records Send calls.
type mockSender struct {
	mu       sync.Mutex
	messages []Message
	err      error
}

func (s *mockSender) Send(ctx context.Context, msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return s.err
}

func (s *mockSender) messageCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func TestServiceSendsNotificationAfterThreshold(t *testing.T) {
	mockDB := newMockNotificationDB()
	sender := &mockSender{}
	threshold := 5 * time.Minute
	now := time.Date(2026, 4, 17, 12, 10, 0, 0, time.UTC)

	svc := NewService(sender, mockDB, threshold)
	svc.nowFn = func() time.Time { return now }

	// Session has been unread for 10 minutes (threshold is 5)
	mockDB.setUnreadSessions([]nodedb.UnreadSession{
		{
			ID:               "s1",
			Name:             "my-session",
			ProviderType:     "claude",
			WorkingDirectory: "/tmp/proj",
			UnreadSince:      "2026-04-17 12:00:00",
		},
	})

	svc.poll(context.Background())

	if sender.messageCount() != 1 {
		t.Fatalf("expected 1 message sent, got %d", sender.messageCount())
	}
	if mockDB.notificationCount("s1") != 1 {
		t.Fatalf("expected 1 notification inserted, got %d", mockDB.notificationCount("s1"))
	}
}

func TestServiceSkipsBelowThreshold(t *testing.T) {
	mockDB := newMockNotificationDB()
	sender := &mockSender{}
	threshold := 5 * time.Minute
	now := time.Date(2026, 4, 17, 12, 3, 0, 0, time.UTC)

	svc := NewService(sender, mockDB, threshold)
	svc.nowFn = func() time.Time { return now }

	// Session has been unread for only 3 minutes (threshold is 5)
	mockDB.setUnreadSessions([]nodedb.UnreadSession{
		{
			ID:               "s1",
			Name:             "my-session",
			ProviderType:     "claude",
			WorkingDirectory: "/tmp/proj",
			UnreadSince:      "2026-04-17 12:00:00",
		},
	})

	svc.poll(context.Background())

	if sender.messageCount() != 0 {
		t.Fatalf("expected 0 messages, got %d", sender.messageCount())
	}
}

func TestServiceDeduplicates(t *testing.T) {
	mockDB := newMockNotificationDB()
	sender := &mockSender{}
	threshold := 5 * time.Minute
	now := time.Date(2026, 4, 17, 12, 10, 0, 0, time.UTC)

	svc := NewService(sender, mockDB, threshold)
	svc.nowFn = func() time.Time { return now }

	mockDB.setUnreadSessions([]nodedb.UnreadSession{
		{
			ID:               "s1",
			Name:             "my-session",
			ProviderType:     "claude",
			WorkingDirectory: "/tmp/proj",
			UnreadSince:      "2026-04-17 12:00:00",
		},
	})

	// First poll sends
	svc.poll(context.Background())
	if sender.messageCount() != 1 {
		t.Fatalf("first poll: expected 1 message, got %d", sender.messageCount())
	}

	// Second poll should deduplicate
	svc.poll(context.Background())
	if sender.messageCount() != 1 {
		t.Fatalf("second poll: expected still 1 message (dedup), got %d", sender.messageCount())
	}
}

func TestServiceSkipsInsertOnSendError(t *testing.T) {
	mockDB := newMockNotificationDB()
	sender := &mockSender{err: fmt.Errorf("slack down")}
	threshold := 5 * time.Minute
	now := time.Date(2026, 4, 17, 12, 10, 0, 0, time.UTC)

	svc := NewService(sender, mockDB, threshold)
	svc.nowFn = func() time.Time { return now }

	mockDB.setUnreadSessions([]nodedb.UnreadSession{
		{
			ID:               "s1",
			Name:             "my-session",
			ProviderType:     "claude",
			WorkingDirectory: "/tmp/proj",
			UnreadSince:      "2026-04-17 12:00:00",
		},
	})

	svc.poll(context.Background())

	// Send was attempted but failed — no notification inserted
	if mockDB.notificationCount("s1") != 0 {
		t.Fatalf("expected 0 notifications (send failed), got %d", mockDB.notificationCount("s1"))
	}
}

func TestServiceNewUnreadEventAfterRead(t *testing.T) {
	mockDB := newMockNotificationDB()
	sender := &mockSender{}
	threshold := 5 * time.Minute

	svc := NewService(sender, mockDB, threshold)

	// First unread event at 12:00, poll at 12:10
	now1 := time.Date(2026, 4, 17, 12, 10, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now1 }
	mockDB.setUnreadSessions([]nodedb.UnreadSession{
		{ID: "s1", Name: "s", ProviderType: "claude", WorkingDirectory: "/tmp", UnreadSince: "2026-04-17 12:00:00"},
	})
	svc.poll(context.Background())
	if sender.messageCount() != 1 {
		t.Fatalf("expected 1 message after first unread event, got %d", sender.messageCount())
	}

	// User reads, then new unread event at 12:20, poll at 12:30
	now2 := time.Date(2026, 4, 17, 12, 30, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return now2 }
	mockDB.setUnreadSessions([]nodedb.UnreadSession{
		{ID: "s1", Name: "s", ProviderType: "claude", WorkingDirectory: "/tmp", UnreadSince: "2026-04-17 12:20:00"},
	})
	svc.poll(context.Background())
	if sender.messageCount() != 2 {
		t.Fatalf("expected 2 messages (new unread event), got %d", sender.messageCount())
	}
}

func TestServiceStartAndClose(t *testing.T) {
	mockDB := newMockNotificationDB()
	sender := &mockSender{}

	svc := NewService(sender, mockDB, 5*time.Minute)

	ctx := context.Background()
	svc.Start(ctx)

	// Start is idempotent
	svc.Start(ctx)

	// Close should not hang
	svc.Close()
}
