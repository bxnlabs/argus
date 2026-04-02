# Branch Deletion on Session Remove Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional branch deletion when removing sessions, gated by ownership checks, with CLI flag and web UI context menu support.

**Architecture:** Add `DeleteBranch` method to worktree manager, extend `session.Manager.Delete` with preflight eligibility checks and best-effort branch deletion in the destructive phase, surface `branch_deleted` in the API response, add `--delete-branch` CLI flag, and add "Delete with branch" context menu item in the web UI.

**Tech Stack:** Go (backend), cobra (CLI), React + TanStack Query (web UI)

**Spec:** `docs/specs/2026-04-02-branch-deletion-on-session-remove-design.md`

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/git/worktree/manager.go` | Add `DeleteBranch` method |
| Modify | `internal/git/worktree/manager_test.go` | Tests for `DeleteBranch` |
| Modify | `internal/node/session/lifecycle.go` | Extend `Delete` with `deleteBranch` param, eligibility checks, best-effort branch deletion, return `DeleteResult` |
| Modify | `internal/node/session/lifecycle_test.go` | Tests for branch deletion in `Delete` |
| Modify | `internal/node/api/sessions.go` | Parse `delete_branch` query param, return `branch_deleted` in response |
| Modify | `cmd/argus/cli/session_delete.go` | Add `--delete-branch` flag, use `url.Values`, parse `branch_deleted` response |
| Modify | `web/src/data/sessions/queries.ts` | Accept `{ sessionId, deleteBranch }` in mutation |
| Modify | `web/src/hooks/useSessions.ts` | Pass `deleteBranch` param through `deleteSession` callback |
| Modify | `web/src/components/SessionList/index.tsx` | Add "Delete with branch" context menu item, update `onDeleteSession` prop type |

---

### Task 1: Add `DeleteBranch` to worktree manager

**Files:**
- Modify: `internal/git/worktree/manager.go` (append after `Cleanup` method, ~line 261)
- Test: `internal/git/worktree/manager_test.go`

- [ ] **Step 1: Write the failing test for successful branch deletion**

Add to `internal/git/worktree/manager_test.go`:

```go
func TestDeleteBranch(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}})

	// Create a worktree (which creates the branch), then remove it (preserving the branch).
	wtPath, branch, _, err := mgr.CreateForLocalRepo(gitRoot, "delete-me")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}
	if err := mgr.RemoveWorktree(wtPath, true); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}

	// Branch should still exist after worktree removal.
	out, err := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Fatalf("branch %q should exist after worktree removal", branch)
	}

	// Delete the branch.
	if err := mgr.DeleteBranch(gitRoot, branch); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}

	// Branch should be gone.
	out, err = exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if err != nil {
		t.Fatalf("git branch --list after delete: %v", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("branch %q should not exist after DeleteBranch", branch)
	}
}

