# Session Directory & Branch Display — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Show the git parent directory and branch for worktree sessions (or working directory for non-git sessions) in both the CLI and web UI.

**Architecture:** The API handler computes a `git_parent_dir` field at response time using the existing `git.FindMainRepo()`. Clients (CLI + web UI) each implement path compression independently: tilde-shorten, then keep first + last segments with `/.../` in the middle. No DB schema change.

**Tech Stack:** Go (API + CLI), TypeScript/React (web UI), Tailwind CSS

---

### Task 1: Add `git_parent_dir` to API session response

**Files:**
- Modify: `internal/agent/api/sessions.go:18-28` (list handler), `:60-84` (get handler), `:30-57` (create handler)

**Step 1: Add `sessionResponse` wrapper and enrichment helper**

In `internal/agent/api/sessions.go`, add a response wrapper type and a function that computes `git_parent_dir` for a single session. Place it above the handler methods:

```go
import (
	"github.com/bxnlabs/argus/internal/git"
)

// sessionResponse extends db.Session with computed display fields.
type sessionResponse struct {
	*db.Session
	GitParentDir *string `json:"git_parent_dir"`
}

// enrichSession computes display fields for a session response.
func enrichSession(s *db.Session) sessionResponse {
	resp := sessionResponse{Session: s}
	if s.WorktreeBranch != nil {
		if dir, err := git.FindMainRepo(s.WorkingDirectory); err == nil {
			resp.GitParentDir = &dir
		}
	}
	return resp
}
```

**Step 2: Update the `list` handler to use `sessionResponse`**

Replace the direct `sessions` return with enriched responses. Also include `home_dir` at the top level so clients can do tilde shortening without an extra API call:

```go
func (h *sessionHandler) list(w http.ResponseWriter, r *http.Request) {
	sessions, err := h.manager.List(r.Context())
	if err != nil {
		respondInternalError(w, err)
		return
	}
	resp := make([]sessionResponse, len(sessions))
	for i, s := range sessions {
		resp[i] = enrichSession(s)
	}
	home, _ := os.UserHomeDir()
	respondJSON(w, http.StatusOK, map[string]any{
		"sessions": resp,
		"home_dir": home,
	})
}
```

**Step 3: Update the `get` handler similarly**

Replace the direct `session` return:

```go
respondJSON(w, http.StatusOK, map[string]any{"session": enrichSession(session)})
```

Apply the same pattern to the `create` and `update` handlers.

**Step 4: Run existing tests**

