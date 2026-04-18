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
		baseURL:   "https://argus.tail123.ts.net:3000",
	}

	msg := Message{
		SessionID:   "sess-1",
		SessionName: "my-feature-branch",
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

func TestEscapeSlack(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{"<!channel>", "&lt;!channel&gt;"},
		{"A & B", "A &amp; B"},
		{"<a&b>", "&lt;a&amp;b&gt;"},
	}
	for _, tt := range tests {
		got := escapeSlack(tt.input)
		if got != tt.want {
			t.Errorf("escapeSlack(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
		WorkingDir:  "/tmp",
		UnreadSince: time.Now(),
		UnreadFor:   5 * time.Minute,
	}

	err := sender.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildBlocksWithBaseURL(t *testing.T) {
	remoteURL := "https://github.com/bxnlabs/argus.git"
	branch := "jeev/feature"
	msg := Message{
		SessionID:      "sess-1",
		SessionName:    "my-session",
		WorkingDir:     "/tmp/proj",
		UnreadFor:      6 * time.Minute,
		GitRemoteURL:   &remoteURL,
		WorktreeBranch: &branch,
	}

	blocks := buildBlocks(msg, "https://argus.ts.net:3000")
	// Header + Session (with button) + Divider + Context + Duration = 5 blocks
	if len(blocks) != 5 {
		t.Errorf("expected 5 blocks with baseURL, got %d", len(blocks))
	}
}

func TestBuildBlocksWithoutBaseURL(t *testing.T) {
	msg := Message{
		SessionID:   "sess-1",
		SessionName: "my-session",
		UnreadFor:   6 * time.Minute,
	}

	blocks := buildBlocks(msg, "")
	// Header + Session (no button) + Divider + Duration = 4 blocks (no context — no git metadata)
	if len(blocks) != 4 {
		t.Errorf("expected 4 blocks without git metadata, got %d", len(blocks))
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"https with .git", "https://github.com/bxnlabs/argus.git", "bxnlabs/argus"},
		{"https without .git", "https://github.com/flyteorg/flyte-sdk", "flyteorg/flyte-sdk"},
		{"ssh url", "git@github.com:bxnlabs/argus.git", "bxnlabs/argus"},
		{"single segment", "https://github.com/argus.git", "argus"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRepoName(tt.input)
			if got != tt.expected {
				t.Errorf("extractRepoName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
