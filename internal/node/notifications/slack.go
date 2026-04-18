package notifications

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/bxnlabs/argus/internal/shared"
	"github.com/slack-go/slack"
)

// sshURLPattern matches git@host:owner/repo.git style URLs.
var sshURLPattern = regexp.MustCompile(`^git@[^:]+:(.+?)(?:\.git)?$`)

// extractRepoName extracts "owner/repo" from a git remote URL.
// Handles both HTTPS and SSH URLs. Returns empty string if input is empty.
func extractRepoName(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}

	// Try SSH format first: git@github.com:owner/repo.git
	if m := sshURLPattern.FindStringSubmatch(remoteURL); len(m) == 2 {
		return m[1]
	}

	// HTTPS format: https://github.com/owner/repo.git
	// Strip scheme (e.g. "https://") before splitting on "/"
	p := strings.TrimSuffix(remoteURL, ".git")
	p = strings.TrimRight(p, "/")
	if idx := strings.Index(p, "://"); idx >= 0 {
		p = p[idx+3:] // skip past "://"
	}
	// p is now "github.com/owner/repo" or "github.com/repo"
	// Drop the host segment and take the remaining path segments
	parts := strings.SplitN(p, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return ""
	}
	pathParts := strings.Split(parts[1], "/")
	if len(pathParts) >= 2 {
		return pathParts[len(pathParts)-2] + "/" + pathParts[len(pathParts)-1]
	}
	return pathParts[0]
}

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
	home, _ := os.UserHomeDir()
	compress := func(p string) string {
		return shared.CompressPath(p, home, 1000) // high threshold = tilde-shorten only
	}

	var repo, localPath string
	if msg.GitRemoteURL != nil && *msg.GitRemoteURL != "" {
		repo = extractRepoName(*msg.GitRemoteURL)
		if msg.GitParentDir != nil && *msg.GitParentDir != "" {
			localPath = compress(*msg.GitParentDir)
		}
	} else if msg.GitParentDir != nil && *msg.GitParentDir != "" {
		repo = compress(*msg.GitParentDir)
	} else if msg.WorkingDir != "" {
		repo = compress(msg.WorkingDir)
	}

	var contextParts []string
	if repo != "" {
		line := fmt.Sprintf("\U0001f4c2  %s", escapeSlack(repo))
		if localPath != "" {
			line += fmt.Sprintf("\n      %s", escapeSlack(localPath))
		}
		contextParts = append(contextParts, line)
	}
	if msg.WorktreeBranch != nil && *msg.WorktreeBranch != "" {
		contextParts = append(contextParts, fmt.Sprintf("\U0001f500  %s", escapeSlack(*msg.WorktreeBranch)))
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
