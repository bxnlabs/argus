# Clone Sessions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let a user clone an existing Argus session so two CLI instances run against the same context (repo, worktree, profile) as independent agents.

**Architecture:** A thin `Manager.Clone(id)` loads the source session and delegates to the existing `Create` path with `Source` set to the source's working directory. Because `resolveSourceToCWD` already reuses an existing worktree path (no new branch, `branch_created=false`), worktree-sharing and branch-ownership semantics come for free. A new `POST /api/sessions/{id}/clone` endpoint and a "Clone" item in the session dropdown menu expose it. The clone starts a fresh CLI conversation (no `provider_session_id` copied).

**Tech Stack:** Go (net/http, stdlib `mux.HandleFunc`), SQLite via the existing `db` package, tmux; React + TypeScript + TanStack Query + Radix dropdown on the frontend.

---

## Spec reference

`docs/superpowers/specs/2026-06-04-clone-sessions-design.md`

## File structure

| File | Change | Responsibility |
| --- | --- | --- |
| `internal/node/session/lifecycle.go` | Modify | Add `Manager.Clone(id)` |
| `internal/node/session/lifecycle_test.go` | Modify | `TestCloneNotFound`, `TestCloneSession` |
| `internal/node/api/sessions.go` | Modify | Add `sessionHandler.clone` |
| `internal/node/api/sessions_test.go` | Modify | `TestCloneHandler_SessionNotFound` |
| `internal/node/api/router.go` | Modify | Register `POST /api/sessions/{id}/clone` |
| `web/src/data/sessions/queries.ts` | Modify | `useCloneSession` hook |
| `web/src/data/sessions/index.ts` | Modify | Re-export `useCloneSession` |
| `web/src/App.tsx` | Modify | `handleCloneSession`, thread `onCloneSession` |
| `web/src/components/views/types.ts` | Modify | Add `onCloneSession` to `ViewProps` |
| `web/src/components/views/DesktopView.tsx` | Modify | Pass `onCloneSession` to `SessionList` |
| `web/src/components/views/MobileView.tsx` | Modify | Pass `onCloneSession` to `SessionList` |
| `web/src/components/SessionList/index.tsx` | Modify | Thread `onCloneSession`, add Clone menu item |

## Testing note (frontend)

The existing `web/src/components/SessionList/index.test.tsx` tests **pure exported helpers** (`partitionSessions`, `readMenuState`, `resolveStatusDisplay`), not menu rendering. The Clone menu item is one-line wiring identical to the existing `onViewInfo` item and would only be testable by driving a Radix dropdown portal in jsdom — brittle and without precedent in this file. Per repo precedent and simplicity, the frontend changes are verified by `tsc` typecheck (which validates the entire `onCloneSession` prop-threading chain) plus the existing test suite staying green. No new RTL test is added.

---

## Task 1: `Manager.Clone` (backend method)

**Files:**
- Modify: `internal/node/session/lifecycle.go` (add method near `Create`)
- Test: `internal/node/session/lifecycle_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/node/session/lifecycle_test.go`. `TestCloneNotFound` needs no tmux; `TestCloneSession` is tmux-guarded and uses Argus's dedicated socket (never the default socket), mirroring `TestChangeProfile`.

