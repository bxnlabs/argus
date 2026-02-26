# Session Dialog UX Improvements — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Simplify the new session dialog with a unified Source picker modal, GitHub repo search, and required name field.

**Architecture:** Extend `DirectoryPicker` into a `SourcePicker` with Local/Remote tabs. Add a backend `/api/github/repos` endpoint that caches GitHub API results and applies server-side fuzzy search using `sahilm/fuzzy`. Frontend gets a new `RepoBrowser` component for the Remote tab.

**Tech Stack:** Go (google/go-github, sahilm/fuzzy), React 19, TanStack Query, Radix UI, Tailwind CSS, shadcn/ui

---

### Task 1: Add `GitHubToken` to config

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Add test for GitHub token parsing**

Add a test case to `config_test.go`:

```go
func TestLoadGitHubToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	os.WriteFile(path, []byte(`github_token = "ghp_test123"`), 0o644)

	cfg, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitHubToken != "ghp_test123" {
		t.Errorf("got %q, want %q", cfg.GitHubToken, "ghp_test123")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadGitHubToken -v`
Expected: FAIL — `Config` has no `GitHubToken` field

**Step 3: Add field to Config struct**

In `config.go`, add to the `Config` struct:

```go
type Config struct {
	BranchPrefix string `toml:"branch_prefix"`
	GitHubToken  string `toml:"github_token"`
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add github_token to config"
```

---

### Task 2: Add `google/go-github` dependency

**Files:**
- Modify: `go.mod`, `go.sum`

**Step 1: Add dependency**

Run: `go get github.com/google/go-github/v69`

**Step 2: Verify import works**

Run: `go build ./...`
Expected: Clean build

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add google/go-github v69"
```

---

### Task 3: Implement GitHub repo listing service

**Files:**
- Create: `internal/github/repos.go`
- Create: `internal/github/repos_test.go`

**Step 1: Write the test**

Create `internal/github/repos_test.go`:

```go
package github

import (
	"testing"
)

