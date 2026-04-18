# Slack Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send Slack notifications when coding sessions have been unread beyond a configurable duration.

**Architecture:** A polling-based `notifications.Service` goroutine (30s interval) queries the DB for unread sessions past the threshold, deduplicates against a `notifications` table, and dispatches messages via a `Sender` interface. The only concrete sender is `SlackSender` using `slack-go/slack` with Block Kit formatting.

**Tech Stack:** Go, SQLite (existing), `slack-go/slack` library, Block Kit messages

---

### Task 1: Add notification config structs and validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write failing tests for notification config defaults and validation**

Add these tests to `internal/config/config_test.go`:

```go
func TestNotificationsDefaults(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Notifications.Channel != "" {
		t.Errorf("Notifications.Channel = %q, want empty", cfg.Notifications.Channel)
	}
	if cfg.Notifications.NotifyAfterUnreadFor != "5m" {
		t.Errorf("Notifications.NotifyAfterUnreadFor = %q, want %q", cfg.Notifications.NotifyAfterUnreadFor, "5m")
	}
}

func TestNotificationsSlackFromFile(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"
notify_after_unread_for = "10m"

[notifications.slack]
bot_token = "xoxb-test-token"
channel_id = "C1234567890"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Notifications.Channel != "slack" {
		t.Errorf("Notifications.Channel = %q, want %q", cfg.Notifications.Channel, "slack")
	}
	if cfg.Notifications.NotifyAfterUnreadFor != "10m" {
		t.Errorf("Notifications.NotifyAfterUnreadFor = %q, want %q", cfg.Notifications.NotifyAfterUnreadFor, "10m")
	}
	if cfg.Notifications.Slack.BotToken != "xoxb-test-token" {
		t.Errorf("Slack.BotToken = %q, want %q", cfg.Notifications.Slack.BotToken, "xoxb-test-token")
	}
	if cfg.Notifications.Slack.ChannelID != "C1234567890" {
		t.Errorf("Slack.ChannelID = %q, want %q", cfg.Notifications.Slack.ChannelID, "C1234567890")
	}
}

func TestValidation_SlackMissingBotToken(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"

[notifications.slack]
channel_id = "C1234567890"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for missing bot_token, got nil")
	}
}

func TestValidation_SlackMissingChannelID(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"

[notifications.slack]
bot_token = "xoxb-test-token"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for missing channel_id, got nil")
	}
}

func TestValidation_InvalidNotifyAfterUnreadFor(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"
notify_after_unread_for = "not-a-duration"

[notifications.slack]
bot_token = "xoxb-test-token"
channel_id = "C1234567890"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for invalid duration, got nil")
	}
}

func TestValidation_UnknownNotificationChannel(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "email"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for unknown channel, got nil")
	}
}

func TestNotificationsDisabledByDefault(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty channel means disabled — no validation of slack fields
	if cfg.Notifications.Channel != "" {
		t.Errorf("Notifications.Channel = %q, want empty (disabled)", cfg.Notifications.Channel)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/config/ -run "TestNotifications|TestValidation_Slack|TestValidation_InvalidNotifyAfterUnreadFor|TestValidation_UnknownNotificationChannel" -v`

Expected: compilation errors — `cfg.Notifications` field does not exist.

- [ ] **Step 3: Add config structs, defaults, and validation**

In `internal/config/config.go`, add the structs and wire them in:

```go
// Add to Config struct:
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Node          NodeConfig          `mapstructure:"node"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Git           GitConfig           `mapstructure:"git"`
	Tailscale     TailscaleConfig     `mapstructure:"tailscale"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
}

// Add new structs (after TailscaleConfig):
type NotificationsConfig struct {
	Channel              string           `mapstructure:"channel"`
	NotifyAfterUnreadFor string           `mapstructure:"notify_after_unread_for"`
	Slack                SlackNotifyConfig `mapstructure:"slack"`
}

type SlackNotifyConfig struct {
	BotToken  string `mapstructure:"bot_token"`
	ChannelID string `mapstructure:"channel_id"`
}
```

Add defaults in `Load()` (after the tailscale defaults):

```go
v.SetDefault("notifications.channel", "")
v.SetDefault("notifications.notify_after_unread_for", "5m")
```

