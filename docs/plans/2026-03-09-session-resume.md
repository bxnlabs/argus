# Session Resume Implementation Plan [BXN-41]

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically capture provider session IDs on exit and resume conversations when sessions are recreated.

**Architecture:** Each provider defines a regex pattern for extracting its session ID from terminal output. The init script (no longer `exec`) captures the tmux pane after the agent exits and persists the session ID via `argus internal session set-provider-id`. `EnsureSession` already passes the stored ID to `--resume` — no changes needed there.

**Tech Stack:** Go (backend/CLI), bash (init script), cobra (CLI framework), SQLite (persistence)

---

### Task 1: Add `SessionIDPattern` to Provider struct

**Files:**
- Modify: `internal/agent/provider/provider.go:16-24`
- Modify: `internal/agent/provider/claude.go`
- Modify: `internal/agent/provider/codex.go`
- Modify: `internal/agent/provider/gemini.go`

**Step 1: Add the field to the Provider struct**

In `internal/agent/provider/provider.go`, add `SessionIDPattern` to the `Provider` struct:

```go
type Provider struct {
	ID               AgentType
	Name             string
	CLI              string // command name (e.g. "claude")
	AutoApproveFlag  string // flag to skip permission prompts
	SupportsResume   bool
	ResumeArg        string
	ModelFlag        string
	SessionIDPattern string // regex with one capture group for extracting session ID from terminal output
}
```

**Step 2: Add a `GetSessionIDPattern` function**

In `internal/agent/provider/provider.go`, add a public accessor:

```go
// GetSessionIDPattern returns the session ID extraction regex for a provider.
// Returns empty string for providers that don't support resume.
func GetSessionIDPattern(id AgentType) string {
	p, ok := providers[id]
	if !ok || !p.SupportsResume {
		return ""
	}
	return p.SessionIDPattern
}
```

**Step 3: Set patterns on each provider**

`internal/agent/provider/claude.go`:
```go
func init() {
	register(&Provider{
		ID:               AgentClaude,
		Name:             "Claude Code",
		CLI:              "claude",
		AutoApproveFlag:  "--dangerously-skip-permissions",
		SupportsResume:   true,
		ResumeArg:        "--resume",
		ModelFlag:        "--model",
		SessionIDPattern: `claude --resume ([0-9a-f-]+)`,
	})
}
```

`internal/agent/provider/codex.go`:
```go
func init() {
	register(&Provider{
		ID:               AgentCodex,
		Name:             "Codex",
		CLI:              "codex",
		AutoApproveFlag:  "--approval-mode full-auto",
		SupportsResume:   true,
		ResumeArg:        "resume",
		ModelFlag:        "--model",
		SessionIDPattern: `codex resume ([0-9a-f-]+)`,
	})
}
```

`internal/agent/provider/gemini.go`:
```go
func init() {
	register(&Provider{
		ID:               AgentGemini,
		Name:             "Gemini CLI",
		CLI:              "gemini",
		AutoApproveFlag:  "--yolomode",
		SupportsResume:   true,
		ResumeArg:        "--resume",
		ModelFlag:        "-m",
		SessionIDPattern: `Session ID:\s+([0-9a-f-]+)`,
	})
}
```

**Step 4: Write test for GetSessionIDPattern**

Add to `internal/agent/provider/provider_test.go`:

```go
func TestGetSessionIDPattern(t *testing.T) {
	tests := []struct {
		agent   AgentType
		wantSet bool
	}{
		{AgentClaude, true},
		{AgentCodex, true},
		{AgentGemini, true},
		{AgentShell, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.agent), func(t *testing.T) {
			pat := GetSessionIDPattern(tt.agent)
			if tt.wantSet && pat == "" {
				t.Errorf("expected pattern for %s", tt.agent)
			}
			if !tt.wantSet && pat != "" {
				t.Errorf("unexpected pattern for %s: %s", tt.agent, pat)
			}
		})
	}
}
```

**Step 5: Run tests**

