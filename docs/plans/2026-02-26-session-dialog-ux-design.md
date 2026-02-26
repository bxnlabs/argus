# New Session Dialog UX Improvements

## Summary

Four UX improvements to the new session dialog:

1. Replace separate Directory input + folder button with a single clickable Source field that opens a modal picker
2. Unified SourcePicker modal with Local/Remote tabs
3. Remote mode: searchable list of GitHub repos from user's orgs, plus free-text entry for arbitrary repos
4. Name field becomes required (names double as branch names)

## New Session Dialog

The form simplifies to:

- **Agent** - dropdown (unchanged)
- **Name** - required text input, no longer optional
- **Source** - single clickable field that opens the SourcePicker modal; displays the selected value (tilde-contracted path or `org/repo`)
- **Auto-approve** - toggle (unchanged)

Removed: Local/Remote tab switcher from the form, FolderOpen button, separate Directory/Repository inputs.

The Create button is disabled when Name is empty.

## SourcePicker Modal

Replaces `DirectoryPicker`. A modal dialog with two tabs:

### Local Tab

Reuses the existing `FileBrowser` component unchanged. Same fuzzy search, breadcrumb navigation, keyboard navigation. Selecting a directory closes the modal and sets the Source field to the tilde-contracted path.

### Remote Tab

New `RepoBrowser` component:

- Search input at top (same styling as FileBrowser)
- Flat scrollable list of `org/repo` entries fetched from backend
- Client-side fuzzy filtering as user types
- Keyboard navigation (arrow keys, Enter) matching FileBrowser behavior
- Selecting a repo closes the modal and sets Source to `org/repo`
- Free-text fallback: when search doesn't match known repos, user can press Enter to submit the typed text as a custom repo URL
- Handles empty state gracefully: if no GitHub token configured, shows the free-text input directly with a hint about configuring a token

The modal remembers the last-active tab across opens within a session.

## Backend: GitHub Repos API

### Config

Add `github_token` to `~/.argus/config.toml`:

```toml
github_token = "ghp_xxxxxxxxxxxx"
```

`Config` struct gains: `GitHubToken string \`toml:"github_token"\``

### Endpoint

`GET /agent/api/github/repos`

- Reads token from config at request time
- If no token: returns `200` with `{"repos": []}`
- Uses `google/go-github` SDK to fetch:
  1. Authenticated user's repos (`client.Repositories.ListByAuthenticatedUser`)
  2. User's orgs (`client.Organizations.List`)
  3. Each org's repos (`client.Repositories.ListByOrg`)
- Returns flat JSON: `{"repos": ["bxnlabs/argus", "bxnlabs/infra", ...]}`
- Sorted alphabetically
- Handles pagination via the SDK's built-in pagination helpers

### Frontend Query

New React Query hook `useGitHubReposQuery()`:
- Fetches from `/agent/api/github/repos`
- Stale time: 60 seconds
- `RepoBrowser` filters this cached list client-side

## Files to Change

### Backend (Go)
- `internal/config/config.go` - add `GitHubToken` field
- `internal/github/repos.go` (new) - GitHub repo listing logic using go-github SDK
- `internal/agent/api/github.go` (new) - HTTP handler for `/agent/api/github/repos`
- `internal/agent/api/routes.go` - register new route
- `go.mod` / `go.sum` - add `google/go-github` dependency

### Frontend (TypeScript/React)
- `web/src/components/NewSessionDialog/index.tsx` - simplify form, add Source field, make Name required
- `web/src/components/DirectoryPicker.tsx` - evolve into `SourcePicker.tsx` with tabs
- `web/src/components/RepoBrowser.tsx` (new) - GitHub repo list with fuzzy search
- `web/src/data/github/queries.ts` (new) - React Query hook for GitHub repos
- `web/src/data/github/keys.ts` (new) - query key constants