func TestDeleteBranchNonexistent(t *testing.T) {
	gitRoot := initGitRepo(t)
	stateDir := t.TempDir()

	mgr := worktree.NewManager(stateDir, &config.Config{})

	err := mgr.DeleteBranch(gitRoot, "nonexistent-branch")
	if err == nil {
		t.Fatal("expected error deleting nonexistent branch, got nil")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go test ./internal/git/worktree/ -run "TestDeleteBranch" -v`
Expected: FAIL with "mgr.DeleteBranch undefined"

- [ ] **Step 3: Implement `DeleteBranch`**

Add to `internal/git/worktree/manager.go` after the `Cleanup` method:

```go
// DeleteBranch force-deletes a local branch (git branch -D).
func (m *Manager) DeleteBranch(repoDir, branch string) error {
	if err := git.Run(repoDir, "branch", "-D", branch); err != nil {
		return fmt.Errorf("git branch -D %s: %w", branch, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go test ./internal/git/worktree/ -run "TestDeleteBranch" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/git/worktree/manager.go internal/git/worktree/manager_test.go
git commit -m "feat: add DeleteBranch method to worktree manager"
```

---

### Task 2: Extend `session.Manager.Delete` with branch deletion

**Files:**
- Modify: `internal/node/session/lifecycle.go` (lines 314-387)
- Test: `internal/node/session/lifecycle_test.go`

- [ ] **Step 1: Write the failing test — branch deleted on eligible session**

Add to `internal/node/session/lifecycle_test.go`:

```go
func TestDeleteWithBranchDeletion(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "branch-del")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID:               "sess-branch-del",
		Name:             "branch-del",
		TmuxName:         "claude-sess-branch-del",
		WorkingDirectory: wtPath,
		ProviderType:     "claude",
		WorktreeBranch:   &branch,
		GitParentDir:     &gitRoot,
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	result, err := mgr.Delete(sess.ID, true, true)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !result.BranchDeleted {
		t.Error("expected BranchDeleted=true")
	}

	// Session should be gone
	got, err := database.GetSession(sess.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got != nil {
		t.Fatal("session should be deleted")
	}

	// Worktree should be gone
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree should be removed")
	}

	// Branch should be gone
	out, gitErr := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if gitErr != nil {
		t.Fatalf("git branch --list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Errorf("branch %q should not exist after delete with deleteBranch=true", branch)
	}
}
```

- [ ] **Step 2: Write the failing test — shared worktree skips branch deletion**

Add to `internal/node/session/lifecycle_test.go`:

```go
func TestDeleteWithBranchDeletionSharedWorktree(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "shared-wt")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	// Create two sessions pointing at the same worktree
	sess1 := &db.Session{
		ID: "sess-shared-1", Name: "shared-1", TmuxName: "claude-sess-shared-1",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch, GitParentDir: &gitRoot,
	}
	sess2 := &db.Session{
		ID: "sess-shared-2", Name: "shared-2", TmuxName: "claude-sess-shared-2",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch, GitParentDir: &gitRoot,
	}
	if err := database.CreateSession(sess1); err != nil {
		t.Fatal(err)
	}
	if err := database.CreateSession(sess2); err != nil {
		t.Fatal(err)
	}

	// Delete first session with deleteBranch — should skip branch deletion
	result, err := mgr.Delete(sess1.ID, true, true)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if result.BranchDeleted {
		t.Error("expected BranchDeleted=false for shared worktree")
	}

	// Branch should still exist
	out, gitErr := exec.Command("git", "-C", gitRoot, "branch", "--list", branch).Output()
	if gitErr != nil {
		t.Fatalf("git branch --list: %v", gitErr)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Error("branch should still exist when other sessions share the worktree")
	}
}
```

- [ ] **Step 3: Write the failing test — nil GitParentDir returns error**

Add to `internal/node/session/lifecycle_test.go`:

```go
func TestDeleteWithBranchDeletionNilGitParentDir(t *testing.T) {
	gitRoot := resolveSymlinks(t, initTestGitRepo(t))
	stateDir := resolveSymlinks(t, t.TempDir())

	database, err := db.Open(filepath.Join(stateDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	wt := worktree.NewManager(stateDir, &config.Config{Git: config.GitConfig{BranchPrefix: "test"}})
	mgr := NewManager(database, wt, stateDir)

	wtPath, branch, _, err := wt.CreateForLocalRepo(gitRoot, "no-parent")
	if err != nil {
		t.Fatalf("CreateForLocalRepo: %v", err)
	}

	sess := &db.Session{
		ID: "sess-no-parent", Name: "no-parent", TmuxName: "claude-sess-no-parent",
		WorkingDirectory: wtPath, ProviderType: "claude",
		WorktreeBranch: &branch,
		GitParentDir:   nil, // intentionally nil
	}
	if err := database.CreateSession(sess); err != nil {
		t.Fatal(err)
	}

	// Should fail because GitParentDir is nil
	_, err = mgr.Delete(sess.ID, true, true)
	if err == nil {
		t.Fatal("expected error for nil GitParentDir, got nil")
	}

	// Session should still exist (preflight failed, no side effects)
	got, dbErr := database.GetSession(sess.ID)
	if dbErr != nil {
		t.Fatalf("GetSession: %v", dbErr)
	}
	if got == nil {
		t.Fatal("session should still exist after preflight failure")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go test ./internal/node/session/ -run "TestDeleteWithBranch" -v`
Expected: FAIL — `Delete` signature mismatch (too many arguments)

- [ ] **Step 5: Implement the changes to `Manager.Delete`**

In `internal/node/session/lifecycle.go`:

1. Add the `DeleteResult` type before `Delete`:

```go
// DeleteResult contains the outcome of a session deletion.
type DeleteResult struct {
	BranchDeleted bool
}
```

2. Replace the `Delete` method signature and body (lines 314-387):

```go
// Delete kills the tmux session and removes from DB. For worktree-backed
// sessions, the worktree is removed but the branch is preserved unless
// deleteBranch is true. If the worktree has uncommitted changes and force
// is false, the delete is refused.
func (m *Manager) Delete(id string, force, deleteBranch bool) (*DeleteResult, error) {
	l := m.sessionLock(id)
	l.Lock()
	defer l.Unlock()

	session, err := m.db.GetSession(id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("%w: %s", db.ErrNotFound, id)
	}

	// Preflight: check for dirty worktree BEFORE any side effects.
	// This ensures that a failed delete (dirty worktree, force=false)
	// leaves the session fully intact (tmux alive, DB row untouched).
	needsWorktreeRemoval := false
	needsBranchDeletion := false
	var branchRepoDir string

	if session.WorktreeBranch != nil && m.wt.IsManaged(session.WorkingDirectory) {
		others, err := m.db.CountSessionsByWorkingDir(id, session.WorkingDirectory)
		if err != nil {
			return nil, fmt.Errorf("check shared worktree: %w", err)
		}
		if others == 0 {
			if _, statErr := os.Stat(session.WorkingDirectory); os.IsNotExist(statErr) {
				// Worktree was removed externally; skip git cleanup.
			} else {
				if !force {
					if err := m.wt.CheckWorktreeDirty(session.WorkingDirectory); err != nil {
						return nil, err
					}
				}
				needsWorktreeRemoval = true
			}

			// Branch deletion eligibility: only when this is the last session
			// for this worktree and the worktree is managed.
			if deleteBranch {
				if session.GitParentDir == nil {
					return nil, fmt.Errorf("%w: cannot delete branch without git_parent_dir; re-create the session or backfill metadata", ErrInvalidInput)
				}
				branchRepoDir = *session.GitParentDir
				needsBranchDeletion = true
			}
		}
	}

	result := &DeleteResult{}

	projectKey := ProjectKeyForSession(session)
	profileName := ptrStr(session.Profile)

	hookEnv := HookEnv{
		SessionID:    session.ID,
		WorkingDir:   session.WorkingDirectory,
		ProviderType: session.ProviderType,
		Profile:      profileName,
	}

	// pre_destroy: LIFO order (project first, then profile), best-effort
	preDestroyPaths := m.hooks.ResolveHookPathsTeardown(HookPreDestroy, profileName, projectKey)
	m.hooks.RunHooksBestEffort(preDestroyPaths, hookEnv)

	// Kill tmux (ignore error if already dead)
	if HasSession(session.TmuxName) {
		KillSession(session.TmuxName)
	}

	// Remove worktree. The preflight dirty check already validated the
	// user's intent (worktree was clean or force=true), so force-remove
	// here to avoid TOCTOU issues (e.g. pre_destroy hooks writing files).
	if needsWorktreeRemoval {
		if err := m.wt.RemoveWorktree(session.WorkingDirectory, true); err != nil {
			return nil, err
		}
	}

	// Delete branch (best-effort). If this fails, log and continue —
	// the branch survives, identical to today's default behavior.
	if needsBranchDeletion {
		if err := m.wt.DeleteBranch(branchRepoDir, *session.WorktreeBranch); err != nil {
			log.Printf("best-effort branch deletion failed for %s: %v", *session.WorktreeBranch, err)
		} else {
			result.BranchDeleted = true
		}
	}

	// Delete DB record
	if err := m.db.DeleteSession(id); err != nil {
		return nil, err
	}

	return result, nil
}
```

- [ ] **Step 6: Fix existing callers of `Delete`**

The only caller outside tests is `internal/node/api/sessions.go:132`. Update it temporarily to compile (we'll update it properly in Task 3):

In `internal/node/api/sessions.go`, change line 132 from:
```go
	if err := h.manager.Delete(id, force); err != nil {
```
to:
```go
	if _, err := h.manager.Delete(id, force, false); err != nil {
```

- [ ] **Step 7: Fix existing test calls to `Delete`**

In `internal/node/session/lifecycle_test.go`, update existing calls. There are three existing tests that call `mgr.Delete`:

1. `TestDeleteDirtyWorktreeBlocksBeforeSideEffects` (line 159):
   Change `err = mgr.Delete(sess.ID, false)` to `_, err = mgr.Delete(sess.ID, false, false)`

2. `TestDeletePreDestroyHookDirtyingWorktreeStillSucceeds` (line 222):
   Change `if err := mgr.Delete(sess.ID, false); err != nil {` to `if _, err := mgr.Delete(sess.ID, false, false); err != nil {`

3. `TestDeleteForceBypassesDirtyCheck` (line 278):
   Change `if err := mgr.Delete(sess.ID, true); err != nil {` to `if _, err := mgr.Delete(sess.ID, true, false); err != nil {`

- [ ] **Step 8: Add `"strings"` import to lifecycle_test.go**

The new tests use `strings.TrimSpace`. Add `"strings"` to the import block in `internal/node/session/lifecycle_test.go` if not already present.

- [ ] **Step 9: Run all tests to verify they pass**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go test ./internal/node/session/ -v`
Expected: ALL PASS

- [ ] **Step 10: Commit**

```bash
git add internal/node/session/lifecycle.go internal/node/session/lifecycle_test.go internal/node/api/sessions.go
git commit -m "feat: extend session Delete with optional branch deletion"
```

---

### Task 3: Update API handler to parse `delete_branch` and return `branch_deleted`

**Files:**
- Modify: `internal/node/api/sessions.go` (lines 128-145)

- [ ] **Step 1: Update the `delete` handler**

Replace the `delete` method in `internal/node/api/sessions.go` (lines 128-145):

```go
// DELETE /api/sessions/{id}
func (h *sessionHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	force := r.URL.Query().Get("force") == "true"
	deleteBranch := r.URL.Query().Get("delete_branch") == "true"

	result, err := h.manager.Delete(id, force, deleteBranch)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		if errors.Is(err, worktree.ErrWorktreeDirty) {
			respondError(w, http.StatusConflict, "worktree has uncommitted changes; use force to delete anyway")
			return
		}
		if errors.Is(err, session.ErrInvalidInput) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		respondInternalError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"branch_deleted": result.BranchDeleted,
	})
}
```

- [ ] **Step 2: Run the full test suite to verify nothing is broken**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go test ./internal/... -v`
Expected: ALL PASS

- [ ] **Step 3: Commit**

```bash
git add internal/node/api/sessions.go
git commit -m "feat: parse delete_branch param and return branch_deleted in API response"
```

---

### Task 4: Update CLI to support `--delete-branch` flag

**Files:**
- Modify: `cmd/argus/cli/session_delete.go`

- [ ] **Step 1: Update the delete command**

Replace the full content of `cmd/argus/cli/session_delete.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

func newDeleteCmd() *cobra.Command {
	var force bool
	var deleteBranch bool

	cmd := &cobra.Command{
		Use:   "rm <name-or-id>",
		Short: "Delete a session",
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

			session, err := fetchAndResolve(c, query)
			if err != nil {
				return err
			}

			params := url.Values{}
			if force {
				params.Set("force", "true")
			}
			if deleteBranch {
				params.Set("delete_branch", "true")
			}

			endpoint := "/api/sessions/" + session.ID
			if len(params) > 0 {
				endpoint += "?" + params.Encode()
			}

			body, err := c.delete(endpoint)
			if err != nil {
				return err
			}

			fmt.Printf("Deleted session %q\n", session.Name)

			if deleteBranch && session.WorktreeBranch != nil {
				var resp struct {
					BranchDeleted bool `json:"branch_deleted"`
				}
				if err := json.Unmarshal(body, &resp); err == nil {
					if resp.BranchDeleted {
						fmt.Printf("Deleted branch %q\n", *session.WorktreeBranch)
					} else {
						fmt.Printf("Branch %q was not deleted (not eligible or could not be removed)\n", *session.WorktreeBranch)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Force delete even if worktree has uncommitted changes")
	cmd.Flags().BoolVar(&deleteBranch, "delete-branch", false, "Also delete the git branch")

	return cmd
}
```

- [ ] **Step 2: Verify the CLI compiles**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go build ./cmd/argus/`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add cmd/argus/cli/session_delete.go
git commit -m "feat: add --delete-branch flag to argus session rm"
```

---

### Task 5: Update web UI mutation to support `deleteBranch`

**Files:**
- Modify: `web/src/data/sessions/queries.ts` (lines 48-58)

- [ ] **Step 1: Update `useDeleteSession` mutation**

In `web/src/data/sessions/queries.ts`, replace the `useDeleteSession` function (lines 48-58):

```ts
export function useDeleteSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      sessionId,
      deleteBranch,
    }: {
      sessionId: string;
      deleteBranch?: boolean;
    }) =>
      apiFetch(
        `/node/api/sessions/${sessionId}?force=true${deleteBranch ? "&delete_branch=true" : ""}`,
        { method: "DELETE" },
      ),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: sessionKeys.list() });
    },
  });
}
```

- [ ] **Step 2: Verify the web app compiles**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion/web && npx tsc --noEmit`
Expected: No errors (or only pre-existing errors)

- [ ] **Step 3: Commit**

```bash
git add web/src/data/sessions/queries.ts
git commit -m "feat: accept deleteBranch param in useDeleteSession mutation"
```

---

### Task 6: Update `useSessions` hook to pass `deleteBranch`

**Files:**
- Modify: `web/src/hooks/useSessions.ts`

- [ ] **Step 1: Update the `deleteSession` callback**

Replace the full content of `web/src/hooks/useSessions.ts`:

```ts
import { useCallback } from "react";
import type { Session } from "@/types";
import {
  useSessionsQuery,
  useDeleteSession,
  useRenameSession,
} from "@/data/sessions";

export function useSessions() {
  const { data, isSuccess } = useSessionsQuery();
  const sessions: Session[] = data?.sessions ?? [];
  const homeDir: string = data?.home_dir ?? "";

  const deleteMutation = useDeleteSession();
  const renameMutation = useRenameSession();

  const deleteSession = useCallback(
    async (sessionId: string, deleteBranch?: boolean) => {
      const message = deleteBranch
        ? "Delete this session and its branch? This cannot be undone."
        : "Delete this session? This cannot be undone.";
      if (!confirm(message)) return;
      await deleteMutation.mutateAsync({ sessionId, deleteBranch });
    },
    [deleteMutation],
  );

  const renameSession = useCallback(
    async (sessionId: string, newName: string) => {
      await renameMutation.mutateAsync({ sessionId, newName });
    },
    [renameMutation],
  );

  return { sessions, homeDir, isLoaded: isSuccess, deleteSession, renameSession };
}
```

- [ ] **Step 2: Verify the web app compiles**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion/web && npx tsc --noEmit`
Expected: No errors (or only pre-existing errors)

- [ ] **Step 3: Commit**

```bash
git add web/src/hooks/useSessions.ts
git commit -m "feat: pass deleteBranch through useSessions deleteSession callback"
```

---

### Task 7: Add "Delete with branch" context menu item

**Files:**
- Modify: `web/src/components/SessionList/index.tsx` (lines 60-73 for props, lines 308-317 for menu)

- [ ] **Step 1: Update the `onDeleteSession` prop type**

In `web/src/components/SessionList/index.tsx`, change the `onDeleteSession` prop type in `SessionListProps` (line 69):

```ts
  onDeleteSession: (sessionId: string, deleteBranch?: boolean) => void;
```

- [ ] **Step 2: Add the "Delete with branch" menu item**

In `web/src/components/SessionList/index.tsx`, after the existing Delete `DropdownMenuItem` (after line 317), add a conditional second item. Replace the entire delete menu item block (lines 308-317) with:

```tsx
                      <DropdownMenuItem
                        onClick={(e) => {
                          e.stopPropagation();
                          onDeleteSession(session.id);
                        }}
                        className="text-red-500 focus:text-red-500"
                      >
                        <Trash2 className="mr-2 h-3 w-3" />
                        Delete
                      </DropdownMenuItem>
                      {session.worktree_branch && (
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation();
                            onDeleteSession(session.id, true);
                          }}
                          className="text-red-500 focus:text-red-500"
                        >
                          <GitBranch className="mr-2 h-3 w-3" />
                          Delete with branch
                        </DropdownMenuItem>
                      )}
```

Note: `GitBranch` is already imported at line 11.

- [ ] **Step 3: Verify the web app compiles**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion/web && npx tsc --noEmit`
Expected: No errors (or only pre-existing errors)

- [ ] **Step 4: Commit**

```bash
git add web/src/components/SessionList/index.tsx
git commit -m "feat: add 'Delete with branch' context menu item for worktree sessions"
```

---

### Task 8: Final integration verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go test ./... 2>&1 | tail -30`
Expected: ALL PASS

- [ ] **Step 2: Build Go binary**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go build ./cmd/argus/`
Expected: No errors

- [ ] **Step 3: Type-check web app**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion/web && npx tsc --noEmit`
Expected: No errors (or only pre-existing errors)

- [ ] **Step 4: Verify `--delete-branch` flag appears in help**

Run: `cd /home/jeevb/.argus/projects/--home--jeevb--Workspace--repos--bxnlabs--argus/worktrees/jeev--bxn-27-branch-deletion && go run ./cmd/argus/ session rm --help`
Expected: Shows `--delete-branch` flag with description "Also delete the git branch"