Run: `go test ./internal/agent/provider/ -v -run TestGetSessionIDPattern`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/agent/provider/
git commit -m "feat: add SessionIDPattern to provider struct for session resume"
```

---

### Task 2: Extend API PATCH handler to accept `provider_session_id`

**Files:**
- Modify: `internal/agent/api/sessions.go:88-121`

**Step 1: Update the PATCH handler body struct and logic**

Replace the `update` method in `internal/agent/api/sessions.go`:

```go
// PATCH /api/sessions/{id}
func (h *sessionHandler) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var body struct {
		Name              *string `json:"name"`
		ProviderSessionID *string `json:"provider_session_id"`
	}
	if err := parseBody(w, r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	update := db.SessionUpdate{
		Name:              body.Name,
		ProviderSessionID: body.ProviderSessionID,
	}
	if err := h.manager.Update(id, update); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(w, http.StatusNotFound, "session not found")
			return
		}
		respondInternalError(w, err)
		return
	}

	session, err := h.manager.Get(id)
	if err != nil {
		respondInternalError(w, err)
		return
	}
	if session == nil {
		respondError(w, http.StatusNotFound, "session not found")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"session": session})
}
```

Note: This replaces the old `Rename`-only path with a single `Update` call that handles both `name` and `provider_session_id` via the existing `db.SessionUpdate` struct. The `Rename` method on the manager is now bypassed here (it just called `Update` internally anyway).

**Step 2: Run existing tests to ensure nothing breaks**

Run: `go test ./internal/agent/... -v`
Expected: PASS (no existing tests for the PATCH handler specifically, but verify no compile errors)

**Step 3: Commit**

```bash
git add internal/agent/api/sessions.go
git commit -m "feat: extend PATCH /api/sessions to accept provider_session_id"
```

---

### Task 3: Create `argus internal session set-provider-id` CLI command

**Files:**
- Create: `cmd/argus/cli/internal_cmd.go`
- Create: `cmd/argus/cli/internal_session_set_provider_id.go`
- Modify: `cmd/argus/main.go:52-56`

**Step 1: Create the `argus internal` parent command**

Create `cmd/argus/cli/internal_cmd.go`:

```go
package cli

import "github.com/spf13/cobra"