Add `"time"` to imports and add validation in `validate()` (before the final `return nil`):

```go
if cfg.Notifications.Channel != "" {
	if _, err := time.ParseDuration(cfg.Notifications.NotifyAfterUnreadFor); err != nil {
		return fmt.Errorf("notifications.notify_after_unread_for must be a valid duration (e.g. \"5m\"), got %q", cfg.Notifications.NotifyAfterUnreadFor)
	}
	switch cfg.Notifications.Channel {
	case "slack":
		if cfg.Notifications.Slack.BotToken == "" {
			return fmt.Errorf("notifications.slack.bot_token is required when notifications.channel = \"slack\"")
		}
		if cfg.Notifications.Slack.ChannelID == "" {
			return fmt.Errorf("notifications.slack.channel_id is required when notifications.channel = \"slack\"")
		}
	default:
		return fmt.Errorf("notifications.channel must be \"slack\" or empty, got %q", cfg.Notifications.Channel)
	}
}
```

Also add these env var keys to `clearArgusEnv` in `config_test.go`:

```go
"ARGUS_NOTIFICATIONS_CHANNEL",
"ARGUS_NOTIFICATIONS_NOTIFY_AFTER_UNREAD_FOR",
"ARGUS_NOTIFICATIONS_SLACK_BOT_TOKEN",
"ARGUS_NOTIFICATIONS_SLACK_CHANNEL_ID",
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/config/ -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add notifications config structs with slack validation"
```

---

### Task 2: Add notifications database table and methods

**Files:**
- Modify: `internal/node/db/schema.go`
- Modify: `internal/node/db/migrations.go`
- Modify: `internal/node/db/db.go` (seedMigrations)
- Create: `internal/node/db/notifications.go`
- Modify: `internal/node/db/db_test.go`

- [ ] **Step 1: Write failing tests for notification DB methods**

Add these tests to `internal/node/db/db_test.go`:

```go
func TestUnreadSessions(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	// Create two sessions
	db.CreateSession(&Session{
		ID: "s1", Name: "session-1", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp/proj1", ProviderType: "claude",
	})
	db.CreateSession(&Session{
		ID: "s2", Name: "session-2", TmuxName: "claude-s2",
		WorkingDirectory: "/tmp/proj2", ProviderType: "codex",
	})

	// No unread sessions initially
	sessions, err := db.UnreadSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 unread sessions, got %d", len(sessions))
	}

	// Mark s1 as unread
	ts := "2026-04-17 12:00:00"
	db.SetUnreadSince(context.Background(), "s1", &ts)

	sessions, err = db.UnreadSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 unread session, got %d", len(sessions))
	}
	if sessions[0].ID != "s1" {
		t.Errorf("expected session ID %q, got %q", "s1", sessions[0].ID)
	}
	if sessions[0].Name != "session-1" {
		t.Errorf("expected session name %q, got %q", "session-1", sessions[0].Name)
	}
	if sessions[0].ProviderType != "claude" {
		t.Errorf("expected provider %q, got %q", "claude", sessions[0].ProviderType)
	}
	if sessions[0].WorkingDirectory != "/tmp/proj1" {
		t.Errorf("expected working dir %q, got %q", "/tmp/proj1", sessions[0].WorkingDirectory)
	}
	if sessions[0].UnreadSince != ts {
		t.Errorf("expected unread_since %q, got %q", ts, sessions[0].UnreadSince)
	}
}

func TestHasNotification(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	// No notification exists
	has, err := db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no notification, got true")
	}

	// Insert a notification
	if err := db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00"); err != nil {
		t.Fatal(err)
	}

	// Notification exists after the unread_since timestamp
	has, err = db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected notification to exist, got false")
	}

	// Notification does NOT exist after a later timestamp (new unread event)
	has, err = db.HasNotification(context.Background(), "s1", "2026-04-17 12:10:00")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected no notification after later timestamp, got true")
	}
}

func TestInsertNotification(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})

	err := db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00")
	if err != nil {
		t.Fatal(err)
	}

	// Verify via HasNotification
	has, err := db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected notification to exist after insert")
	}
}

func TestNotificationsCascadeOnSessionDelete(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	db.CreateSession(&Session{
		ID: "s1", Name: "test", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp", ProviderType: "claude",
	})
	db.InsertNotification(context.Background(), "s1", "2026-04-17 12:05:00")

	// Delete the session
	if err := db.DeleteSession("s1"); err != nil {
		t.Fatal(err)
	}

	// Notification should be gone (cascade delete)
	has, err := db.HasNotification(context.Background(), "s1", "2026-04-17 12:00:00")
	if err != nil {
		t.Fatal(err)
	}
	if has {
		t.Error("expected notification to be cascade-deleted with session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/node/db/ -run "TestUnreadSessions|TestHasNotification|TestInsertNotification|TestNotificationsCascade" -v`

