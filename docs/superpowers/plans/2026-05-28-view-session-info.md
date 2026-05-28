# View Session Info (BXN-97) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a CLI `argus session describe <id-or-name>` command, a read-only web session-info dialog, and surface the attached profile in both the CLI `session ls` table and the web sidebar.

**Architecture:** Independent rendering per surface — the CLI formats curated fields as Go text, the web formats them in a React dialog. Both read from data that already exists (`GET /api/sessions/{id}` and `/api/sessions` for the CLI; the TanStack Query cache + status map for the web). No new API endpoints, no data-model changes. Pure formatting helpers are unit-tested; command wiring and UI are verified by build + manual run.

**Tech Stack:** Go (cobra CLI, `text/tabwriter`), React + TypeScript (Radix dialog/dropdown via shadcn, TanStack Query), Vitest for web unit tests, Go's `testing` for CLI tests.

**Spec:** `docs/superpowers/specs/2026-05-28-view-session-info-design.md`

---

## Curated field set (single source for both surfaces)

Shown (only when present), grouped:
- **Header**: name, ID, status (active/idle/dead), pinned, profile.
- **Provider**: provider type, model, auto-approve.
- **Location**: directory (home-compressed; prefer `git_parent_dir`, else `working_directory`), repo (parsed from `git_remote_url`), worktree branch.
- **Timestamps**: created, updated (absolute + relative).

Hidden as internal plumbing (CLI `--json` still exposes them): `tmux_name`, `provider_session_id`, `system_prompt`, `branch_created`, `last_viewed_at`, `unread_since`, `user_marked_unread_at`.

## File structure

**CLI (Go), package `cli` under `cmd/argus/cli/`:**
- `resolve.go` — modify: add `GitRemoteURL *string` to `sessionInfo`.
- `repo.go` (new) — `parseRepo(remoteURL string) string` helper + `repo_test.go`.
- `session_describe.go` (new) — `newDescribeCmd()`, `formatSessionDescribe(...)`, small value helpers + `session_describe_test.go`.
- `cli.go` — modify: register `newDescribeCmd()`.
- `session_list.go` — modify: add `PROFILE` column.

**Web (TS) under `web/src/`:**
- `components/SessionInfoDialog/fields.ts` (new) — `buildSessionInfoSections(...)` pure helper + `fields.test.ts`.
- `components/SessionInfoDialog/index.tsx` (new) — `SessionInfoDialog` component.
- `components/SessionList/index.tsx` — modify: add `onViewInfo` prop + "Session info" dropdown item + profile metadata line.
- `components/views/types.ts` — modify: add `onViewInfo` to `ViewProps`.
- `components/views/DesktopView.tsx`, `components/views/MobileView.tsx` — modify: thread `onViewInfo` to `SessionList`.
- `App.tsx` — modify: `infoSession` state, `onViewInfo` in `viewProps`, render `<SessionInfoDialog>`.

---

## Task 1: CLI `parseRepo` helper

**Files:**
- Create: `cmd/argus/cli/repo.go`
- Test: `cmd/argus/cli/repo_test.go`

- [ ] **Step 1: Write the failing test**

Create `cmd/argus/cli/repo_test.go`:

```go
package cli

import "testing"

func TestParseRepo(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"scp-ssh", "git@github.com:bxnlabs/argus.git", "bxnlabs/argus"},
		{"scp-ssh no suffix", "git@github.com:bxnlabs/argus", "bxnlabs/argus"},
		{"https", "https://github.com/bxnlabs/argus.git", "bxnlabs/argus"},
		{"https no suffix", "https://github.com/bxnlabs/argus", "bxnlabs/argus"},
		{"subgroup", "https://gitlab.com/group/sub/proj.git", "group/sub/proj"},
		{"empty", "", ""},
		{"garbage", "not a url", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRepo(tt.url); got != tt.want {
				t.Errorf("parseRepo(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/argus/cli/ -run TestParseRepo`
Expected: FAIL — `undefined: parseRepo`.

- [ ] **Step 3: Write minimal implementation**

Create `cmd/argus/cli/repo.go`:

```go
package cli

import (
	neturl "net/url"
	"strings"
)

// parseRepo extracts "org/repo" (or a deeper subgroup path) from a git remote
// URL. Supports scp-style SSH (git@host:path) and https://host/path. Returns ""
// when the URL doesn't match an expected shape.
func parseRepo(url string) string {
	if url == "" {
		return ""
	}

	var path string

	// scp-style SSH: user@host:path
	if at := strings.Index(url, "@"); at >= 0 {
		if colon := strings.Index(url[at:], ":"); colon >= 0 {
			path = url[at+colon+1:]
		}
	}

	// https://host/path (or ssh://host/path)
	if path == "" {
		if u, err := neturl.Parse(url); err == nil && len(u.Path) > 1 {
			path = u.Path
		}
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if !strings.Contains(path, "/") {
		return ""
	}
	return path
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/argus/cli/ -run TestParseRepo`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/argus/cli/repo.go cmd/argus/cli/repo_test.go
git commit -m "feat(cli): parseRepo helper for git remote URLs (BXN-97)"
```

---

## Task 2: CLI describe formatter + `sessionInfo.GitRemoteURL`

**Files:**
- Modify: `cmd/argus/cli/resolve.go` (add one field to `sessionInfo`)
- Create: `cmd/argus/cli/session_describe.go` (formatter + helpers only this task)
- Test: `cmd/argus/cli/session_describe_test.go`

- [ ] **Step 1: Add the `GitRemoteURL` field to `sessionInfo`**

In `cmd/argus/cli/resolve.go`, the `sessionInfo` struct ends with the `Profile` field. Add `GitRemoteURL` after it:

```go
	GitParentDir     *string `json:"git_parent_dir"`
	Profile          *string `json:"profile"`
	GitRemoteURL     *string `json:"git_remote_url"`
}
```

- [ ] **Step 2: Write the failing test**

Create `cmd/argus/cli/session_describe_test.go`:

```go
package cli

import (
	"strings"
	"testing"
)

func strptr(s string) *string { return &s }