func TestFuzzySearch(t *testing.T) {
	repos := []string{
		"bxnlabs/argus",
		"bxnlabs/infra",
		"bxnlabs/sdk",
		"myorg/backend",
		"myorg/frontend",
	}

	tests := []struct {
		query string
		want  []string
	}{
		{"arg", []string{"bxnlabs/argus"}},
		{"bxn", []string{"bxnlabs/argus", "bxnlabs/infra", "bxnlabs/sdk"}},
		{"front", []string{"myorg/frontend"}},
		{"", nil}, // empty query returns nil (caller should return full list)
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := fuzzyFilterRepos(repos, tt.query)
			if len(got) != len(tt.want) {
				t.Errorf("fuzzyFilterRepos(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/github/ -run TestFuzzySearch -v`
Expected: FAIL — package doesn't exist yet

**Step 3: Implement the package**

Create `internal/github/repos.go`:

```go
package github

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	gh "github.com/google/go-github/v69/github"
	"github.com/sahilm/fuzzy"
)

// RepoService lists and searches GitHub repositories for the authenticated user.
type RepoService struct {
	token string

	mu        sync.Mutex
	cached    []string
	fetchedAt time.Time
}

const cacheTTL = 5 * time.Minute

// NewRepoService creates a new RepoService with the given GitHub token.
func NewRepoService(token string) *RepoService {
	return &RepoService{token: token}
}

// Search returns repos matching the query. If query is empty, returns all repos.
// Results are cached for 5 minutes.
func (s *RepoService) Search(ctx context.Context, query string) ([]string, error) {
	repos, err := s.listAll(ctx)
	if err != nil {
		return nil, err
	}
	if query == "" {
		return repos, nil
	}
	return fuzzyFilterRepos(repos, query), nil
}

func (s *RepoService) listAll(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	if s.cached != nil && time.Since(s.fetchedAt) < cacheTTL {
		cached := s.cached
		s.mu.Unlock()
		return cached, nil
	}
	s.mu.Unlock()

	client := gh.NewClient(nil).WithAuthToken(s.token)

	var allRepos []string

	// Fetch user's repos (includes personal + repos with push access)
	opts := &gh.RepositoryListByAuthenticatedUserOptions{
		Sort:        "full_name",
		ListOptions: gh.ListOptions{PerPage: 100},
	}
	for {
		repos, resp, err := client.Repositories.ListByAuthenticatedUser(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("listing user repos: %w", err)
		}
		for _, r := range repos {
			allRepos = append(allRepos, r.GetFullName())
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Fetch org repos
	orgs, _, err := client.Organizations.List(ctx, "", &gh.ListOptions{PerPage: 100})
	if err != nil {
		return nil, fmt.Errorf("listing orgs: %w", err)
	}
	for _, org := range orgs {
		orgOpts := &gh.RepositoryListByOrgOptions{
			Sort:        "full_name",
			ListOptions: gh.ListOptions{PerPage: 100},
		}
		for {
			repos, resp, err := client.Repositories.ListByOrg(ctx, org.GetLogin(), orgOpts)
			if err != nil {
				return nil, fmt.Errorf("listing repos for org %s: %w", org.GetLogin(), err)
			}
			for _, r := range repos {
				allRepos = append(allRepos, r.GetFullName())
			}
			if resp.NextPage == 0 {
				break
			}
			orgOpts.Page = resp.NextPage
		}
	}

	// Deduplicate (user repos may overlap with org repos)
	seen := make(map[string]bool, len(allRepos))
	deduped := allRepos[:0]
	for _, r := range allRepos {
		if !seen[r] {
			seen[r] = true
			deduped = append(deduped, r)
		}
	}
	sort.Strings(deduped)

	s.mu.Lock()
	s.cached = deduped
	s.fetchedAt = time.Now()
	s.mu.Unlock()

	return deduped, nil
}

// fuzzyFilterRepos applies fuzzy matching on repo full names.
func fuzzyFilterRepos(repos []string, query string) []string {
	if query == "" {
		return nil
	}
	src := repoSource(repos)
	matches := fuzzy.FindFromNoSort(query, src)
	sort.SliceStable(matches, func(i, j int) bool {
		ii, jj := matches[i].Index, matches[j].Index
		// Tier 1: prefer matches where the repo name (after /) matches
		ti := repoNameTier(repos[ii], query)
		tj := repoNameTier(repos[jj], query)
		if ti != tj {
			return ti < tj
		}
		// Tier 2: fuzzy score (descending)
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		// Tier 3: lexical
		return repos[ii] < repos[jj]
	})
	result := make([]string, len(matches))
	for i, m := range matches {
		result[i] = repos[m.Index]
	}
	return result
}

type repoSource []string

func (s repoSource) String(i int) string { return s[i] }
func (s repoSource) Len() int            { return len(s) }

// repoNameTier returns a ranking tier based on how the query matches the repo
// name (the part after the /). Lower is better.
func repoNameTier(fullName, query string) int {
	q := strings.ToLower(query)
	_, name, _ := strings.Cut(fullName, "/")
	name = strings.ToLower(name)
	switch {
	case name == q:
		return 0 // exact
	case strings.HasPrefix(name, q):
		return 1 // prefix
	case strings.Contains(name, q):
		return 2 // substring
	default:
		return 3 // path-only
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/github/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/github/
git commit -m "feat: add github repo listing service with fuzzy search"
```

---

### Task 4: Add GitHub repos HTTP handler and route

**Files:**
- Create: `internal/agent/api/github.go`
- Modify: `internal/agent/api/router.go`

**Step 1: Create the handler**

Create `internal/agent/api/github.go`:

```go
package api

import (
	"net/http"

	ghservice "github.com/bxnlabs/argus/internal/github"
)

type githubHandler struct {
	repoService *ghservice.RepoService
}

// GET /api/github/repos?q=...
func (h *githubHandler) listRepos(w http.ResponseWriter, r *http.Request) {
	if h.repoService == nil {
		respondJSON(w, http.StatusOK, map[string]any{"repos": []string{}})
		return
	}

	query := r.URL.Query().Get("q")
	repos, err := h.repoService.Search(r.Context(), query)
	if err != nil {
		respondInternalError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"repos": repos})
}
```

**Step 2: Wire the route into the router**

In `router.go`:
- Add `RepoService *ghservice.RepoService` to `Deps` struct (import `ghservice "github.com/bxnlabs/argus/internal/github"`)
- Register the route in `NewRouter`:

```go
// GitHub routes
ghub := &githubHandler{repoService: deps.RepoService}
mux.HandleFunc("GET /api/github/repos", ghub.listRepos)
```

**Step 3: Wire RepoService creation in the server startup**

Find where `Deps` is constructed (likely in `cmd/` or `main.go`) and create the `RepoService` from config:

```go
var repoService *github.RepoService
if cfg.GitHubToken != "" {
    repoService = github.NewRepoService(cfg.GitHubToken)
}
```

Pass it into `Deps{RepoService: repoService}`.

**Step 4: Verify build**

Run: `go build ./...`
Expected: Clean build

**Step 5: Commit**

```bash
git add internal/agent/api/github.go internal/agent/api/router.go cmd/
git commit -m "feat: add GET /api/github/repos endpoint"
```

---

### Task 5: Add frontend GitHub repos query hooks

**Files:**
- Create: `web/src/data/github/keys.ts`
- Create: `web/src/data/github/queries.ts`
- Create: `web/src/data/github/index.ts`

**Step 1: Create query key factory**

Create `web/src/data/github/keys.ts`:

```typescript
export const githubKeys = {
  all: ["github"] as const,
  repos: (query: string) => [...githubKeys.all, "repos", query] as const,
};
```

**Step 2: Create query hook**

Create `web/src/data/github/queries.ts`:

```typescript
import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "@/api/client";
import { githubKeys } from "./keys";

interface GitHubReposResponse {
  repos: string[];
}

export function useGitHubReposQuery(
  query: string,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: githubKeys.repos(query),
    queryFn: () => {
      const params = new URLSearchParams();
      if (query) params.set("q", query);
      const qs = params.toString();
      return apiFetch<GitHubReposResponse>(
        `/agent/api/github/repos${qs ? `?${qs}` : ""}`,
      );
    },
    enabled: options?.enabled ?? true,
    staleTime: 60_000,
  });
}
```

**Step 3: Create barrel export**

Create `web/src/data/github/index.ts`:

```typescript
export { githubKeys } from "./keys";
export { useGitHubReposQuery } from "./queries";
```

**Step 4: Verify build**

Run: `cd web && npm run build`
Expected: Clean build (unused imports are fine at this stage)

**Step 5: Commit**

```bash
git add web/src/data/github/
git commit -m "feat: add useGitHubReposQuery hook"
```

---

### Task 6: Create RepoBrowser component

**Files:**
- Create: `web/src/components/RepoBrowser.tsx`

**Step 1: Implement the component**

Create `web/src/components/RepoBrowser.tsx`. This follows the same patterns as `FileBrowser.tsx` — search input, keyboard navigation, scrollable list.

```typescript
import { useState, useRef, useEffect, useCallback } from "react";
import { useGitHubReposQuery } from "@/data/github";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Search } from "lucide-react";

interface RepoBrowserProps {
  open: boolean;
  onSelect: (repo: string) => void;
  onClose: () => void;
  placeholder?: string;
  initialQuery?: string;
}

export function RepoBrowser({
  open,
  onSelect,
  onClose,
  placeholder = "Search repos or enter a URL...",
  initialQuery = "",
}: RepoBrowserProps) {
  const [query, setQuery] = useState(initialQuery);
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  // Debounce the query for the API call
  const [debouncedQuery, setDebouncedQuery] = useState(query);
  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const { data, isLoading } = useGitHubReposQuery(debouncedQuery, {
    enabled: open,
  });
  const repos = data?.repos ?? [];

  // Reset selection when results change
  useEffect(() => {
    setSelectedIndex(0);
  }, [repos.length, debouncedQuery]);

  // Focus input when opened
  useEffect(() => {
    if (open) {
      setQuery(initialQuery);
      setDebouncedQuery(initialQuery);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open, initialQuery]);

  // Scroll selected item into view
  useEffect(() => {
    const list = listRef.current;
    if (!list) return;
    const items = list.querySelectorAll("[data-repo-item]");
    items[selectedIndex]?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const handleSelect = useCallback(
    (repo: string) => {
      onSelect(repo);
    },
    [onSelect],
  );

  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        setSelectedIndex((i) => Math.min(i + 1, repos.length - 1));
        break;
      case "ArrowUp":
        e.preventDefault();
        setSelectedIndex((i) => Math.max(i - 1, 0));
        break;
      case "Enter":
        e.preventDefault();
        if (repos.length > 0 && selectedIndex < repos.length) {
          handleSelect(repos[selectedIndex]);
        } else if (query.trim()) {
          // Free-text fallback: submit the typed text as custom repo
          handleSelect(query.trim());
        }
        break;
      case "Escape":
        e.preventDefault();
        onClose();
        break;
    }
  };

  return (
    <div className="flex h-full flex-col">
      {/* Search input */}
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={placeholder}
          className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          spellCheck={false}
          autoComplete="off"
        />
      </div>

      {/* Results */}
      <ScrollArea className="flex-1">
        <div ref={listRef} className="py-1">
          {isLoading && repos.length === 0 && (
            <div className="px-3 py-8 text-center text-sm text-muted-foreground">
              Loading repositories...
            </div>
          )}

          {!isLoading && repos.length === 0 && !query.trim() && (
            <div className="px-3 py-8 text-center text-sm text-muted-foreground">
              {data
                ? "No repositories found. Is your GitHub token configured in ~/.argus/config.toml?"
                : "Type to search or enter a repo URL"}
            </div>
          )}

          {!isLoading && repos.length === 0 && query.trim() && (
            <div className="px-3 py-8 text-center text-sm text-muted-foreground">
              No matching repos. Press Enter to use "{query.trim()}" as-is.
            </div>
          )}

          {repos.map((repo, i) => (
            <button
              key={repo}
              data-repo-item
              type="button"
              onClick={() => handleSelect(repo)}
              className={`flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm ${
                i === selectedIndex
                  ? "bg-accent text-accent-foreground"
                  : "hover:bg-accent/50"
              }`}
            >
              {repo}
            </button>
          ))}
        </div>
      </ScrollArea>
    </div>
  );
}
```

**Step 2: Verify build**

Run: `cd web && npm run build`
Expected: Clean build

**Step 3: Commit**

```bash
git add web/src/components/RepoBrowser.tsx
git commit -m "feat: add RepoBrowser component with fuzzy search"
```

---

### Task 7: Create SourcePicker modal (replace DirectoryPicker)

**Files:**
- Create: `web/src/components/SourcePicker.tsx`
- Delete: `web/src/components/DirectoryPicker.tsx` (after migration)

**Step 1: Create SourcePicker**

Create `web/src/components/SourcePicker.tsx`. This wraps `FileBrowser` (Local tab) and `RepoBrowser` (Remote tab) in a dialog with tab switching.

```typescript
import { useCallback, useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn, contractTilde } from "@/lib/utils";
import { useFilesQuery } from "@/data/files";
import { useViewport } from "@/hooks/useViewport";
import { FileBrowser } from "./FileBrowser";
import { RepoBrowser } from "./RepoBrowser";

type SourceTab = "local" | "remote";

interface SourcePickerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (source: string, tab: SourceTab) => void;
  initialTab?: SourceTab;
  initialLocalPath?: string;
  initialRemoteQuery?: string;
}

export function SourcePicker({
  open,
  onOpenChange,
  onSelect,
  initialTab = "local",
  initialLocalPath = "~",
  initialRemoteQuery = "",
}: SourcePickerProps) {
  const { isMobile } = useViewport();
  const [tab, setTab] = useState<SourceTab>(initialTab);
  const [homePath, setHomePath] = useState("");

  const homeQuery = useFilesQuery("~", { enabled: !homePath });
  useEffect(() => {
    if (homeQuery.data?.path && !homePath) {
      setHomePath(homeQuery.data.path);
    }
  }, [homeQuery.data, homePath]);

  // Remember last tab across opens
  useEffect(() => {
    if (open && initialTab) {
      setTab(initialTab);
    }
  }, [open, initialTab]);

  const handleLocalSelect = useCallback(
    (absolutePath: string) => {
      const contracted = homePath
        ? contractTilde(absolutePath, homePath)
        : absolutePath;
      onSelect(contracted, "local");
      onOpenChange(false);
    },
    [homePath, onSelect, onOpenChange],
  );

  const handleRemoteSelect = useCallback(
    (repo: string) => {
      onSelect(repo, "remote");
      onOpenChange(false);
    },
    [onSelect, onOpenChange],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          "gap-0 overflow-hidden p-0",
          isMobile
            ? "top-[env(safe-area-inset-top)] left-0 right-0 h-[calc(var(--app-height)_-_env(safe-area-inset-top))] max-w-none translate-x-0 translate-y-0 rounded-none border-0"
            : "top-[50%] left-[50%] translate-x-[-50%] translate-y-[-50%] sm:max-w-md",
        )}
        showCloseButton={false}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>Select Source</DialogTitle>
        </DialogHeader>

        {/* Tab bar */}
        <div className="flex border-b">
          <button
            type="button"
            onClick={() => setTab("local")}
            className={cn(
              "flex-1 px-4 py-2 text-sm font-medium transition-colors",
              tab === "local"
                ? "border-b-2 border-foreground text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            Local
          </button>
          <button
            type="button"
            onClick={() => setTab("remote")}
            className={cn(
              "flex-1 px-4 py-2 text-sm font-medium transition-colors",
              tab === "remote"
                ? "border-b-2 border-foreground text-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            Remote
          </button>
        </div>

        {/* Tab content */}
        <div className={cn("flex-1 overflow-hidden", isMobile ? "h-full" : "h-[400px]")}>
          {tab === "local" ? (
            <FileBrowser
              open={open && tab === "local"}
              onSelect={handleLocalSelect}
              onClose={() => onOpenChange(false)}
              mode="directory"
              placeholder="Search folders or type a path..."
              initialQuery={initialLocalPath === "~" ? "" : initialLocalPath}
            />
          ) : (
            <RepoBrowser
              open={open && tab === "remote"}
              onSelect={handleRemoteSelect}
              onClose={() => onOpenChange(false)}
              placeholder="Search repos or enter a URL..."
              initialQuery={initialRemoteQuery}
            />
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
```

**Step 2: Verify build**

Run: `cd web && npm run build`
Expected: Clean build

**Step 3: Commit**

```bash
git add web/src/components/SourcePicker.tsx
git commit -m "feat: add SourcePicker modal with Local/Remote tabs"
```

---

### Task 8: Update NewSessionDialog to use SourcePicker

**Files:**
- Modify: `web/src/components/NewSessionDialog/index.tsx`

**Step 1: Rewrite the dialog**

Replace the current dialog with the simplified version:

```typescript
import { useState, useEffect, useRef } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { AgentSelector } from "./AgentSelector";
import { SourcePicker } from "@/components/SourcePicker";
import type { AgentType, CreateSessionParams } from "@/types";

type SourceTab = "local" | "remote";

interface NewSessionDialogProps {
  open: boolean;
  onClose: () => void;
  onCreateSession: (params: CreateSessionParams) => void;
}

export function NewSessionDialog({
  open,
  onClose,
  onCreateSession,
}: NewSessionDialogProps) {
  const [name, setName] = useState("");
  const [agentType, setAgentType] = useState<AgentType>("claude");
  const [source, setSource] = useState("");
  const [sourceTab, setSourceTab] = useState<SourceTab>("local");
  const [autoApprove, setAutoApprove] = useState(true);
  const [showSourcePicker, setShowSourcePicker] = useState(false);
  const sourcePickerClosingRef = useRef(false);

  useEffect(() => {
    if (open) {
      setName("");
      setAgentType("claude");
      setSource("");
      setSourceTab("local");
      setAutoApprove(true);
      setShowSourcePicker(false);
    }
  }, [open]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    const trimmedName = name.trim();
    if (!trimmedName) return;

    const params: CreateSessionParams = {
      name: trimmedName,
      agent_type: agentType,
    };

    if (source) {
      params.source = source;
    }

    if (autoApprove) {
      params.auto_approve = true;
    }

    onCreateSession(params);
    onClose();
  };

  return (
    <>
      <Dialog open={open} onOpenChange={(o) => !o && onClose()}>
        <DialogContent
          className="top-[env(safe-area-inset-top)] translate-y-0 max-h-[85vh] overflow-y-auto sm:top-[50%] sm:translate-y-[-50%]"
          onPointerDownOutside={(e) => {
            if (sourcePickerClosingRef.current) e.preventDefault();
          }}
          onFocusOutside={(e) => {
            if (sourcePickerClosingRef.current) {
              e.preventDefault();
              sourcePickerClosingRef.current = false;
            }
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter" && e.shiftKey) {
              e.preventDefault();
              handleSubmit(e as unknown as React.FormEvent);
            }
          }}
        >
          <DialogHeader>
            <DialogTitle>New Session</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <AgentSelector value={agentType} onChange={setAgentType} />

            <div className="space-y-2">
              <label className="text-sm font-medium">Name</label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-feature"
                autoFocus
              />
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">Source</label>
              <Input
                value={source}
                readOnly
                onClick={() => setShowSourcePicker(true)}
                placeholder="Click to select a folder or repository..."
                className="cursor-pointer"
              />
            </div>

            <div className="flex items-center justify-between">
              <div className="space-y-0.5">
                <label className="text-sm font-medium">Auto-approve</label>
                <p className="text-muted-foreground text-xs">
                  Skip permission prompts for tool calls
                </p>
              </div>
              <Switch
                checked={autoApprove}
                onCheckedChange={setAutoApprove}
              />
            </div>

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                Cancel
              </Button>
              <Button type="submit" disabled={!name.trim()}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <SourcePicker
        open={showSourcePicker}
        onOpenChange={(o) => {
          if (!o) sourcePickerClosingRef.current = true;
          setShowSourcePicker(o);
        }}
        onSelect={(value, tab) => {
          setSource(value);
          setSourceTab(tab);
        }}
        initialTab={sourceTab}
        initialLocalPath={sourceTab === "local" ? source : undefined}
        initialRemoteQuery={sourceTab === "remote" ? source : undefined}
      />
    </>
  );
}
```

Key changes:
- Removed `FolderOpen` import, `DirectoryPicker` import
- Removed `localDir`, `remoteRepo` state — replaced with single `source`
- Name is now required: `disabled={!name.trim()}` on Create button
- Source field is read-only clickable input that opens SourcePicker
- Tab switcher moved inside SourcePicker modal

**Step 2: Remove old DirectoryPicker import usage**

Check if `DirectoryPicker` is used anywhere else. If not, delete `web/src/components/DirectoryPicker.tsx`.

**Step 3: Verify build**

Run: `cd web && npm run build`
Expected: Clean build

**Step 4: Manual test**

- Open the new session dialog
- Verify Name field is present without "(optional)" label
- Verify Create button is disabled until name is entered
- Click Source field → SourcePicker modal opens
- Local tab: file browser works as before
- Remote tab: shows repo list (if token configured) or empty state
- Select a source → modal closes, Source field shows selection

**Step 5: Commit**

```bash
git add web/src/components/NewSessionDialog/index.tsx
git rm web/src/components/DirectoryPicker.tsx
git commit -m "feat: simplify new session dialog with SourcePicker"
```

---

### Task 9: Final cleanup and integration test

**Files:**
- Verify all imports/exports are clean

**Step 1: Run full backend tests**

Run: `go test ./...`
Expected: ALL PASS

**Step 2: Run full frontend build**

Run: `cd web && npm run build`
Expected: Clean build, no warnings

**Step 3: Run linters if configured**

Run: `cd web && npm run lint` (if exists)

**Step 4: Manual end-to-end test**

1. Set `github_token` in `~/.argus/config.toml`
2. Start the app
3. Create a new session:
   - Enter a name (required)
   - Click Source → Local tab → browse and select a directory
   - Verify source populates
4. Create another session:
   - Click Source → Remote tab → search for a repo
   - Verify fuzzy search works
   - Select a repo → verify source populates with `org/repo`
5. Try Remote tab with free-text: type a custom URL, press Enter

**Step 5: Commit any fixes, then final commit**

```bash
git add -A
git commit -m "chore: cleanup and verify session dialog UX improvements"
```
