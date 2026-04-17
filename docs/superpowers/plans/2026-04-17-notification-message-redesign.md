# Notification Message Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the Slack notification message with visual hierarchy, git metadata, and deep-link support for sessions.

**Architecture:** Expand the `UnreadSession` DB query to include git metadata fields (`worktree_branch`, `git_parent_dir`, `git_remote_url`). Thread a `baseURL` from Tailscale FQDN through `setup.go` → `SlackSender`. Rewrite `buildBlocks()` with the new Block Kit layout. Add `?session=` deep-link handling in the React frontend.

**Tech Stack:** Go (Block Kit via slack-go/slack), React/TypeScript, SQLite, tsnet

---

### Task 1: Expand UnreadSession with git metadata

**Files:**
- Modify: `internal/node/db/notifications.go:6-12` (UnreadSession struct)
- Modify: `internal/node/db/notifications.go:16-28` (UnreadSessions query)
- Modify: `internal/node/db/db_test.go:460-511` (TestUnreadSessions)

- [ ] **Step 1: Update the UnreadSession struct**

Add nullable git fields to the struct in `internal/node/db/notifications.go`:

```go
// UnreadSession holds the fields needed by the notification service.
type UnreadSession struct {
	ID               string
	Name             string
	ProviderType     string
	WorkingDirectory string
	UnreadSince      string
	WorktreeBranch   *string
	GitParentDir     *string
	GitRemoteURL     *string
}
```

- [ ] **Step 2: Update the UnreadSessions SQL query**

Replace the query in `internal/node/db/notifications.go` `UnreadSessions` method:

