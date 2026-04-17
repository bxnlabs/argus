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
