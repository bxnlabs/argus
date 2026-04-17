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