```go
func TestCloneNotFound(t *testing.T) {
	stateDir := t.TempDir()
	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	if _, err := mgr.Clone("does-not-exist"); err == nil {
		t.Fatal("expected error cloning missing session, got nil")
	} else if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestCloneSession(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not available")
	}
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	// Create spawns tmux for the clone via the dedicated server, which is booted
	// with the seeded config (-f). Point ARGUS_HOME at stateDir, force the
	// dedicated socket, and seed the config exactly as the node does at startup.
	t.Setenv("ARGUS_HOME", stateDir)
	requireDedicatedSocketUnder(t, stateDir)
	if _, err := shared.EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	if _, err := shared.SeedTmuxConfig(); err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	// Profile "work" must exist on disk so Create's profile validation passes.
	if err := os.MkdirAll(filepath.Join(stateDir, "profiles", "work", "hooks"), 0755); err != nil {
		t.Fatal(err)
	}

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	// A real worktree the clone can reuse via its path.
	wtPath, branch, _, _, err := wt.CreateForLocalRepo(gitRoot, "src-work", "")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	model := "opus"
	profile := "work"
	providerSessionID := "prov-123"
	src := &db.Session{
		ID: "sess-src", Name: "My Work", TmuxName: "claude-sess-src",
		WorkingDirectory: wtPath, ProviderType: "claude",
		Model: &model, AutoApprove: true, Profile: &profile,
		ProviderSessionID: &providerSessionID,
		WorktreeBranch:    &branch, BranchCreated: true, GitParentDir: &gitRoot,
	}
	if err := database.CreateSession(src); err != nil {
		t.Fatal(err)
	}

	clone, err := mgr.Clone("sess-src")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	t.Cleanup(func() { KillSession(clone.TmuxName) })

	if clone.ID == src.ID {
		t.Error("clone must have a new ID")
	}
	if clone.Name != "My Work (copy)" {
		t.Errorf("clone name = %q, want %q", clone.Name, "My Work (copy)")
	}
	if clone.WorkingDirectory != wtPath {
		t.Errorf("clone working_directory = %q, want %q (shared worktree)", clone.WorkingDirectory, wtPath)
	}
	if clone.WorktreeBranch == nil || *clone.WorktreeBranch != branch {
		t.Errorf("clone worktree_branch = %v, want %q", clone.WorktreeBranch, branch)
	}
	if clone.BranchCreated {
		t.Error("clone branch_created must be false (reused worktree, does not own branch)")
	}
	if clone.ProviderSessionID != nil {
		t.Errorf("clone provider_session_id = %v, want nil (fresh conversation)", clone.ProviderSessionID)
	}
	if clone.ProviderType != "claude" {
		t.Errorf("clone provider_type = %q, want %q", clone.ProviderType, "claude")
	}
	if clone.Model == nil || *clone.Model != "opus" {
		t.Errorf("clone model = %v, want %q", clone.Model, "opus")
	}
	if !clone.AutoApprove {
		t.Error("clone auto_approve = false, want true")
	}
	if clone.Profile == nil || *clone.Profile != "work" {
		t.Errorf("clone profile = %v, want %q", clone.Profile, "work")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/node/session/ -run 'TestClone' -v`
Expected: compile failure — `mgr.Clone undefined (type *Manager has no field or method Clone)`.

- [ ] **Step 3: Implement `Manager.Clone`**

Add to `internal/node/session/lifecycle.go`, immediately after the `Create` method (after the closing brace at line 255):