Run: `go test ./internal/agent/api/... -v -count=1`
Expected: All existing tests pass (no behavior change for tests that don't check for the new field).

**Step 5: Commit**

```
feat: add git_parent_dir computed field to session API responses
```

---

### Task 2: Path compression utility in Go (CLI)

**Files:**
- Create: `cmd/argus/cli/pathutil.go`
- Create: `cmd/argus/cli/pathutil_test.go`

**Step 1: Write the failing tests**

Create `cmd/argus/cli/pathutil_test.go`:

```go
package cli

import "testing"

func TestCompressPath(t *testing.T) {
	home := "/Users/jeevb"
	tests := []struct {
		name      string
		path      string
		home      string
		threshold int
		want      string
	}{
		{
			name:      "short path unchanged",
			path:      "/tmp/project",
			home:      home,
			threshold: 40,
			want:      "/tmp/project",
		},
		{
			name:      "tilde replaces home prefix",
			path:      "/Users/jeevb/project",
			home:      home,
			threshold: 40,
			want:      "~/project",
		},
		{
			name:      "home dir itself",
			path:      "/Users/jeevb",
			home:      home,
			threshold: 40,
			want:      "~",
		},
		{
			name:      "long path compressed",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			home:      home,
			threshold: 30,
			want:      "~/Workspace/.../bxnlabs/argus",
		},
		{
			name:      "non-home long path compressed",
			path:      "/opt/data/very/deep/nested/project",
			home:      home,
			threshold: 20,
			want:      "/opt/.../nested/project",
		},
		{
			name:      "three segments no compression needed",
			path:      "/Users/jeevb/project",
			home:      home,
			threshold: 10,
			want:      "~/project",
		},
		{
			name:      "exactly at threshold no compression",
			path:      "/Users/jeevb/short",
			home:      home,
			threshold: 40,
			want:      "~/short",
		},
		{
			name:      "empty home falls back",
			path:      "/Users/jeevb/Workspace/repos/bxnlabs/argus",
			home:      "",
			threshold: 30,
			want:      "/Users/.../bxnlabs/argus",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressPath(tt.path, tt.home, tt.threshold)
			if got != tt.want {
				t.Errorf("compressPath(%q, %q, %d) = %q, want %q",
					tt.path, tt.home, tt.threshold, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/argus/cli/... -run TestCompressPath -v -count=1`
Expected: FAIL — `compressPath` undefined

**Step 3: Implement `compressPath`**

Create `cmd/argus/cli/pathutil.go`:

```go
package cli

import (
	"path/filepath"
	"strings"
)

// compressPath shortens a path for display:
// 1. Replace home prefix with ~
// 2. If longer than threshold, keep first + last 2 segments joined by /.../
func compressPath(path, home string, threshold int) string {
	// Step 1: tilde-shorten
	display := path
	if home != "" {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			display = "~" + path[len(home):]
		}
	}

	// Step 2: compress if over threshold
	if len(display) <= threshold {
		return display
	}

	// Split into prefix (~ or empty) and segments
	var prefix string
	rest := display
	if strings.HasPrefix(display, "~/") {
		prefix = "~"
		rest = display[1:] // "/Workspace/repos/..."
	}

	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) <= 3 {
		// Can't compress further — already first + 2 tail segments
		return display
	}

	// Keep first segment + last 2 segments
	first := segments[0]
	tail := segments[len(segments)-2:]
	return prefix + "/" + first + "/.../" + filepath.Join(tail[0], tail[1])
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/argus/cli/... -run TestCompressPath -v -count=1`
Expected: PASS

**Step 5: Commit**

```
feat: add path compression utility for CLI directory display
```

---

### Task 3: Add DIRECTORY column to CLI session list

**Files:**
- Modify: `cmd/argus/cli/resolve.go:10-21` (add `GitParentDir` to `sessionInfo`)
- Modify: `cmd/argus/cli/session_list.go:61-75` (add DIRECTORY column)

**Step 1: Add `GitParentDir` field to `sessionInfo`**

In `cmd/argus/cli/resolve.go`, add the new field:

```go
type sessionInfo struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	TmuxName         string  `json:"tmux_name"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	WorkingDirectory string  `json:"working_directory"`
	AgentType        string  `json:"agent_type"`
	AutoApprove      bool    `json:"auto_approve"`
	Model            *string `json:"model"`
	WorktreeBranch   *string `json:"worktree_branch"`
	GitParentDir     *string `json:"git_parent_dir"`
}
```

**Step 2: Update the table header and row in `session_list.go`**

Add DIRECTORY column between PROVIDER and BRANCH. Use `compressPath` and `os.UserHomeDir()` for compression:

```go
func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List all sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			// ...existing client + fetch code...

			home, _ := os.UserHomeDir()

			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tSTATUS\tPROVIDER\tDIRECTORY\tBRANCH\tUPDATED")
			for _, s := range resp.Sessions {
				st := statuses[s.ID]
				if st == "" {
					st = "-"
				}
				branch := ""
				if s.WorktreeBranch != nil {
					branch = *s.WorktreeBranch
				}
				dir := s.WorkingDirectory
				if s.GitParentDir != nil {
					dir = *s.GitParentDir
				}
				if dir == "" {
					dir = "-"
				} else {
					dir = compressPath(dir, home, 35)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					s.ID, s.Name, st, s.AgentType, dir, branch, relativeTime(s.UpdatedAt))
			}
			w.Flush()
			return nil
		},
	}
}
```

**Step 3: Run all CLI tests**

Run: `go test ./cmd/argus/cli/... -v -count=1`
Expected: PASS

**Step 4: Commit**

```
feat: add DIRECTORY column to CLI session list
```

---

### Task 4: Update TypeScript types and data layer for `git_parent_dir` and `home_dir`

**Files:**
- Modify: `web/src/types.ts:1-14` (add `git_parent_dir` to `Session`)
- Modify: `web/src/data/sessions/queries.ts:6-8` (add `home_dir` to `SessionsResponse`)
- Modify: `web/src/hooks/useSessions.ts:9-11` (expose `homeDir` from hook)

**Step 1: Add `git_parent_dir` to Session type**

In `web/src/types.ts`:

```typescript
export interface Session {
  id: string;
  name: string;
  tmux_name: string;
  created_at: string;
  updated_at: string;
  working_directory: string;
  worktree_branch: string | null;
  git_parent_dir: string | null;  // computed by API for worktree sessions
  provider_session_id: string | null;
  model: string | null;
  system_prompt: string | null;
  agent_type: AgentType;
  auto_approve: boolean;
}
```

**Step 2: Update `SessionsResponse` in queries**

In `web/src/data/sessions/queries.ts`, add `home_dir`:

```typescript
interface SessionsResponse {
  sessions: Session[];
  home_dir: string;
}
```

**Step 3: Expose `homeDir` from `useSessions` hook**

In `web/src/hooks/useSessions.ts`, extract and return `homeDir`:

```typescript
export function useSessions() {
  const { data, isSuccess } = useSessionsQuery();
  const sessions: Session[] = data?.sessions ?? [];
  const homeDir: string = data?.home_dir ?? "";
  // ...existing mutations...
  return { sessions, homeDir, isLoaded: isSuccess, deleteSession, renameSession };
}
```

**Step 4: Commit**

```
feat: add git_parent_dir and home_dir to TypeScript session types
```

---

### Task 5: Path compression utility in TypeScript

**Files:**
- Modify: `web/src/lib/utils.ts` (add `compressPath` function)

**Step 1: Add `compressPath` function**

Append to `web/src/lib/utils.ts`:

```typescript
/**
 * Compress a path for display in constrained UI space.
 * 1. Replace home prefix with ~
 * 2. If longer than threshold, keep first + last 2 segments with /.../
 */
