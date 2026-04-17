package notifications

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/slack-go/slack"
)

// escapeSlack escapes characters that are special in Slack mrkdwn.
func escapeSlack(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

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
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Session:*\n%s", escapeSlack(msg.SessionName)), false, false),
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Provider:*\n%s", escapeSlack(msg.Provider)), false, false),
		slack.NewTextBlockObject(slack.MarkdownType, fmt.Sprintf("*Directory:*\n%s", escapeSlack(msg.WorkingDir)), false, false),
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