```go
// Clone creates a new session that shares the source session's context — the
// same working directory (and therefore the same worktree and branch for
// worktree-backed sessions), provider, model, system prompt, auto-approve, and
// profile. The clone starts a fresh CLI conversation: provider_session_id is
// not copied. Because the source's working directory is passed as the new
// session's Source, resolveSourceToCWD reuses the existing worktree (no new
// branch; the clone does not own the branch). Returns ErrNotFound if the source
// session does not exist.
func (m *Manager) Clone(id string) (*db.Session, error) {
	src, err := m.db.GetSession(id)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	return m.Create(CreateOptions{
		Name:         src.Name + " (copy)",
		ProviderType: src.ProviderType,
		Source:       src.WorkingDirectory,
		Model:        src.Model,
		SystemPrompt: src.SystemPrompt,
		AutoApprove:  src.AutoApprove,
		Profile:      src.Profile,
	})
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/node/session/ -run 'TestClone' -v`
Expected: PASS (`TestCloneNotFound`, `TestCloneSession`). If tmux is unavailable in the environment, `TestCloneSession` reports SKIP — still acceptable; `TestCloneNotFound` must PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/node/session/lifecycle.go internal/node/session/lifecycle_test.go
git commit -m "feat(session): add Manager.Clone for cloning a session's context (BXN-107)"
```

---

## Task 2: Clone API endpoint

**Files:**
- Modify: `internal/node/api/sessions.go` (add `clone` handler)
- Modify: `internal/node/api/router.go` (register route)
- Test: `internal/node/api/sessions_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/node/api/sessions_test.go`. The not-found path returns before any tmux spawn, so no tmux setup is needed. (The success path is covered end-to-end by `TestCloneSession` in Task 1.)

```go
func TestCloneHandler_SessionNotFound(t *testing.T) {
	h, _ := newTestSessionHandler(t)

	req := httptest.NewRequest("POST", "/api/sessions/missing/clone", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()

	h.clone(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing session, got %d", w.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/node/api/ -run TestCloneHandler -v`
Expected: compile failure — `h.clone undefined (type *sessionHandler has no field or method clone)`.

- [ ] **Step 3: Implement the handler**

Add to `internal/node/api/sessions.go`, immediately after the `create` handler (after its closing brace at line 73):

```go
// POST /api/sessions/{id}/clone
func (h *sessionHandler) clone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	sess, err := h.manager.Clone(id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}

	if h.watcherManager != nil {
		h.watcherManager.EnsureWatching(sess.ID, sess.TmuxName, sess.ProviderType)
	}

	respondJSON(w, http.StatusCreated, map[string]any{"session": sess})
}
```

- [ ] **Step 4: Register the route**

In `internal/node/api/router.go`, add the clone route directly below the `POST /api/sessions` line (line 32):

```go
	mux.HandleFunc("POST /api/sessions/{id}/clone", sh.clone)
```

So the block reads:

```go
	mux.HandleFunc("GET /api/sessions", sh.list)
	mux.HandleFunc("POST /api/sessions", sh.create)
	mux.HandleFunc("POST /api/sessions/{id}/clone", sh.clone)
	mux.HandleFunc("GET /api/sessions/{id}", sh.get)
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/node/api/ -run TestCloneHandler -v`
Expected: PASS.

- [ ] **Step 6: Run the full backend build + vet to confirm nothing else broke**

Run: `go build ./... && go vet ./internal/node/...`
Expected: no output (success).

- [ ] **Step 7: Commit**

```bash
git add internal/node/api/sessions.go internal/node/api/router.go internal/node/api/sessions_test.go
git commit -m "feat(api): add POST /api/sessions/{id}/clone endpoint (BXN-107)"
```

---

## Task 3: `useCloneSession` query hook

**Files:**
- Modify: `web/src/data/sessions/queries.ts`
- Modify: `web/src/data/sessions/index.ts`

- [ ] **Step 1: Add the hook**

In `web/src/data/sessions/queries.ts`, add directly after `useCreateSession` (after its closing brace at line 47). It reuses the existing `CreateSessionResponse` interface (declared just above `useCreateSession`):

```ts
export function useCloneSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (sessionId: string) =>
      apiFetch<CreateSessionResponse>(
        `/node/api/sessions/${sessionId}/clone`,
        { method: "POST" },
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}
```

- [ ] **Step 2: Re-export the hook**

`web/src/data/sessions/index.ts` re-exports the query hooks. Add `useCloneSession` to the export list from `./queries` (it currently lists `useCreateSession`, `useDeleteSession`, `useRenameSession`, etc.). Open the file and add the name alongside `useCreateSession`:

```ts
  useCreateSession,
  useCloneSession,
```

- [ ] **Step 3: Typecheck**

Run: `cd web && pnpm exec tsc -b`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/data/sessions/queries.ts web/src/data/sessions/index.ts
git commit -m "feat(web): add useCloneSession mutation hook (BXN-107)"
```

---

## Task 4: Wire the Clone menu item through the UI

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/components/views/types.ts`
- Modify: `web/src/components/views/DesktopView.tsx`
- Modify: `web/src/components/views/MobileView.tsx`
- Modify: `web/src/components/SessionList/index.tsx`

- [ ] **Step 1: App.tsx — import and instantiate the clone mutation**

In `web/src/App.tsx`, the existing import at line 11 reads:

```ts
import { useCreateSession } from "@/data/sessions/queries";
```

Change it to:

```ts
import { useCreateSession, useCloneSession } from "@/data/sessions/queries";
```

Then, directly after the create mutation ref setup (lines 71–73):

```ts
  const createSessionMutation = useCreateSession();
  const createMutateRef = useRef(createSessionMutation.mutateAsync);
  createMutateRef.current = createSessionMutation.mutateAsync;
```

add:

```ts
  const cloneSessionMutation = useCloneSession();
  const cloneMutateRef = useRef(cloneSessionMutation.mutateAsync);
  cloneMutateRef.current = cloneSessionMutation.mutateAsync;
```

- [ ] **Step 2: App.tsx — add the clone handler**

Directly after `handleCreateSession` (after its closing `);` at line 310), add:

```ts
  // Clone session handler — creates a sibling session sharing the same context
  // (worktree, profile, provider) and attaches to it.
  const handleCloneSession = useCallback(
    async (sessionId: string) => {
      try {
        const result = await cloneMutateRef.current(sessionId);
        if (result.session) {
          attachToSession(result.session);
        }
      } catch (err) {
        console.error("Failed to clone session:", err);
        toast.error("Failed to clone session");
      }
    },
    [attachToSession]
  );
```

- [ ] **Step 3: App.tsx — pass the handler into viewProps**

In the `viewProps` object, the create/delete/rename handlers are wired (lines 422–424):

```ts
    onCreateSession: handleCreateSession,
    onDeleteSession: handleDeleteSession,
    onRenameSession: handleRenameSession,
```

Add below them:

```ts
    onCloneSession: handleCloneSession,
```

- [ ] **Step 4: views/types.ts — extend ViewProps**

In `web/src/components/views/types.ts`, the session operations block lists `onRenameSession` (line 30). Add directly after it:

```ts
  onCloneSession: (sessionId: string) => void;
```

- [ ] **Step 5: DesktopView.tsx — forward the prop**

In `web/src/components/views/DesktopView.tsx`, the destructured props include `onRenameSession` (line 29). Add `onCloneSession` to the destructure:

```ts
  onCloneSession,
```

Then in the `<SessionList ... />` usage (where `onRenameSession={onRenameSession}` appears at line 134), add:

```tsx
                onCloneSession={onCloneSession}
```

- [ ] **Step 6: MobileView.tsx — forward the prop**

In `web/src/components/views/MobileView.tsx`, mirror DesktopView. Add `onCloneSession` to the destructured props (alongside `onRenameSession` at line 23):

```ts
  onCloneSession,
```

And in its `<SessionList ... />` usage (next to `onRenameSession={onRenameSession}` at line 100):

```tsx
                    onCloneSession={onCloneSession}
```

- [ ] **Step 7: SessionList/index.tsx — import the Copy icon**

The lucide import at line 12 is:

```ts
import { Plus, AlertCircle, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch, BrushCleaning, Settings2, Pin, MailOpen, Mail, Info, ChevronRight } from "lucide-react";
```

Add `Copy`:

```ts
import { Plus, AlertCircle, Ellipsis, Pencil, Trash2, Folder, FolderGit2, GitBranch, BrushCleaning, Settings2, Pin, MailOpen, Mail, Info, ChevronRight, Copy } from "lucide-react";
```

- [ ] **Step 8: SessionList/index.tsx — thread onCloneSession into SessionItem**

In `SessionItemProps` (interface starting line 108), `onDeleteSession` is declared at line 124. Add directly after it:

```ts
  onCloneSession: (sessionId: string) => void;
```

In the `SessionItem` destructure (the function params starting line 134), `onDeleteSession` appears at line 150. Add after it:

```ts
  onCloneSession,
```

- [ ] **Step 9: SessionList/index.tsx — add the Clone menu item**

In `SessionItem`'s dropdown, the "Change profile" item closes at line 328 and the "Info" item begins at line 329. Insert the Clone item between them:

```tsx
          <DropdownMenuItem
            onClick={(e) => {
              e.stopPropagation();
              onCloneSession(session.id);
            }}
          >
            <Copy className="mr-2 h-3 w-3" />
            Clone
          </DropdownMenuItem>
```

- [ ] **Step 10: SessionList/index.tsx — thread onCloneSession through SessionList props**

In `SessionListProps` (interface starting line 370), `onDeleteSession` is declared at line 379. Add after it:

```ts
  onCloneSession: (sessionId: string) => void;
```

In the `SessionList` destructure (params starting line 390), `onDeleteSession` appears at line 399. Add after it:

```ts
  onCloneSession,
```

In the `<SessionItem ... />` render (lines 476–501), `onDeleteSession={onDeleteSession}` appears at line 493. Add after it:

```tsx
        onCloneSession={onCloneSession}
```

- [ ] **Step 11: Typecheck and run the frontend test suite**

Run: `cd web && pnpm exec tsc -b && pnpm test`
Expected: no type errors; all existing tests pass. The `tsc` pass validates the full `onCloneSession` threading chain (App → ViewProps → Desktop/MobileView → SessionList → SessionItem).

- [ ] **Step 12: Production build sanity check**

Run: `cd web && pnpm build`
Expected: build succeeds (tsc + vite).

- [ ] **Step 13: Commit**

```bash
git add web/src/App.tsx web/src/components/views/types.ts web/src/components/views/DesktopView.tsx web/src/components/views/MobileView.tsx web/src/components/SessionList/index.tsx
git commit -m "feat(web): add Clone action to session menu (BXN-107)"
```

---

## Final verification

- [ ] **Run the full backend test suite**

Run: `go test ./internal/node/...`
Expected: PASS (tmux-dependent tests may SKIP if tmux is unavailable).

- [ ] **Manual smoke test (optional, requires `make dev`)**

1. Start the dev stack (`make dev`), open the web UI.
2. Create a worktree-backed session, then open its `⋯` menu and click **Clone**.
3. Confirm a new session named `"<name> (copy)"` appears and is selected, its terminal opens a fresh CLI, and (via the Info dialog) it shares the same working directory and branch as the original.
4. Confirm the original session is untouched and both can run concurrently against the shared worktree.

## Out of scope (from spec)

- Resuming/forking the original agent conversation into the clone.
- Pre-filled New-Session dialog or per-clone setting overrides.
- Smart clone naming / de-duplication of `(copy)` suffixes.