func TestFormatSessionDescribe_Full(t *testing.T) {
	s := sessionInfo{
		ID:               "sess_abc123",
		Name:             "my-session",
		CreatedAt:        "2026-05-20 14:32:05",
		UpdatedAt:        "2026-05-28 09:15:00",
		WorkingDirectory: "/home/u/work",
		ProviderType:     "claude",
		AutoApprove:      true,
		Pinned:           true,
		Model:            strptr("claude-opus-4-7"),
		WorktreeBranch:   strptr("jeev/bxn-97"),
		Profile:          strptr("default"),
		GitRemoteURL:     strptr("git@github.com:bxnlabs/argus.git"),
	}
	out := formatSessionDescribe(s, "idle", "/home/u")

	for _, want := range []string{
		"Session: my-session",
		"sess_abc123",
		"Status:", "idle",
		"Pinned:", "yes",
		"Profile:", "default",
		"Type:", "claude",
		"Model:", "claude-opus-4-7",
		"Auto-approve:", "on",
		"Repo:", "bxnlabs/argus",
		"Branch:", "jeev/bxn-97",
		"Created:", "2026-05-20 14:32:05",
		"Updated:", "2026-05-28 09:15:00",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

func TestFormatSessionDescribe_Minimal(t *testing.T) {
	s := sessionInfo{
		ID:               "sess_x",
		Name:             "bare",
		CreatedAt:        "2026-05-20 14:32:05",
		UpdatedAt:        "2026-05-20 14:32:05",
		WorkingDirectory: "/home/u/work",
		ProviderType:     "shell",
	}
	out := formatSessionDescribe(s, "", "/home/u")

	if !strings.Contains(out, "Profile:") || !strings.Contains(out, "none") {
		t.Errorf("expected profile 'none', got:\n%s", out)
	}
	if !strings.Contains(out, "Auto-approve:") || !strings.Contains(out, "off") {
		t.Errorf("expected auto-approve 'off', got:\n%s", out)
	}
	if strings.Contains(out, "Branch:") {
		t.Errorf("did not expect a Branch line for a session with no worktree:\n%s", out)
	}
	if strings.Contains(out, "Repo:") {
		t.Errorf("did not expect a Repo line for a session with no remote:\n%s", out)
	}
	if !strings.Contains(out, "Status:") || !strings.Contains(out, "-") {
		t.Errorf("expected status '-' when unknown, got:\n%s", out)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/argus/cli/ -run TestFormatSessionDescribe`
Expected: FAIL — `undefined: formatSessionDescribe`.

- [ ] **Step 4: Write the formatter (and value helpers) in a new file**

Create `cmd/argus/cli/session_describe.go` with **only** the formatter and helpers for now (the cobra command is added in Task 3):

```go
package cli

import (
	"fmt"
	"strings"
)

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func strOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// formatSessionDescribe renders the curated, sectioned human-readable summary
// of a session. status is the runtime status ("active"/"idle"/"dead"), or ""
// when unavailable; home is the user's home dir for path compression.
func formatSessionDescribe(s sessionInfo, status, home string) string {
	if status == "" {
		status = "-"
	}

	dir := s.WorkingDirectory
	if s.GitParentDir != nil && *s.GitParentDir != "" {
		dir = *s.GitParentDir
	}
	if dir == "" {
		dir = "-"
	} else {
		dir = compressPath(dir, home, 60)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Session: %s\n", s.Name)
	fmt.Fprintf(&b, "  ID:        %s\n", s.ID)
	fmt.Fprintf(&b, "  Status:    %s\n", status)
	fmt.Fprintf(&b, "  Pinned:    %s\n", yesNo(s.Pinned))
	fmt.Fprintf(&b, "  Profile:   %s\n", strOr(s.Profile, "none"))

	b.WriteString("\nProvider\n")
	fmt.Fprintf(&b, "  Type:         %s\n", s.ProviderType)
	if s.Model != nil && *s.Model != "" {
		fmt.Fprintf(&b, "  Model:        %s\n", *s.Model)
	}
	fmt.Fprintf(&b, "  Auto-approve: %s\n", onOff(s.AutoApprove))

	b.WriteString("\nLocation\n")
	fmt.Fprintf(&b, "  Directory: %s\n", dir)
	if s.GitRemoteURL != nil {
		if repo := parseRepo(*s.GitRemoteURL); repo != "" {
			fmt.Fprintf(&b, "  Repo:      %s\n", repo)
		}
	}
	if s.WorktreeBranch != nil && *s.WorktreeBranch != "" {
		fmt.Fprintf(&b, "  Branch:    %s\n", *s.WorktreeBranch)
	}

	b.WriteString("\nTimestamps\n")
	fmt.Fprintf(&b, "  Created:   %s (%s)\n", s.CreatedAt, relativeTime(s.CreatedAt))
	fmt.Fprintf(&b, "  Updated:   %s (%s)\n", s.UpdatedAt, relativeTime(s.UpdatedAt))

	return b.String()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./cmd/argus/cli/ -run TestFormatSessionDescribe`
Expected: PASS.

- [ ] **Step 6: Run the full CLI package test (ensure nothing else broke)**

Run: `go test ./cmd/argus/cli/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add cmd/argus/cli/resolve.go cmd/argus/cli/session_describe.go cmd/argus/cli/session_describe_test.go
git commit -m "feat(cli): session describe formatter + git_remote_url field (BXN-97)"
```

---

## Task 3: CLI `session describe` command wiring

**Files:**
- Modify: `cmd/argus/cli/session_describe.go` (add `newDescribeCmd`)
- Modify: `cmd/argus/cli/cli.go` (register the command)

- [ ] **Step 1: Add the command to `session_describe.go`**

Append `newDescribeCmd` to `cmd/argus/cli/session_describe.go`, and add the imports it needs. The full import block for the file becomes:

```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)
```

Append this function:

```go
// newDescribeCmd returns the "session describe" command.
func newDescribeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "describe <sess>",
		Short: "Show full details for a session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			query := args[0]

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			// Fetch the list once: resolve the query and learn the home dir
			// (used for path compression). Mirrors `session ls`.
			body, err := c.get("/api/sessions")
			if err != nil {
				return err
			}
			var listResp struct {
				Sessions []sessionInfo `json:"sessions"`
				HomeDir  string        `json:"home_dir"`
			}
			if err := json.Unmarshal(body, &listResp); err != nil {
				return fmt.Errorf("parse response: %w", err)
			}
			s, err := resolveSession(listResp.Sessions, query)
			if err != nil {
				return err
			}

			if asJSON {
				// Print the canonical full record from GET /api/sessions/{id}.
				detail, err := c.get("/api/sessions/" + s.ID)
				if err != nil {
					return err
				}
				var wrap struct {
					Session json.RawMessage `json:"session"`
				}
				if err := json.Unmarshal(detail, &wrap); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				var pretty bytes.Buffer
				if err := json.Indent(&pretty, wrap.Session, "", "  "); err != nil {
					return fmt.Errorf("format json: %w", err)
				}
				fmt.Println(pretty.String())
				return nil
			}

			// Best-effort runtime status (don't fail if unavailable).
			status := ""
			if statusBody, err := c.get("/api/sessions/status"); err == nil {
				var statusResp struct {
					Statuses map[string]struct {
						Status string `json:"status"`
					} `json:"statuses"`
				}
				if err := json.Unmarshal(statusBody, &statusResp); err == nil {
					status = statusResp.Statuses[s.ID].Status
				}
			}

			fmt.Print(formatSessionDescribe(*s, status, listResp.HomeDir))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Output the raw session record as JSON")
	return cmd
}
```

Note: `strings` is imported because `formatSessionDescribe` (same file) uses it; `bytes`/`encoding/json` are used by `newDescribeCmd`.

- [ ] **Step 2: Register the command in `cli.go`**

In `cmd/argus/cli/cli.go`, add `newDescribeCmd()` to the `cmd.AddCommand(...)` list (place it right after `newListCmd()`):

```go
	cmd.AddCommand(
		newListCmd(),
		newDescribeCmd(),
		newCreateCmd(),
		newAttachCmd(),
		newDeleteCmd(),
		newRenameCmd(),
		newProfileCmd(),
		newPwdCmd(),
	)
```

- [ ] **Step 3: Build and verify the package compiles + tests pass**

Run: `go build ./... && go test ./cmd/argus/cli/`
Expected: build succeeds, tests PASS.

- [ ] **Step 4: Manual smoke test against the dev node**

In one terminal start the dev node (if not already running): `make dev-api`
In another terminal:

```bash
ARGUS_HOME=$PWD/.dev go run ./cmd/argus session ls          # note a name or ID
ARGUS_HOME=$PWD/.dev go run ./cmd/argus session describe <name-or-id>
ARGUS_HOME=$PWD/.dev go run ./cmd/argus session describe <name-or-id> --json | jq .
```

Expected: the first prints the curated sections; `--json` prints a pretty JSON object that `jq` parses without error. Also confirm an unknown name errors clearly (`session describe nope`).

- [ ] **Step 5: Commit**

```bash
git add cmd/argus/cli/session_describe.go cmd/argus/cli/cli.go
git commit -m "feat(cli): add 'session describe' command with --json (BXN-97)"
```

---

## Task 4: Profile column in `session ls`

**Files:**
- Modify: `cmd/argus/cli/session_list.go`

- [ ] **Step 1: Add `PROFILE` to the header row**

In `cmd/argus/cli/session_list.go`, change the header line (currently around line 81) to insert `PROFILE` after `PROVIDER`:

```go
		fmt.Fprintln(w, "  ID\tPINNED\tNAME\tSTATUS\tPROVIDER\tPROFILE\tDIRECTORY\tBRANCH\tUPDATED")
```

- [ ] **Step 2: Compute the profile cell and add it to the row**

Inside the `for _, s := range pinnedFirst(resp.Sessions)` loop, just before the `fmt.Fprintf(w, ...)` call, add:

```go
		profile := "-"
		if s.Profile != nil && *s.Profile != "" {
			profile = *s.Profile
		}
```

Then change the row `Fprintf` (currently around line 120) to add one `%s` after `PROVIDER` and pass `profile` after `s.ProviderType`:

```go
		fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			marker, s.ID, pinnedMark, s.Name, st, s.ProviderType, profile, dir, branch, updated)
```

- [ ] **Step 3: Build and run package tests**

Run: `go build ./... && go test ./cmd/argus/cli/`
Expected: build succeeds, tests PASS (existing tests cover `relativeTime`/`pinnedFirst` only, so they are unaffected).

- [ ] **Step 4: Manual smoke test**

Run (against the dev node from Task 3):

```bash
ARGUS_HOME=$PWD/.dev go run ./cmd/argus session ls
```

Expected: a `PROFILE` column appears after `PROVIDER`, showing the profile name for sessions that have one and `-` otherwise; columns stay aligned.

- [ ] **Step 5: Commit**

```bash
git add cmd/argus/cli/session_list.go
git commit -m "feat(cli): show profile column in 'session ls' (BXN-97)"
```

---

## Task 5: Web `buildSessionInfoSections` helper

**Files:**
- Create: `web/src/components/SessionInfoDialog/fields.ts`
- Test: `web/src/components/SessionInfoDialog/fields.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/components/SessionInfoDialog/fields.test.ts`:

```ts
import { describe, it, expect } from "vitest";
import { buildSessionInfoSections } from "./fields";
import type { Session } from "@/types";

function makeSession(overrides: Partial<Session> = {}): Session {
  return {
    id: "id",
    name: "name",
    tmux_name: "claude-id",
    created_at: "2026-05-20 14:32:05",
    updated_at: "2026-05-28 09:15:00",
    working_directory: "/home/u/work",
    worktree_branch: null,
    git_parent_dir: null,
    git_remote_url: null,
    provider_session_id: null,
    model: null,
    system_prompt: null,
    provider_type: "claude",
    auto_approve: false,
    profile: null,
    pinned: false,
    ...overrides,
  };
}

function findRow(sections: ReturnType<typeof buildSessionInfoSections>, label: string) {
  for (const s of sections) {
    const row = s.rows.find((r) => r.label === label);
    if (row) return row;
  }
  return undefined;
}

describe("buildSessionInfoSections", () => {
  it("renders header, provider, location, and timestamps sections", () => {
    const sections = buildSessionInfoSections(makeSession(), "idle", "/home/u");
    expect(sections.map((s) => s.title)).toEqual([null, "Provider", "Location", "Timestamps"]);
  });

  it("maps status, pinned, profile, and auto-approve to friendly values", () => {
    const sections = buildSessionInfoSections(
      makeSession({ pinned: true, profile: "default", auto_approve: true }),
      "active",
      "/home/u",
    );
    expect(findRow(sections, "Status")?.value).toBe("Active");
    expect(findRow(sections, "Pinned")?.value).toBe("Yes");
    expect(findRow(sections, "Profile")?.value).toBe("default");
    expect(findRow(sections, "Auto-approve")?.value).toBe("On");
  });

  it("falls back when optional fields are absent", () => {
    const sections = buildSessionInfoSections(makeSession(), undefined, "/home/u");
    expect(findRow(sections, "Status")?.value).toBe("Unknown");
    expect(findRow(sections, "Profile")?.value).toBe("None");
    expect(findRow(sections, "Model")).toBeUndefined();
    expect(findRow(sections, "Branch")).toBeUndefined();
    expect(findRow(sections, "Repo")).toBeUndefined();
  });

  it("includes model, repo, and branch when present", () => {
    const sections = buildSessionInfoSections(
      makeSession({
        model: "claude-opus-4-7",
        git_remote_url: "git@github.com:bxnlabs/argus.git",
        worktree_branch: "jeev/bxn-97",
      }),
      "idle",
      "/home/u",
    );
    expect(findRow(sections, "Model")?.value).toBe("claude-opus-4-7");
    expect(findRow(sections, "Repo")?.value).toBe("bxnlabs/argus");
    expect(findRow(sections, "Branch")?.value).toBe("jeev/bxn-97");
  });

  it("shows absolute timestamps", () => {
    const sections = buildSessionInfoSections(makeSession(), "idle", "/home/u");
    expect(findRow(sections, "Created")?.value).toContain("2026-05-20 14:32:05");
    expect(findRow(sections, "Updated")?.value).toContain("2026-05-28 09:15:00");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && pnpm exec vitest run src/components/SessionInfoDialog/fields.test.ts`
Expected: FAIL — cannot resolve `./fields`.

- [ ] **Step 3: Write the helper**

Create `web/src/components/SessionInfoDialog/fields.ts`:

```ts
import type { Session } from "@/types";
import { compressPath, formatRelativeTime, parseRepoFromRemoteURL } from "@/lib/utils";

export interface InfoRow {
  label: string;
  value: string;
}

export interface InfoSection {
  title: string | null;
  rows: InfoRow[];
}

function statusLabel(status: string | undefined): string {
  switch (status) {
    case "active":
      return "Active";
    case "idle":
      return "Idle";
    case "dead":
      return "Dead";
    default:
      return "Unknown";
  }
}

// buildSessionInfoSections produces the curated, grouped fields shown in the
// session info dialog. Optional fields (model, repo, branch) are omitted when
// absent. status is the runtime status string, or undefined when unavailable.
export function buildSessionInfoSections(
  session: Session,
  status: string | undefined,
  homeDir: string,
): InfoSection[] {
  const repo = session.git_remote_url
    ? parseRepoFromRemoteURL(session.git_remote_url)
    : null;
  const dir = compressPath(
    session.git_parent_dir ?? session.working_directory,
    homeDir,
    60,
  );

  const header: InfoRow[] = [
    { label: "ID", value: session.id },
    { label: "Status", value: statusLabel(status) },
    { label: "Pinned", value: session.pinned ? "Yes" : "No" },
    { label: "Profile", value: session.profile ?? "None" },
  ];

  const provider: InfoRow[] = [{ label: "Type", value: session.provider_type }];
  if (session.model) provider.push({ label: "Model", value: session.model });
  provider.push({
    label: "Auto-approve",
    value: session.auto_approve ? "On" : "Off",
  });

  const location: InfoRow[] = [{ label: "Directory", value: dir }];
  if (repo) location.push({ label: "Repo", value: repo });
  if (session.worktree_branch)
    location.push({ label: "Branch", value: session.worktree_branch });

  const timestamps: InfoRow[] = [
    {
      label: "Created",
      value: `${session.created_at} (${formatRelativeTime(session.created_at)})`,
    },
    {
      label: "Updated",
      value: `${session.updated_at} (${formatRelativeTime(session.updated_at)})`,
    },
  ];

  return [
    { title: null, rows: header },
    { title: "Provider", rows: provider },
    { title: "Location", rows: location },
    { title: "Timestamps", rows: timestamps },
  ];
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && pnpm exec vitest run src/components/SessionInfoDialog/fields.test.ts`
Expected: PASS (all 5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/SessionInfoDialog/fields.ts web/src/components/SessionInfoDialog/fields.test.ts
git commit -m "feat(web): session info field builder + tests (BXN-97)"
```

---

## Task 6: Web `SessionInfoDialog` component

**Files:**
- Create: `web/src/components/SessionInfoDialog/index.tsx`

- [ ] **Step 1: Write the component**

Create `web/src/components/SessionInfoDialog/index.tsx`:

```tsx
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import type { Session } from "@/types";
import { buildSessionInfoSections } from "./fields";

interface SessionInfoDialogProps {
  session: Session | null;
  status?: string;
  homeDir: string;
  onClose: () => void;
}

export function SessionInfoDialog({
  session,
  status,
  homeDir,
  onClose,
}: SessionInfoDialogProps) {
  const sections = session
    ? buildSessionInfoSections(session, status, homeDir)
    : [];

  return (
    <Dialog open={session !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="truncate">
            {session?.name || "Session"}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          {sections.map((section, i) => (
            <div key={section.title ?? `section-${i}`} className="space-y-1">
              {section.title && (
                <div className="text-muted-foreground text-xs font-bold uppercase tracking-wide">
                  {section.title}
                </div>
              )}
              <dl className="space-y-1">
                {section.rows.map((row) => (
                  <div key={row.label} className="flex gap-2 text-sm">
                    <dt className="text-muted-foreground w-28 flex-shrink-0">
                      {row.label}
                    </dt>
                    <dd className="min-w-0 break-all">{row.value}</dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 2: Typecheck (the component isn't imported yet, so this just confirms it compiles)**

Run: `cd web && pnpm exec tsc -b`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add web/src/components/SessionInfoDialog/index.tsx
git commit -m "feat(web): SessionInfoDialog component (BXN-97)"
```

---

## Task 7: Wire "Session info" into the menu and app

This task threads a new `onViewInfo` callback end-to-end and renders the dialog, so the whole tree compiles in one commit.

**Files:**
- Modify: `web/src/components/SessionList/index.tsx`
- Modify: `web/src/components/views/types.ts`
- Modify: `web/src/components/views/DesktopView.tsx`
- Modify: `web/src/components/views/MobileView.tsx`
- Modify: `web/src/App.tsx`

- [ ] **Step 1: Add `Info` to the SessionList icon import**

In `web/src/components/SessionList/index.tsx`, the lucide import (line 12) currently ends with `Mail`. Add `Info`:

```tsx
import { Plus, AlertCircle, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch, BrushCleaning, SlidersHorizontal, Pin, MailOpen, Mail, Info } from "lucide-react";
```

- [ ] **Step 2: Add `onViewInfo` to `SessionItemProps`**

In the `SessionItemProps` interface, add after `onChangeProfile`:

```tsx
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
```

- [ ] **Step 3: Destructure `onViewInfo` in `SessionItem`**

In the `SessionItem` function parameter list, add `onViewInfo,` next to `onChangeProfile,`.

- [ ] **Step 4: Add the "Session info" dropdown item**

In `SessionItem`'s `DropdownMenuContent`, add this as the **first** item (immediately after `<DropdownMenuContent ...>` opens, before the Pin item), followed by a separator:

```tsx
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onViewInfo(session);
            }}
          >
            <Info className="mr-2 h-3 w-3" />
            Session info
          </DropdownMenuItem>
          <DropdownMenuSeparator />
```

- [ ] **Step 5: Add `onViewInfo` to `SessionListProps` and thread it through**

In `SessionListProps`, add after `onChangeProfile`:

```tsx
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
```

In the `SessionList` function parameter list, add `onViewInfo,` next to `onChangeProfile,`.

In `renderItem`, pass it to `SessionItem` next to `onChangeProfile`:

```tsx
        onChangeProfile={onChangeProfile}
        onViewInfo={onViewInfo}
```

- [ ] **Step 6: Add `onViewInfo` to `ViewProps`**

In `web/src/components/views/types.ts`, add after `onChangeProfile`:

```tsx
  onChangeProfile: (session: Session) => void;
  onViewInfo: (session: Session) => void;
```

- [ ] **Step 7: Thread `onViewInfo` in DesktopView**

In `web/src/components/views/DesktopView.tsx`: add `onViewInfo,` to the destructured props (next to `onChangeProfile,`), and pass `onViewInfo={onViewInfo}` to `<SessionList>` (next to `onChangeProfile={onChangeProfile}`).

- [ ] **Step 8: Thread `onViewInfo` in MobileView**

In `web/src/components/views/MobileView.tsx`: add `onViewInfo,` to the destructured props (next to `onChangeProfile,`), and pass `onViewInfo={onViewInfo}` to `<SessionList>` (next to `onChangeProfile={onChangeProfile}`).

- [ ] **Step 9: Add state, viewProps entry, and render the dialog in App.tsx**

In `web/src/App.tsx`:

a) Add the import (next to the `ChangeProfileDialog` import, line 12):

```tsx
import { SessionInfoDialog } from "@/components/SessionInfoDialog";
```

b) Add state next to `changeProfileSession` (line 30):

```tsx
  const [infoSession, setInfoSession] = useState<Session | null>(null);
```

c) Add to `viewProps` (next to `onChangeProfile: setChangeProfileSession,`, line 384):

```tsx
    onChangeProfile: setChangeProfileSession,
    onViewInfo: setInfoSession,
```

d) Render `<SessionInfoDialog>` in **both** return branches, immediately after each `<ChangeProfileDialog .../>`:

```tsx
        <SessionInfoDialog
          session={infoSession}
          status={infoSession ? sessionStatuses[infoSession.id]?.status : undefined}
          homeDir={homeDir}
          onClose={() => setInfoSession(null)}
        />
```

(Add it once inside the mobile `<>...</>` block after the mobile `ChangeProfileDialog`, and once inside the desktop `<>...</>` block after the desktop `ChangeProfileDialog`.)

- [ ] **Step 10: Typecheck**

Run: `cd web && pnpm exec tsc -b`
Expected: no errors.

- [ ] **Step 11: Manual browser verification**

Start the stack (`make dev`) and open http://localhost:5273. With at least one session:
- Open a session row's `...` menu → click **Session info** → the dialog opens showing the curated fields (header + Provider + Location + Timestamps).
- Confirm optional rows behave: a session with no worktree shows no Branch/Repo; a session with a profile shows the profile, otherwise "None".
- Confirm the dialog is read-only (only a Close button) and closes via Close, Escape, and clicking outside.

- [ ] **Step 12: Commit**

```bash
git add web/src/components/SessionList/index.tsx web/src/components/views/types.ts web/src/components/views/DesktopView.tsx web/src/components/views/MobileView.tsx web/src/App.tsx
git commit -m "feat(web): open session info dialog from the session menu (BXN-97)"
```

---

## Task 8: Profile line in the sidebar session row

**Files:**
- Modify: `web/src/components/SessionList/index.tsx`

- [ ] **Step 1: Add the profile metadata line**

In `SessionItem` (`web/src/components/SessionList/index.tsx`), directly after the branch line block (the `{session.worktree_branch && ( ... )}` JSX, which ends around line 232), add:

```tsx
            {/* Line 5: Profile (only when attached) */}
            {session.profile && (
              <span className="text-muted-foreground mt-0.5 flex items-center gap-1 text-xs">
                <SlidersHorizontal className="h-3 w-3 flex-shrink-0" />
                <span className="truncate">{session.profile}</span>
              </span>
            )}
```

(`SlidersHorizontal` is already imported.)

- [ ] **Step 2: Typecheck**

Run: `cd web && pnpm exec tsc -b`
Expected: no errors.

- [ ] **Step 3: Manual browser verification**

In the running app: a session with a profile attached shows a new line with the sliders icon + profile name beneath the directory/branch lines; a session with no profile shows no such line. (Use `argus session profile set <sess> <profile>` from the CLI to attach one if needed, or the "Change profile" menu item.)

- [ ] **Step 4: Commit**

```bash
git add web/src/components/SessionList/index.tsx
git commit -m "feat(web): show attached profile on sidebar session rows (BXN-97)"
```

---

## Task 9: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Run all Go tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 2: Build the whole Go module**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Run all web unit tests**

Run: `cd web && pnpm exec vitest run`
Expected: PASS (including the new `SessionInfoDialog/fields.test.ts` and the existing `SessionList/index.test.tsx`).

- [ ] **Step 4: Web typecheck + production build**

Run: `cd web && pnpm run build`
Expected: `tsc -b` clean, Vite build succeeds.

- [ ] **Step 5: End-to-end manual smoke (CLI + web)**

With the dev node running and at least one session that has a profile attached:
- CLI: `ARGUS_HOME=$PWD/.dev go run ./cmd/argus session ls` shows the `PROFILE` column.
- CLI: `... session describe <name>` shows the curated sections; `... session describe <name> --json | jq .` parses cleanly.
- Web: the `...` menu opens the **Session info** dialog; the sidebar row shows the profile line when attached.

- [ ] **Step 6: Confirm the working tree is clean**

Run: `git status`
Expected: nothing to commit (all task commits landed).
