# Slack Notifications for Unread Sessions

## Overview

Add support for Slack notifications that are sent when sessions have been unread for a configurable duration. This extends Argus's existing browser-based notification system with an external channel for sessions the user may have missed.

## Goals

- Notify the user via Slack when a session has been unread beyond a configurable threshold
- Send a single notification per unread event (no repeated reminders)
- Keep the system extensible for future notification channels
- All configuration via `config.toml` — no UI for notification settings

## Non-Goals

- Interactive Slack messages (e.g., "Mark as Read" buttons) — Argus runs on a private Tailscale network and may not be reachable from Slack's servers
- Per-session notification overrides — a single global threshold applies
- Multiple simultaneous notification channels — one active channel at a time
- Notification preferences UI in the web frontend

## Configuration

New `[notifications]` section in `~/.argus/config.toml`:

```toml
[notifications]
# Which channel to use: "slack" (or empty to disable)
channel = ""
# Duration a session must be unread before notifying
notify_after_unread_for = "5m"

# Slack-specific (required when channel = "slack")
[notifications.slack]
bot_token = ""
channel_id = ""
```

- Notifications are disabled by default (`channel = ""`)
- When `channel = "slack"`, `bot_token` and `channel_id` are validated as non-empty
- `notify_after_unread_for` is a Go-parseable duration string (e.g., `"5m"`, `"30s"`, `"10m"`)
- Environment variable overrides follow existing convention: `ARGUS_NOTIFICATIONS_CHANNEL`, `ARGUS_NOTIFICATIONS_SLACK_BOT_TOKEN`, etc.

### Config Structs

```go
type NotificationsConfig struct {
    Channel              string            `mapstructure:"channel"`
    NotifyAfterUnreadFor string            `mapstructure:"notify_after_unread_for"`
    Slack                SlackNotifyConfig `mapstructure:"slack"`
}

type SlackNotifyConfig struct {
    BotToken  string `mapstructure:"bot_token"`
    ChannelID string `mapstructure:"channel_id"`
}
```

Validation: if `Channel == "slack"`, require `Slack.BotToken` and `Slack.ChannelID` to be non-empty. Future channels add a new nested struct and validation branch.

## Database

### New Table: `notifications`

```sql
CREATE TABLE notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    sent_at TEXT NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
)
```

- One row per notification sent for an unread event
- No `channel` column — all registered channels fire together, tracked as a single event
- Deduplication: a notification row with `sent_at > session.unread_since` means the current unread event has already been notified

### Migration

Added as a new entry in `allMigrations` in `internal/node/db/migrations.go`.

### DB Methods

New methods in `internal/node/db/notifications.go`:

- `UnreadSessions(ctx) ([]Session, error)` — returns sessions where `unread_since IS NOT NULL`
- `HasNotification(ctx, sessionID, afterTimestamp) (bool, error)` — checks if a notification exists with `sent_at > afterTimestamp` for the given session
- `InsertNotification(ctx, sessionID, sentAt) error` — inserts a notification row

## Notification Service

New package: `internal/node/notifications/`

### Sender Interface

```go
type Sender interface {
    Send(ctx context.Context, msg Message) error
}

type Message struct {
    SessionID   string
    SessionName string
    Provider    string
    WorkingDir  string
    UnreadSince time.Time
    UnreadFor   time.Duration
}
```

### Service

`Service` struct in `internal/node/notifications/service.go`:

- Holds a `Sender`, DB interface, and config
- `Start(ctx)` launches a polling goroutine
- `Close()` cancels and waits for the goroutine to exit
- Follows the same lifecycle pattern as `WatcherManager` and `RepoIndexer`

**Polling loop** (30-second interval):

1. Query all sessions where `unread_since IS NOT NULL`
2. Filter to those where `now - unread_since > notify_after_unread_for`
3. For each eligible session, check `HasNotification(sessionID, unread_since)` — if a notification with `sent_at > unread_since` exists, skip
4. Call `sender.Send()` with the session details
5. On success, call `InsertNotification(sessionID, now)`
6. On failure, log the error and skip the insert — the session remains eligible and will be retried on the next tick

### DB Interface

```go
type UnreadSession struct {
    ID          string
    Name        string
    Provider    string
    WorkingDir  string
    UnreadSince string // SQLite datetime
}

type NotificationDB interface {
    UnreadSessions(ctx context.Context) ([]UnreadSession, error)
    HasNotification(ctx context.Context, sessionID string, after string) (bool, error)
    InsertNotification(ctx context.Context, sessionID, sentAt string) error
}
```

## Slack Sender

`internal/node/notifications/slack.go`

### Dependency

`slack-go/slack` Go library added to `go.mod`.

### Message Format

Rich Block Kit message:

```
┌─────────────────────────────────────────┐
│ Session waiting for attention           │  <- header
│                                         │
│ Session:    my-feature-branch           │  <- section fields
│ Provider:   Claude                      │
│ Directory:  ~/repos/my-project          │
│ Unread for: 12 minutes                  │
└─────────────────────────────────────────┘
```

Built with Slack `SectionBlock` and `Fields`. No interactive elements.

### Error Handling

If `Send()` fails, the error is returned to the service. No exponential backoff — the 30-second polling loop provides natural retry.

## Wiring

In `internal/node/setup.go`:

1. Check if `cfg.Notifications.Channel` is non-empty
2. Create the appropriate sender (currently only `SlackSender`)
3. Create `notifications.NewService(sender, notificationDB, cfg.Notifications)`
4. Call `service.Start(ctx)`
5. Add `service.Close()` to the cleanup function

No new API endpoints.

## File Changes

### New Files

| File | Purpose |
|------|---------|
| `internal/node/notifications/service.go` | Polling loop and orchestration |
| `internal/node/notifications/slack.go` | Slack sender implementation |
| `internal/node/db/notifications.go` | Notification DB methods |

### Modified Files

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `NotificationsConfig`, `SlackNotifyConfig` structs, defaults, validation |
| `internal/node/db/migrations.go` | Add migration for `notifications` table |
| `internal/node/setup.go` | Wire up notification service |
| `go.mod` / `go.sum` | Add `slack-go/slack` dependency |

## Testing Strategy

- **Unit tests** for the notification service: mock the `Sender` and `NotificationDB` interfaces. Verify polling logic, deduplication, and error handling.
- **Unit tests** for the Slack sender: mock the Slack API client to verify Block Kit message construction.
- **Unit tests** for DB methods: use the existing in-memory SQLite test pattern.
- **Config validation tests**: verify that missing Slack fields are rejected when `channel = "slack"`.