Expected: compilation errors — `UnreadSessions`, `HasNotification`, `InsertNotification` methods don't exist.

- [ ] **Step 3: Add notifications table to schema and migration**

In `internal/node/db/schema.go`, add the notifications table to the schema constant (after the `_migrations` table):

```sql
-- Notifications tracking
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  sent_at TEXT NOT NULL,
  FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);
```

In `internal/node/db/migrations.go`, add a new migration entry to the `allMigrations` slice:

```go
{"create_notifications_table", func(d *DB) error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS notifications (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		sent_at TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`)
	return err
}},
```

In `internal/node/db/db.go`, update `seedMigrations()` to detect and seed the new migration for fresh databases. After the existing `rows.Err()` check (and before the existing seeding `if` blocks), add a check for the notifications table:

```go
// Check if notifications table exists (fresh schema includes it).
var notifTableCount int
row := d.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='notifications'`)
if err := row.Scan(&notifTableCount); err != nil {
	return err
}
```

Then at the end of `seedMigrations()`, before the final `return nil`, add:

```go
if notifTableCount > 0 {
	if _, err := d.sql.Exec(
		`INSERT OR IGNORE INTO _migrations (name) VALUES (?)`,
		"create_notifications_table",
	); err != nil {
		return err
	}
}
```

- [ ] **Step 4: Create notification DB methods**

Create `internal/node/db/notifications.go`:

```go
package db

import "context"

// UnreadSession holds the fields needed by the notification service.
type UnreadSession struct {
	ID               string
	Name             string
	ProviderType     string
	WorkingDirectory string
	UnreadSince      string
}

// UnreadSessions returns sessions where unread_since IS NOT NULL.
func (d *DB) UnreadSessions(ctx context.Context) ([]UnreadSession, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, name, provider_type, working_directory, unread_since
		 FROM sessions WHERE unread_since IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UnreadSession
	for rows.Next() {
		var s UnreadSession
		if err := rows.Scan(&s.ID, &s.Name, &s.ProviderType, &s.WorkingDirectory, &s.UnreadSince); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// HasNotification checks if a notification exists for the given session
// with sent_at > the provided timestamp. Used for deduplication: if a
// notification was sent after the current unread_since, the event has
// already been notified.
func (d *DB) HasNotification(ctx context.Context, sessionID, after string) (bool, error) {
	var count int
	err := d.sql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE session_id = ? AND sent_at > ?`,
		sessionID, after,
	).Scan(&count)
	return count > 0, err
}