```go
func (d *DB) UnreadSessions(ctx context.Context) ([]UnreadSession, error) {
	rows, err := d.sql.QueryContext(ctx,
		`SELECT id, name, provider_type, working_directory, unread_since,
		        worktree_branch, git_parent_dir, git_remote_url
		 FROM sessions WHERE unread_since IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []UnreadSession
	for rows.Next() {
		var s UnreadSession
		if err := rows.Scan(&s.ID, &s.Name, &s.ProviderType, &s.WorkingDirectory, &s.UnreadSince,
			&s.WorktreeBranch, &s.GitParentDir, &s.GitRemoteURL); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
```

- [ ] **Step 3: Update TestUnreadSessions to set git fields**

In `internal/node/db/db_test.go`, update `TestUnreadSessions`. After the existing session creation, update `s1` to have git metadata and verify it comes back from `UnreadSessions`:

```go
func TestUnreadSessions(t *testing.T) {
	db := testDB(t)
	if err := db.RunMigrations(); err != nil {
		t.Fatal(err)
	}

	// Create two sessions
	branch := "jeev/feature"
	parentDir := "/home/jeev/repos/myproject"
	remoteURL := "https://github.com/bxnlabs/argus.git"
	db.CreateSession(&Session{
		ID: "s1", Name: "session-1", TmuxName: "claude-s1",
		WorkingDirectory: "/tmp/proj1", ProviderType: "claude",
		WorktreeBranch: &branch, GitParentDir: &parentDir, GitRemoteURL: &remoteURL,
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
	if sessions[0].WorktreeBranch == nil || *sessions[0].WorktreeBranch != branch {
		t.Errorf("expected worktree_branch %q, got %v", branch, sessions[0].WorktreeBranch)
	}
	if sessions[0].GitParentDir == nil || *sessions[0].GitParentDir != parentDir {
		t.Errorf("expected git_parent_dir %q, got %v", parentDir, sessions[0].GitParentDir)
	}
	if sessions[0].GitRemoteURL == nil || *sessions[0].GitRemoteURL != remoteURL {
		t.Errorf("expected git_remote_url %q, got %v", remoteURL, sessions[0].GitRemoteURL)
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/node/db/ -run TestUnreadSessions -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/node/db/notifications.go internal/node/db/db_test.go
git commit -m "feat: add git metadata to UnreadSession query"
```

---

### Task 2: Update Message struct and service poll

**Files:**
- Modify: `internal/node/notifications/sender.go` (Message struct)
- Modify: `internal/node/notifications/service.go:132-139` (poll message construction)
- Modify: `internal/node/notifications/service_test.go` (mock DB sessions)

- [ ] **Step 1: Update the Message struct**

Replace the Message struct in `internal/node/notifications/sender.go`:

```go
// Message holds the data needed to compose a notification.
type Message struct {
	SessionID      string
	SessionName    string
	WorkingDir     string
	UnreadSince    time.Time
	UnreadFor      time.Duration
	WorktreeBranch *string
	GitParentDir   *string
	GitRemoteURL   *string
}
```

Note: `Provider` field is removed (per design spec — it's almost always "claude" and adds noise).

- [ ] **Step 2: Update poll() to pass git metadata**

In `internal/node/notifications/service.go`, update the `msg` construction in `poll()`:

```go
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
```

- [ ] **Step 3: Fix compilation errors in tests**

The mock DB's `UnreadSession` structs in `internal/node/notifications/service_test.go` reference `ProviderType` which is still a field on `UnreadSession` but `Provider` was removed from `Message`. The mock sender tests capture `Message` objects — update any assertions that reference `msg.Provider`. Since no tests assert on `msg.Provider`, the only change needed is removing `Provider` from the `msg` construction if it causes a compile error. The `ProviderType` field stays on `UnreadSession` — it's just not mapped to `Message` anymore.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/node/notifications/ -v`
Expected: PASS (all existing tests still work)

- [ ] **Step 5: Commit**

```bash
git add internal/node/notifications/sender.go internal/node/notifications/service.go internal/node/notifications/service_test.go
git commit -m "feat: add git metadata to notification Message, drop Provider"
```

---

### Task 3: Add path display helpers

**Files:**
- Create: `internal/node/notifications/paths.go`
- Create: `internal/node/notifications/paths_test.go`

- [ ] **Step 1: Write tests for path helpers**

Create `internal/node/notifications/paths_test.go`:

```go
package notifications

import "testing"

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

func TestCompressPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"home path", "/home/jeev/repos/myproject", "~/repos/myproject"},
		{"Users path", "/Users/jeev/Workspace/repos/foo", "~/Workspace/repos/foo"},
		{"no home prefix", "/tmp/foo", "/tmp/foo"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressHomePath(tt.input)
			if got != tt.expected {
				t.Errorf("compressHomePath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/node/notifications/ -run 'TestExtractRepoName|TestCompressPath' -v`
Expected: FAIL (functions not defined)

- [ ] **Step 3: Implement path helpers**

Create `internal/node/notifications/paths.go`:

```go
package notifications

import (
	"path"
	"regexp"
	"strings"
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
	p := strings.TrimSuffix(remoteURL, ".git")
	// Take last two path segments as owner/repo
	p = strings.TrimRight(p, "/")
	parts := strings.Split(p, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-2] + "/" + parts[len(parts)-1]
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return ""
}

// homePattern matches /home/<user>/ or /Users/<user>/ prefixes.
var homePattern = regexp.MustCompile(`^/(home|Users)/[^/]+/`)

// compressHomePath replaces /home/<user>/ or /Users/<user>/ with ~/.
func compressHomePath(p string) string {
	if p == "" {
		return ""
	}
	return homePattern.ReplaceAllString(p, "~/")
}

// buildLocationLine constructs the repo/path display string for a notification.
// Returns (repoLine, localPathLine, branchLine). Any may be empty.
func buildLocationLine(gitRemoteURL, gitParentDir, workingDir *string, worktreeBranch *string) (repo, localPath, branch string) {
	if gitRemoteURL != nil && *gitRemoteURL != "" {
		repo = extractRepoName(*gitRemoteURL)
		if gitParentDir != nil && *gitParentDir != "" {
			localPath = compressHomePath(*gitParentDir)
		}
	} else if gitParentDir != nil && *gitParentDir != "" {
		repo = compressHomePath(*gitParentDir)
		// localPath omitted — would be redundant
	} else if workingDir != nil && *workingDir != "" {
		repo = compressHomePath(*workingDir)
	}

	if worktreeBranch != nil && *worktreeBranch != "" {
		branch = *worktreeBranch
	}

	return repo, localPath, branch
}
```

Note: `buildLocationLine` takes `workingDir` as `*string` — the caller will pass `&msg.WorkingDir`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/node/notifications/ -run 'TestExtractRepoName|TestCompressPath' -v`
Expected: PASS

- [ ] **Step 5: Add test for buildLocationLine**

Add to `internal/node/notifications/paths_test.go`:

```go
func TestBuildLocationLine(t *testing.T) {
	s := func(v string) *string { return &v }

	tests := []struct {
		name                              string
		remoteURL, parentDir, workingDir  *string
		branch                            *string
		wantRepo, wantLocal, wantBranch   string
	}{
		{
			name:      "full git metadata",
			remoteURL: s("https://github.com/bxnlabs/argus.git"),
			parentDir: s("/home/jeev/repos/argus"),
			workingDir: s("/home/jeev/.argus/projects/foo/worktrees/bar"),
			branch:    s("jeev/feature"),
			wantRepo:  "bxnlabs/argus",
			wantLocal: "~/repos/argus",
			wantBranch: "jeev/feature",
		},
		{
			name:       "no remote, has parent dir",
			remoteURL:  nil,
			parentDir:  s("/Users/jeev/Workspace/repos/myproject"),
			workingDir: s("/tmp/foo"),
			branch:     nil,
			wantRepo:   "~/Workspace/repos/myproject",
			wantLocal:  "",
			wantBranch: "",
		},
		{
			name:       "only working dir",
			remoteURL:  nil,
			parentDir:  nil,
			workingDir: s("/tmp/project"),
			branch:     nil,
			wantRepo:   "/tmp/project",
			wantLocal:  "",
			wantBranch: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, local, branch := buildLocationLine(tt.remoteURL, tt.parentDir, tt.workingDir, tt.branch)
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if local != tt.wantLocal {
				t.Errorf("local = %q, want %q", local, tt.wantLocal)
			}
			if branch != tt.wantBranch {
				t.Errorf("branch = %q, want %q", branch, tt.wantBranch)
			}
		})
	}
}
```

- [ ] **Step 6: Run all tests**

Run: `go test ./internal/node/notifications/ -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/node/notifications/paths.go internal/node/notifications/paths_test.go
git commit -m "feat: add path display helpers for notification messages"
```

---

### Task 4: Rewrite Slack Block Kit message

**Files:**
- Modify: `internal/node/notifications/slack.go` (buildBlocks, SlackSender, NewSlackSender)
- Modify: `internal/node/notifications/slack_test.go`

- [ ] **Step 1: Update SlackSender to accept baseURL**

In `internal/node/notifications/slack.go`, add a `baseURL` field to `SlackSender` and update `NewSlackSender`:

```go
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
```

- [ ] **Step 2: Rewrite buildBlocks with the new layout**

Replace the `buildBlocks` function in `internal/node/notifications/slack.go`:

```go
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
```

- [ ] **Step 3: Update Send to pass baseURL**

Update the `Send` method in `internal/node/notifications/slack.go`:

```go
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
```

- [ ] **Step 4: Add `strings` import if not already present**

The `buildBlocks` function uses `strings.Join`. Verify `strings` is in the import block (it was added for `escapeSlack` — confirm it's still there).

- [ ] **Step 5: Update slack tests**

In `internal/node/notifications/slack_test.go`, update `TestSlackSenderSend` and `TestSlackSenderSendError` — the `SlackSender` now takes `baseURL` as third field:

```go
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
```

- [ ] **Step 6: Add test for buildBlocks with and without baseURL**

Add to `internal/node/notifications/slack_test.go`:

```go
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
		WorkingDir:  "/tmp/proj",
		UnreadFor:   6 * time.Minute,
	}

	blocks := buildBlocks(msg, "")
	// Header + Session (no button) + Divider + Duration = 4 blocks (no context — no git metadata)
	if len(blocks) != 4 {
		t.Errorf("expected 4 blocks without git metadata, got %d", len(blocks))
	}
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/node/notifications/ -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/node/notifications/slack.go internal/node/notifications/slack_test.go
git commit -m "feat: redesign Slack notification with visual hierarchy and deep links"
```

---

### Task 5: Wire Tailscale FQDN into setup

**Files:**
- Modify: `internal/tailscale/server.go` (expose FQDN)
- Modify: `internal/node/setup.go` (accept baseURL, pass to SlackSender)
- Modify: `cmd/argus/main.go` (derive baseURL from tsnet, pass to Setup)

- [ ] **Step 1: Add FQDN method to tailscale Server**

In `internal/tailscale/server.go`, add a method to get the FQDN after the server is up. The `tsnet.Server` has a `CertDomains()` method that returns the server's Tailscale FQDN(s):

```go
// FQDN returns the Tailscale fully-qualified domain name after Up() succeeds.
// Returns empty string if the server hasn't started or has no cert domains.
func (s *Server) FQDN() string {
	if !s.started {
		return ""
	}
	domains := s.ts.CertDomains()
	if len(domains) == 0 {
		return ""
	}
	return domains[0]
}
```

- [ ] **Step 2: Update makeListeners to return FQDN**

In `cmd/argus/main.go`, change `makeListeners` to return the FQDN as an additional return value. Update the signature:

```go
func makeListeners(ctx context.Context, tsCfg config.TailscaleConfig, bindAddress string, port int, mode string) (listeners []net.Listener, discoveryAddr string, tsFQDN string, tsCloser func() error, err error)
```

In the non-Tailscale paths, return empty FQDN:
```go
return lns, lns[1].Addr().String(), "", nil, nil
```
```go
return lns, net.JoinHostPort("127.0.0.1", actualPort), "", nil, nil
```
```go
return lns, lns[0].Addr().String(), "", nil, nil
```

In the Tailscale path, after `tsServer.Up()` succeeds, get the FQDN:
```go
fqdn := tsServer.FQDN()
```

And return it:
```go
return []net.Listener{loopbackLn, tsLn}, discoveryAddr, fqdn, tsServer.Close, nil
```

- [ ] **Step 3: Update all makeListeners call sites**

In `cmd/argus/main.go`, update both call sites to handle the new return value:

In `newNodeCmd()`:
```go
listeners, discoveryAddr, tsFQDN, tsCloser, err := makeListeners(cmd.Context(), cfg.Tailscale, cfg.Node.BindAddress, cfg.Node.Port, "node")
```

In `runCombined()`:
```go
listeners, discoveryAddr, tsFQDN, tsCloser, err := makeListeners(ctx, cfg.Tailscale, cfg.Server.BindAddress, cfg.Server.Port, "combined")
```

For both, compute the baseURL and pass it to `node.Setup`:
```go
var baseURL string
if tsFQDN != "" {
    baseURL = fmt.Sprintf("https://%s:%d", tsFQDN, cfg.Server.Port)
}
nodeHandler, cleanup, err := node.Setup(cfg, baseURL)
```

For the node-only mode, use `cfg.Node.Port` instead of `cfg.Server.Port`:
```go
if tsFQDN != "" {
    baseURL = fmt.Sprintf("https://%s:%d", tsFQDN, cfg.Node.Port)
}
```

- [ ] **Step 4: Update node.Setup signature**

In `internal/node/setup.go`, add `baseURL string` parameter:

```go
func Setup(cfg *config.Config, baseURL string) (http.Handler, func(), error) {
```

And pass it to `NewSlackSender`:

```go
case "slack":
    sender = notifications.NewSlackSender(
        cfg.Notifications.Slack.BotToken,
        cfg.Notifications.Slack.ChannelID,
        baseURL,
    )
```

- [ ] **Step 5: Run Go tests**

Run: `go test ./internal/node/notifications/ ./internal/tailscale/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tailscale/server.go internal/node/setup.go cmd/argus/main.go
git commit -m "feat: derive Tailscale FQDN for notification deep links"
```

---

### Task 6: Add deep-link handling in React frontend

**Files:**
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Add session query param handling**

In `web/src/App.tsx`, add a `useEffect` after the existing stale-tab cleanup effect (around line 54). This reads the `?session=` query param, finds the matching session, and auto-attaches:

```tsx
// Deep-link: auto-attach session from ?session= query param (e.g. from Slack notification)
const deepLinkHandled = useRef(false);
useEffect(() => {
  if (!sessionsLoaded || deepLinkHandled.current) return;

  const params = new URLSearchParams(window.location.search);
  const sessionId = params.get("session");
  if (!sessionId) return;

  deepLinkHandled.current = true;

  const session = sessions.find((s) => s.id === sessionId);
  if (session) {
    attachToSession(session);
    // Open sidebar on mobile so user can see the session
    if (isMobile) setSidebarOpen(true);
  }

  // Clear query param to avoid re-triggering on refresh
  const url = new URL(window.location.href);
  url.searchParams.delete("session");
  window.history.replaceState({}, "", url.pathname + url.hash);
}, [sessionsLoaded, sessions, attachToSession, isMobile]);
```

- [ ] **Step 2: Verify the frontend builds**

Run: `cd web && npm run build`
Expected: Build succeeds with no TypeScript errors

- [ ] **Step 3: Commit**

```bash
git add web/src/App.tsx
git commit -m "feat: add deep-link support for ?session= query param"
```

---

### Task 7: Push and update PR

**Files:** None (git operations only)

- [ ] **Step 1: Run full test suite**

Run: `go test ./internal/config/ ./internal/node/db/ ./internal/node/notifications/ ./internal/tailscale/ -count=1`
Expected: All PASS

- [ ] **Step 2: Push all commits**

```bash
git push
```

- [ ] **Step 3: Update the PR description**

Update PR #73 body to reflect the redesign work included in the branch.
