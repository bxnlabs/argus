# Slack Notification Message Redesign

## Problem

The current notification message is a plain 2x2 field grid with no visual hierarchy. The directory field shows an argus-internal worktree path instead of the real repo. All fields have equal visual weight, making it hard to scan. There's no link to the session, no emoji, and no personality.

## Design

### Message Layout

```
🔔  Session waiting for attention

    zero-trust-testdrive
    ID: abc-123-def                     [View in Argus →]

    ──────────────────────────────────

    📂  flyteorg/flyte-sdk
        ~/Workspace/repos/flyteorg/flyte-sdk
    🔀  jeev/zero-trust-testdrive

    ⏳  Unread for 6 minutes
```

### Block Kit Structure

1. **Header block** — `🔔 Session waiting for attention`
2. **Section block** — Session name (bold) + ID (muted monospace). When a deep link is available, a "View in Argus" button appears as a right-aligned accessory.
3. **Divider block**
4. **Context block** — Repo, local path, and branch as a compact multi-line context element.
5. **Section block** — `⏳ Unread for {duration}` as the urgency signal at the bottom.

### Icons

| Element  | Emoji |
| -------- | ----- |
| Header   | 🔔    |
| Repo     | 📂    |
| Branch   | 🔀    |
| Duration | ⏳    |

### Information Hierarchy

Top to bottom, most to least important:

1. **Session name + ID** — what's waiting, with a direct link to act on it
2. **Repo + path + branch** — location context
3. **Unread duration** — urgency signal

Provider is intentionally omitted — it's almost always "claude" and adds noise.

### Path Display Logic

- **Repo line**: Extract `owner/repo` from `git_remote_url` (strip `.git` suffix, take last two path segments). Fall back to compressed `git_parent_dir` (relative to `~`) if no remote URL. Fall back to `working_directory` if neither is available.
- **Local path line**: Show `git_parent_dir` compressed relative to `~`. Omit if unavailable or if it would be redundant with the repo line (i.e., no `git_remote_url` and `git_parent_dir` is already shown as the repo line).
- **Branch line**: Show `worktree_branch` if set. Omit entirely if null.

## Data Changes

### UnreadSession Query

Add `worktree_branch`, `git_parent_dir`, and `git_remote_url` to the `UnreadSessions` query and `UnreadSession` struct. These are nullable fields — use `*string`.

### Message Struct

Add to `Message`:
- `WorktreeBranch *string`
- `GitParentDir *string`
- `GitRemoteURL *string`

### Slack Sender

The `SlackSender` needs to know the base URL for deep links. Add a `baseURL` field (string, may be empty). When non-empty, the session section includes an "View in Argus" button linking to `{baseURL}?session={sessionID}`. When empty, no button — just the name and ID.

## Deep Link Support

### URL Format

```
https://{tailscale_hostname}:{server_port}?session={session_id}
```

### Base URL Construction

Derived at startup when Tailscale is enabled:
- The tsnet `Server` is already started in `main.go` via `makeListeners`. After `Up()`, call `CertDomains()` on the `tsnet.Server` to get the FQDN (e.g. `argus-macbook.tailnet-name.ts.net`).
- Pass the FQDN from `main.go` into `node.Setup()` as a new parameter. Combine with `server.port` to form `https://{fqdn}:{port}`.
- If Tailscale is not enabled, `baseURL` is empty and no link is shown.

No new config fields required.

### Frontend Change

On app load, read the `session` query parameter from `window.location.search`. If present:
1. Find the matching session by ID
2. Auto-attach to it (calls `attachToSession`)
3. Clear the query parameter from the URL (via `history.replaceState`) to avoid re-triggering on refresh

This is a small change in `App.tsx` within the existing session-load effect.

## Scope

- Redesign the Slack Block Kit message in `slack.go`
- Expand `UnreadSession` / `Message` structs with git metadata
- Update `UnreadSessions` SQL query
- Construct deep link base URL from Tailscale state in `setup.go`
- Pass base URL through to `SlackSender`
- Add `?session=` query param handling in the React app
- Update existing tests