export function compressPath(
  path: string,
  homePath: string,
  threshold: number = 30,
): string {
  // Step 1: tilde-shorten
  let display = contractTilde(path, homePath);

  // Step 2: compress if over threshold
  if (display.length <= threshold) return display;

  let prefix = "";
  let rest = display;
  if (display.startsWith("~/")) {
    prefix = "~";
    rest = display.slice(1); // "/Workspace/repos/..."
  }

  const segments = rest.split("/").filter(Boolean);
  if (segments.length <= 3) return display;

  const first = segments[0];
  const tail = segments.slice(-2);
  return `${prefix}/${first}/.../${tail.join("/")}`;
}
```

**Step 2: Commit**

```
feat: add path compression utility for web UI
```

---

### Task 6: Thread `homeDir` through to SessionList and add directory/branch lines

**Files:**
- Modify: `web/src/App.tsx` (extract `homeDir` from `useSessions`, pass down)
- Modify: `web/src/components/views/DesktopView.tsx` (accept + forward `homeDir` prop)
- Modify: `web/src/components/views/MobileView.tsx` (accept + forward `homeDir` prop)
- Modify: `web/src/components/SessionList/index.tsx` (accept `homeDir` prop, add lines 3-4)

The data flow is: `useSessions()` in `App.tsx` → returns `homeDir` → passed as prop to `DesktopView`/`MobileView` → forwarded to `<SessionList homeDir={homeDir}>`.

**Step 1: Import `compressPath` and add `homeDir` prop to SessionList**

In `web/src/components/SessionList/index.tsx`, update the import and the props interface:

```typescript
import { cn, formatRelativeTime, compressPath } from "@/lib/utils";

interface SessionListProps {
  sessions: Session[];
  homeDir: string;
  // ...existing props...
}
```

**Step 2: Add directory and branch display lines**

Inside the session item, after the status line, add lines 3-4:

```tsx
<>
  <span className="block truncate text-sm">
    {session.name || "Unnamed Session"}
  </span>
  <div className="mt-0.5 flex items-center gap-1.5">
    {/* ...existing status dot and label... */}
  </div>
  {/* Line 3: Directory */}
  <span className="text-muted-foreground mt-0.5 block truncate text-xs">
    {compressPath(
      session.git_parent_dir ?? session.working_directory,
      homeDir,
    )}
  </span>
  {/* Line 4: Branch (worktree sessions only) */}
  {session.worktree_branch && (
    <span className="text-muted-foreground mt-0.5 block truncate text-xs">
      ↳ {session.worktree_branch}
    </span>
  )}
</>
```

**Step 3: Thread `homeDir` through the view components**

In `App.tsx`, extract `homeDir` from `useSessions()` and pass it down to the view components. In `DesktopView.tsx` and `MobileView.tsx`, accept and forward the `homeDir` prop to `<SessionList>`.

**Step 4: Build to verify**

Run: `cd web && npm run build`
Expected: Clean build, no type errors.

**Step 5: Commit**

```
feat: display directory and branch in session list UI
```

---

### Task 7: Verify end-to-end and final commit

**Step 1: Run all Go tests**

Run: `go test ./... -count=1`
Expected: All pass.

**Step 2: Run web build**

Run: `cd web && npm run build`
Expected: Clean build.

**Step 3: Manual smoke test (if running locally)**

- Start the agent
- Create a worktree-backed session
- Create a shell session (non-git)
- Verify `argus ls` shows DIRECTORY column with compressed paths
- Verify web UI shows directory on line 3 and branch on line 4

**Step 4: Final commit (if any cleanup needed)**

```
chore: clean up session directory display implementation
```