// NewInternalCmd returns the "internal" parent command for non-user-facing operations.
func NewInternalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "internal",
		Short:  "Internal commands (not user-facing)",
		Hidden: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}

	sessionCmd := &cobra.Command{
		Use:   "session",
		Short: "Internal session operations",
	}
	sessionCmd.AddCommand(
		newSetProviderIDCmd(),
	)

	cmd.AddCommand(sessionCmd)

	return cmd
}
```

**Step 2: Create the `set-provider-id` command**

Create `cmd/argus/cli/internal_session_set_provider_id.go`:

```go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func newSetProviderIDCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-provider-id <session-id> <provider-session-id>",
		Short: "Persist a provider session ID for resume support",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			sessionID := args[0]
			providerSessionID := args[1]

			path, err := discoveryFilePath()
			if err != nil {
				return err
			}
			c, err := newClient(path)
			if err != nil {
				return err
			}

			reqBody, err := json.Marshal(map[string]string{
				"provider_session_id": providerSessionID,
			})
			if err != nil {
				return fmt.Errorf("marshal request: %w", err)
			}
			if _, err := c.patch("/api/sessions/"+sessionID, bytes.NewReader(reqBody)); err != nil {
				return err
			}

			return nil
		},
	}
}
```

**Step 3: Register in main.go**

In `cmd/argus/main.go`, add `cli.NewInternalCmd()` to the `rootCmd.AddCommand` block:

```go
rootCmd.AddCommand(
    newServerCmd(),
    newAgentCmd(),
    cli.NewSessionCmd(),
    cli.NewInternalCmd(),
)
```

**Step 4: Verify it compiles**

Run: `go build ./cmd/argus/`
Expected: Compiles without errors

**Step 5: Commit**

```bash
git add cmd/argus/cli/internal_cmd.go cmd/argus/cli/internal_session_set_provider_id.go cmd/argus/main.go
git commit -m "feat: add argus internal session set-provider-id CLI command"
```

---

### Task 4: Modify init script to capture provider session ID on exit

**Files:**
- Modify: `internal/agent/session/initscript.go`
- Modify: `internal/agent/session/lifecycle.go:104-122` (Create call site)
- Modify: `internal/agent/session/lifecycle.go:382-398` (EnsureSession call site)

**Step 1: Update `GenerateInitScript` signature and add post-exit capture**

Replace `internal/agent/session/initscript.go` entirely:

```go
package session

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateInitScript returns a bash init script that shows the Argus banner,
// runs the agent command, and captures the provider session ID on exit.
func GenerateInitScript(sessionID, agentCommand, sessionIDPattern string) string {
	script := "#!/bin/bash\n" +
		"# Argus Session Init Script\n" +
		"# Auto-generated - do not edit manually\n" +
		"\n" +
		"# Self-cleanup: remove this temp script\n" +
		"rm -f -- \"$0\"\n" +
		"\n" +
		"# ANSI Colors (purple theme)\n" +
		"C_RESET=$'\\033[0m'\n" +
		"C_PURPLE=$'\\033[38;5;141m'\n" +
		"C_PURPLE2=$'\\033[38;5;177m'\n" +
		"C_PINK=$'\\033[38;5;213m'\n" +
		"\n" +
		"# Clear and show banner\n" +
		"clear\n" +
		"\n" +
		"printf \"\\n\"\n" +
		"printf \"${C_PURPLE}       _${C_RESET}\\n\"\n" +
		"printf \"${C_PURPLE}      / \\\\   _ __ __ _ _   _ ___${C_RESET}\\n\"\n" +
		"printf \"${C_PURPLE2}     / _ \\\\ | '__/ _\\` | | | / __|${C_RESET}\\n\"\n" +
		"printf \"${C_PURPLE2}    / ___ \\\\| | | (_| | |_| \\\\__ \\\\${C_RESET}\\n\"\n" +
		"printf \"${C_PINK}   /_/   \\\\_\\\\_|  \\\\__, |\\\\__,_|___/${C_RESET}\\n\"\n" +
		"printf \"${C_PINK}                |___/${C_RESET}\\n\"\n" +
		"printf \"\\n\"\n" +
		"\n" +
		"# Brief pause to show banner\n" +
		"sleep 0.8\n" +
		"\n" +
		"# Ensure ~/.local/bin is in PATH (where claude is installed)\n" +
		"export PATH=\"$HOME/.local/bin:$PATH\"\n" +
		"\n" +
		"# Start the agent (no exec — script continues after exit)\n" +
		agentCommand + "\n"

	// If the provider supports session ID capture, add post-exit logic
	if sessionIDPattern != "" {
		script += "\n" +
			"# Capture provider session ID from terminal output\n" +
			"PANE_CONTENT=$(tmux capture-pane -p -S -100 2>/dev/null)\n" +
			"PROVIDER_ID=$(echo \"$PANE_CONTENT\" | grep -oP '" + sessionIDPattern + "' | tail -1)\n" +
			"\n" +
			"if [ -n \"$PROVIDER_ID\" ]; then\n" +
			"  argus internal session set-provider-id '" + sessionID + "' \"$PROVIDER_ID\" 2>/dev/null\n" +
			"fi\n"
	}

	return script
}