// InsertNotification records that a notification was sent for a session.
func (d *DB) InsertNotification(ctx context.Context, sessionID, sentAt string) error {
	_, err := d.sql.ExecContext(ctx,
		`INSERT INTO notifications (session_id, sent_at) VALUES (?, ?)`,
		sessionID, sentAt,
	)
	return err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/node/db/ -v`

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/node/db/schema.go internal/node/db/migrations.go internal/node/db/db.go internal/node/db/notifications.go internal/node/db/db_test.go
git commit -m "feat: add notifications table with DB methods for deduplication"
```

---

### Task 3: Create Sender interface and Slack sender

**Files:**
- Create: `internal/node/notifications/sender.go`
- Create: `internal/node/notifications/slack.go`
- Create: `internal/node/notifications/slack_test.go`

- [ ] **Step 1: Add `slack-go/slack` dependency**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go get github.com/slack-go/slack`

- [ ] **Step 2: Create Sender interface and Message type**

Create `internal/node/notifications/sender.go`:

```go
package notifications

import (
	"context"
	"time"
)

// Message holds the data needed to compose a notification.
type Message struct {
	SessionID   string
	SessionName string
	Provider    string
	WorkingDir  string
	UnreadSince time.Time
	UnreadFor   time.Duration
}

// Sender sends a notification message. Implementations are channel-specific
// (e.g., Slack). Send should return an error only for transient failures
// that warrant a retry on the next polling cycle.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}
```

- [ ] **Step 3: Write failing test for Slack sender message construction**

Create `internal/node/notifications/slack_test.go`:

```go
package notifications

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// capturedMessage records the arguments passed to PostMessageContext.
type capturedMessage struct {
	channelID string
	options   []slack.MsgOption
}

// fakeSlackClient captures PostMessageContext calls without hitting the Slack API.
type fakeSlackClient struct {
	messages []capturedMessage
	err      error
}

func (f *fakeSlackClient) PostMessageContext(ctx context.Context, channelID string, opts ...slack.MsgOption) (string, string, error) {
	f.messages = append(f.messages, capturedMessage{channelID: channelID, options: opts})
	return "", "", f.err
}

func TestSlackSenderSend(t *testing.T) {
	client := &fakeSlackClient{}
	sender := &SlackSender{
		client:    client,
		channelID: "C1234567890",
	}

	msg := Message{
		SessionID:   "sess-1",
		SessionName: "my-feature-branch",
		Provider:    "claude",
		WorkingDir:  "~/repos/my-project",
		UnreadSince: time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC),
		UnreadFor:   12 * time.Minute,
	}

	err := sender.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(client.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(client.messages))
	}
	if client.messages[0].channelID != "C1234567890" {
		t.Errorf("channelID = %q, want %q", client.messages[0].channelID, "C1234567890")
	}
}

func TestSlackSenderSendError(t *testing.T) {
	client := &fakeSlackClient{err: fmt.Errorf("slack API error")}
	sender := &SlackSender{
		client:    client,
		channelID: "C1234567890",
	}

	msg := Message{
		SessionID:   "sess-1",
		SessionName: "test",
		Provider:    "claude",
		WorkingDir:  "/tmp",
		UnreadSince: time.Now(),
		UnreadFor:   5 * time.Minute,
	}

	err := sender.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/node/notifications/ -run "TestSlackSender" -v`

Expected: compilation errors — `SlackSender` struct doesn't exist.

- [ ] **Step 5: Implement Slack sender**

Create `internal/node/notifications/slack.go`:

```go
package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/slack-go/slack"
)

// slackClient abstracts the Slack API for testing.
type slackClient interface {
	PostMessageContext(ctx context.Context, channelID string, options ...slack.MsgOption) (string, string, error)
}

// SlackSender sends notifications to a Slack channel using Block Kit formatting.
type SlackSender struct {
	client    slackClient
	channelID string
}

// NewSlackSender creates a SlackSender from a bot token and channel ID.
func NewSlackSender(botToken, channelID string) *SlackSender {
	return &SlackSender{
		client:    slack.New(botToken),
		channelID: channelID,
	}
}

// Send posts a Block Kit message to the configured Slack channel.
func (s *SlackSender) Send(ctx context.Context, msg Message) error {
	blocks := buildBlocks(msg)
	_, _, err := s.client.PostMessageContext(ctx, s.channelID,
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		return fmt.Errorf("slack send: %w", err)
	}
	return nil
}

// buildBlocks constructs Block Kit blocks for the notification message.
func buildBlocks(msg Message) []slack.Block {
	header := slack.NewHeaderBlock(
		slack.NewTextBlockObject(slack.PlainTextType, "Session waiting for attention", false, false),
	)

	fields := []*slack.TextBlockObject{
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Session:*\n%s", msg.SessionName), false, false),
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Provider:*\n%s", msg.Provider), false, false),
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Directory:*\n%s", msg.WorkingDir), false, false),
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Unread for:*\n%s", formatDuration(msg.UnreadFor)), false, false),
	}

	section := slack.NewSectionBlock(nil, fields, nil)

	return []slack.Block{header, section}
}

