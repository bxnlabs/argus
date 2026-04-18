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
	baseURL   string
}

// NewSlackSender creates a SlackSender from a bot token, channel ID, and optional base URL for deep links.
func NewSlackSender(botToken, channelID, baseURL string) *SlackSender {
	return &SlackSender{
		client:    slack.New(botToken),
		channelID: channelID,
		baseURL:   baseURL,
	}
}

// Send posts a Block Kit message to the configured Slack channel.
func (s *SlackSender) Send(ctx context.Context, msg Message) error {
	blocks := buildBlocks(msg, s.baseURL)
	_, _, err := s.client.PostMessageContext(ctx, s.channelID,
		slack.MsgOptionBlocks(blocks...),
	)
	if err != nil {
		return fmt.Errorf("slack send: %w", err)
	}
	return nil
}

// buildBlocks constructs Block Kit blocks for the notification message.
func buildBlocks(msg Message, baseURL string) []slack.Block {
	var blocks []slack.Block

	// Header
	header := slack.NewHeaderBlock(
		slack.NewTextBlockObject(slack.PlainTextType, "\U0001f514 Session waiting for attention", true, false),
	)
	blocks = append(blocks, header)

	// Session name + ID section
	sessionText := fmt.Sprintf("*%s*\nID: `%s`", escapeSlack(msg.SessionName), escapeSlack(msg.SessionID))
	sessionSection := slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType, sessionText, false, false),
		nil, nil,
	)
	if baseURL != "" {
		link := fmt.Sprintf("%s?session=%s", baseURL, msg.SessionID)
		sessionSection.Accessory = slack.NewAccessory(
			slack.NewButtonBlockElement("view_session", msg.SessionID,
				slack.NewTextBlockObject(slack.PlainTextType, "View in Argus \u2192", true, false),
			).WithURL(link),
		)
	}
	blocks = append(blocks, sessionSection)

	// Divider
	blocks = append(blocks, slack.NewDividerBlock())

	// Location context: repo, local path, branch
	workingDir := msg.WorkingDir
	repo, localPath, branch := buildLocationLine(msg.GitRemoteURL, msg.GitParentDir, &workingDir, msg.WorktreeBranch)

	var contextParts []string
	if repo != "" {
		line := fmt.Sprintf("\U0001f4c2  %s", escapeSlack(repo))
		if localPath != "" {
			line += fmt.Sprintf("\n      %s", escapeSlack(localPath))
		}
		contextParts = append(contextParts, line)
	}
	if branch != "" {
		contextParts = append(contextParts, fmt.Sprintf("\U0001f500  %s", escapeSlack(branch)))
	}

	if len(contextParts) > 0 {
		contextText := strings.Join(contextParts, "\n")
		contextSection := slack.NewSectionBlock(
			slack.NewTextBlockObject(slack.MarkdownType, contextText, false, false),
			nil, nil,
		)
		blocks = append(blocks, contextSection)
	}

	// Unread duration
	durationSection := slack.NewSectionBlock(
		slack.NewTextBlockObject(slack.MarkdownType,
			fmt.Sprintf("\u23f3  Unread for %s", formatDuration(msg.UnreadFor)), false, false),
		nil, nil,
	)
	blocks = append(blocks, durationSection)

	return blocks
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