// WriteInitScript writes the init script to a temp file and returns the path.
// The sessionID is used to make the filename unique across concurrent calls.
func WriteInitScript(sessionID, agentCommand, sessionIDPattern string) (string, error) {
	content := GenerateInitScript(sessionID, agentCommand, sessionIDPattern)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("argus-init-%s.sh", sessionID))
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write init script: %w", err)
	}
	return path, nil
}
```

**Important note on `grep -oP`**: The `-P` flag enables Perl-compatible regex which supports `\s+` (used by Gemini's pattern). If the target system doesn't have GNU grep, fall back to `grep -oE` with an adjusted Gemini pattern using `[[:space:]]+` instead of `\s+`. On macOS, `grep -oP` requires GNU grep (`ggrep`). Since Argus sessions run in tmux where `$PATH` may vary, consider using `grep -oE` universally and adjusting patterns accordingly:

- Claude: `claude --resume ([0-9a-f-]+)` (works with `-oE`)
- Codex: `codex resume ([0-9a-f-]+)` (works with `-oE`)
- Gemini: `Session ID:[[:space:]]+([0-9a-f-]+)` (use POSIX class instead of `\s+`)

**However**, `grep -oE` with capture groups doesn't extract the capture group — it returns the full match. Instead, use `grep -oE` to match the full line, then pipe through `sed` to extract the ID. A cleaner approach: use `sed -nE` directly:

```bash
PROVIDER_ID=$(echo "$PANE_CONTENT" | sed -nE 's/.*<pattern>.*/\1/p' | tail -1)
```

Update the script generation accordingly — replace the grep line with:

```go
"PROVIDER_ID=$(echo \"$PANE_CONTENT\" | sed -nE 's/.*" + sessionIDPattern + ".*/\\1/p' | tail -1)\n" +
```

This works on both macOS and Linux `sed`.

**Step 2: Update call sites in lifecycle.go**

In `internal/agent/session/lifecycle.go`, update the `Create` method (around line 117):

```go
	// For shell sessions (no command), start tmux with default shell
	// For agent sessions, wrap with init script
	var tmuxCmd string
	if agentCmd != "" {
		pattern := provider.GetSessionIDPattern(provider.AgentType(opts.AgentType))
		scriptPath, err := WriteInitScript(sessionID, agentCmd, pattern)
		if err != nil {
			return nil, fmt.Errorf("write init script: %w", err)
		}
		tmuxCmd = "bash " + scriptPath
	}
```

And in `EnsureSession` (around line 392):

```go
	var tmuxCmd string
	if agentCmd != "" {
		pattern := provider.GetSessionIDPattern(provider.AgentType(session.AgentType))
		scriptPath, err := WriteInitScript(session.ID, agentCmd, pattern)
		if err != nil {
			return "", fmt.Errorf("write init script: %w", err)
		}
		tmuxCmd = "bash " + scriptPath
	}
```

**Step 3: Write test for init script generation**

Add to or create `internal/agent/session/initscript_test.go`:

```go
package session

import (
	"strings"
	"testing"
)

func TestGenerateInitScript_WithPattern(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", `claude --resume ([0-9a-f-]+)`)

	// Should NOT contain exec
	if strings.Contains(script, "exec ") {
		t.Error("script should not use exec when pattern is set")
	}

	// Should contain the agent command
	if !strings.Contains(script, "claude --dangerously-skip-permissions") {
		t.Error("script should contain agent command")
	}

	// Should contain capture logic
	if !strings.Contains(script, "tmux capture-pane") {
		t.Error("script should contain tmux capture-pane")
	}
	if !strings.Contains(script, "argus internal session set-provider-id") {
		t.Error("script should contain argus CLI call")
	}
	if !strings.Contains(script, "sess_abc123") {
		t.Error("script should contain session ID")
	}
}

func TestGenerateInitScript_WithoutPattern(t *testing.T) {
	script := GenerateInitScript("sess_abc123", "claude --dangerously-skip-permissions", "")

	// Should contain the agent command without exec
	if !strings.Contains(script, "claude --dangerously-skip-permissions") {
		t.Error("script should contain agent command")
	}

	// Should NOT contain capture logic
	if strings.Contains(script, "tmux capture-pane") {
		t.Error("script should not contain capture logic without pattern")
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/agent/session/ -v -run TestGenerateInitScript`
Expected: PASS

**Step 5: Verify full build**

Run: `go build ./cmd/argus/`
Expected: Compiles without errors

**Step 6: Commit**

```bash
git add internal/agent/session/initscript.go internal/agent/session/initscript_test.go internal/agent/session/lifecycle.go
git commit -m "feat: capture provider session ID on exit for session resume [BXN-41]"
```

---

### Task 5: End-to-end verification

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Manual smoke test**

1. Build and run Argus: `go run ./cmd/argus/`
2. Create a Claude session (via UI or CLI)
3. Inside the session, press ctrl-c to exit Claude
4. Check the logs or DB to confirm `provider_session_id` was captured
5. Re-open the session — it should launch with `--resume <captured-id>`

**Step 3: Verify the CLI command works standalone**

Run: `go run ./cmd/argus/ internal session set-provider-id --help`
Expected: Shows usage: `set-provider-id <session-id> <provider-session-id>`

**Step 4: Commit any fixes from smoke testing**

```bash
git add -A
git commit -m "fix: address issues found during session resume smoke test"
```