// formatDuration formats a duration into a human-readable string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/node/notifications/ -run "TestSlackSender" -v`

Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/node/notifications/sender.go internal/node/notifications/slack.go internal/node/notifications/slack_test.go
git commit -m "feat: add Sender interface and Slack sender with Block Kit formatting"
```

---

### Task 4: Create notification service with polling loop

**Files:**
- Create: `internal/node/notifications/service.go`
- Create: `internal/node/notifications/service_test.go`

- [ ] **Step 1: Write failing tests for the notification service**

Create `internal/node/notifications/service_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/node/notifications/ -run "TestService" -v`

Expected: compilation errors — `NewService`, `Service`, `NotificationDB` don't exist.

- [ ] **Step 3: Implement the notification service**

Create `internal/node/notifications/service.go`:

```go
package notifications

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bxnlabs/argus/internal/node/db"
)

const pollInterval = 30 * time.Second

const sqliteDatetimeFormat = "2006-01-02 15:04:05"

// NotificationDB abstracts database operations for the notification service.
type NotificationDB interface {
	UnreadSessions(ctx context.Context) ([]db.UnreadSession, error)
	HasNotification(ctx context.Context, sessionID, after string) (bool, error)
	InsertNotification(ctx context.Context, sessionID, sentAt string) error
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

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll(ctx)
		}
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
			SessionID:   sess.ID,
			SessionName: sess.Name,
			Provider:    sess.ProviderType,
			WorkingDir:  sess.WorkingDirectory,
			UnreadSince: unreadSince,
			UnreadFor:   unreadFor,
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/node/notifications/ -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/notifications/service.go internal/node/notifications/service_test.go
git commit -m "feat: add notification service with polling loop and deduplication"
```

---

### Task 5: Wire notification service into setup.go

**Files:**
- Modify: `internal/node/setup.go`

- [ ] **Step 1: Write the wiring code**

In `internal/node/setup.go`, add the notification service wiring. Add imports:

```go
"log"
"time"

"github.com/bxnlabs/argus/internal/node/notifications"
```

In the `Setup` function, after the `repoIndexer.Start(context.Background())` line and before the `handler := api.NewRouter(...)` line, add:

```go
// Notification service (optional — only started when a channel is configured).
var notifService *notifications.Service
if cfg.Notifications.Channel != "" {
	threshold, err := time.ParseDuration(cfg.Notifications.NotifyAfterUnreadFor)
	if err != nil {
		// Should not happen — validated in config.Load
		database.Close()
		return nil, nil, fmt.Errorf("parse notification threshold: %w", err)
	}

	var sender notifications.Sender
	switch cfg.Notifications.Channel {
	case "slack":
		sender = notifications.NewSlackSender(
			cfg.Notifications.Slack.BotToken,
			cfg.Notifications.Slack.ChannelID,
		)
	}

	notifService = notifications.NewService(sender, database, threshold)
	notifService.Start(context.Background())
	log.Printf("notification service started (channel=%s, threshold=%s)",
		cfg.Notifications.Channel, cfg.Notifications.NotifyAfterUnreadFor)
}
```

Update the cleanup function to close the notification service:

```go
cleanup := func() {
	if notifService != nil {
		notifService.Close()
	}
	watcherMgr.Close()
	repoIndexer.Close()
	database.Close()
}
```

- [ ] **Step 2: Verify `*db.DB` satisfies `NotificationDB` interface**

The `notifications.NotificationDB` interface requires:
- `UnreadSessions(ctx) ([]db.UnreadSession, error)`
- `HasNotification(ctx, sessionID, after) (bool, error)`
- `InsertNotification(ctx, sessionID, sentAt) error`

All three methods are defined on `*db.DB` in `internal/node/db/notifications.go`. The `database` variable in `setup.go` is `*db.DB`, so it directly satisfies the interface — no adapter needed.

- [ ] **Step 3: Run full build to verify compilation**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go build ./...`

Expected: builds successfully with no errors.

- [ ] **Step 4: Run all tests**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./internal/config/ ./internal/node/db/ ./internal/node/notifications/ -v`

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/setup.go
git commit -m "feat: wire notification service into node setup"
```

---

### Task 6: Run full test suite and verify

**Files:** (none — verification only)

- [ ] **Step 1: Run the full project test suite**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go test ./... -v -count=1`

Expected: all tests PASS. No regressions in existing tests.

- [ ] **Step 2: Verify go.mod is tidy**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--slack-notifications && go mod tidy && git diff go.mod go.sum`

If there are changes, commit them:

```bash
git add go.mod go.sum
git commit -m "chore: tidy go.mod"
```
