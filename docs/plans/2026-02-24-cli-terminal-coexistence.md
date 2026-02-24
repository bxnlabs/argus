# CLI & Terminal Co-existence Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add `argus session {list,create,attach,delete,rename}` CLI subcommands so developers can manage Argus sessions from the terminal, attaching directly to tmux, while web/mobile clients continue using the WebSocket bridge.

**Architecture:** Thin CLI layer in `cmd/argus/cli/` that reads a discovery file (`~/.argus/agent.json`) to locate the running agent, makes HTTP calls to the existing API for CRUD, and `exec`s into tmux for `attach`. The agent writes the discovery file on startup and removes it on shutdown.

**Tech Stack:** Go standard library (`flag`, `net/http`, `encoding/json`, `os/exec`, `syscall`, `text/tabwriter`). No new dependencies.

**Design doc:** `docs/plans/2026-02-24-cli-terminal-coexistence-design.md`

---

### Task 1: Agent Discovery File — Write on Startup

The agent writes `~/.argus/agent.json` when it starts listening, and removes it on shutdown.

**Files:**
- Create: `internal/agent/discovery.go`
- Create: `internal/agent/discovery_test.go`
- Modify: `internal/agent/setup.go`
- Modify: `cmd/argus/main.go`

**Step 1: Write the failing test**

Create `internal/agent/discovery_test.go`:

```go
package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDiscoveryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")

	if err := WriteDiscoveryFile(path, "127.0.0.1:3000"); err != nil {
		t.Fatalf("WriteDiscoveryFile: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var info DiscoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if info.Address != "127.0.0.1:3000" {
		t.Errorf("address = %q, want %q", info.Address, "127.0.0.1:3000")
	}
	if info.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", info.PID, os.Getpid())
	}
}

func TestRemoveDiscoveryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.json")

	if err := WriteDiscoveryFile(path, "127.0.0.1:3000"); err != nil {
		t.Fatalf("WriteDiscoveryFile: %v", err)
	}

	RemoveDiscoveryFile(path)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after RemoveDiscoveryFile")
	}
}

func TestRemoveDiscoveryFile_Missing(t *testing.T) {
	// Should not panic on missing file.
	RemoveDiscoveryFile("/tmp/nonexistent-argus-test-file.json")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestWriteDiscoveryFile -v`
Expected: FAIL — `WriteDiscoveryFile` undefined.

**Step 3: Write minimal implementation**

Create `internal/agent/discovery.go`:

```go
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DiscoveryInfo is the content of ~/.argus/agent.json.
type DiscoveryInfo struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
}

// DefaultDiscoveryPath returns ~/.argus/agent.json.
func DefaultDiscoveryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".argus", "agent.json"), nil
}

// WriteDiscoveryFile writes the agent discovery file.
func WriteDiscoveryFile(path, address string) error {
	info := DiscoveryInfo{
		PID:     os.Getpid(),
		Address: address,
	}
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal discovery: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir discovery: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write discovery: %w", err)
	}
	return nil
}

// RemoveDiscoveryFile removes the discovery file. Errors are silently ignored
// (the file may have already been cleaned up).
func RemoveDiscoveryFile(path string) {
	os.Remove(path)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/ -run TestDiscovery -v`
Expected: PASS (all three tests).

**Step 5: Wire discovery into agent startup**

Modify `internal/agent/setup.go` — add `Address string` to `Config`:

```go
type Config struct {
	DBPath  string
	Address string // listen address (host:port) for discovery file
}
```

Update `Setup` to accept and store the address. Do **not** write the discovery file inside `Setup` — the caller (`main.go`) writes it after the server begins listening, since we need the resolved port.

Modify `cmd/argus/main.go`:

In both `runCombined` and `runAgent`, after `srv.ListenAndServe` starts but before waiting on signals, write the discovery file. Add removal to shutdown. The `serve` function needs to accept a callback that runs once the server is listening. Refactor `serve` to:

```go
func serve(addr string, handler http.Handler, name string, onListening func(addr string)) error {
```

Inside `serve`, after `ListenAndServe` is called and no immediate error, invoke `onListening(addr)`. Both `runCombined` and `runAgent` pass a callback that writes the discovery file:

```go
return serve(fmt.Sprintf(":%d", *port), mux, "argus", func(addr string) {
    dp, err := DefaultDiscoveryPath()
    if err != nil {
        log.Printf("warning: cannot determine discovery path: %v", err)
        return
    }
    if err := WriteDiscoveryFile(dp, addr); err != nil {
        log.Printf("warning: cannot write discovery file: %v", err)
    }
})
```

Add cleanup at shutdown (inside `serve`, after `Shutdown` returns):

```go
dp, _ := DefaultDiscoveryPath()
RemoveDiscoveryFile(dp)
```

`runServer` passes `nil` for the callback since the SPA-only server does not need discovery.

Update `serve` signature and body to handle the `onListening` callback being nil.

**Step 6: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles without errors.

**Step 7: Commit**

```bash
git add internal/agent/discovery.go internal/agent/discovery_test.go internal/agent/setup.go cmd/argus/main.go
git commit -m "feat: write agent discovery file on startup"
```

---

### Task 2: CLI Discovery Client — Read and Validate

The CLI reads the discovery file, checks PID liveness, handles stale files.

**Files:**
- Create: `cmd/argus/cli/client.go`
- Create: `cmd/argus/cli/client_test.go`

**Step 1: Write the failing test**

Create `cmd/argus/cli/client_test.go`:

```go
package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeTestDiscovery(t *testing.T, dir string, pid int, address string) string {
	t.Helper()
	path := filepath.Join(dir, "agent.json")
	data, _ := json.Marshal(map[string]any{"pid": pid, "address": address})
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscover_Valid(t *testing.T) {
	dir := t.TempDir()
	// Use current process PID so the liveness check passes.
	path := writeTestDiscovery(t, dir, os.Getpid(), "127.0.0.1:3000")

	info, err := discover(path)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if info.Address != "127.0.0.1:3000" {
		t.Errorf("address = %q, want %q", info.Address, "127.0.0.1:3000")
	}
}

func TestDiscover_MissingFile(t *testing.T) {
	_, err := discover("/tmp/nonexistent-argus-test.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestDiscover_StalePID(t *testing.T) {
	dir := t.TempDir()
	// PID 2147483647 should not be alive.
	path := writeTestDiscovery(t, dir, 2147483647, "127.0.0.1:3000")

	_, err := discover(path)
	if err == nil {
		t.Fatal("expected error for stale PID")
	}

	// File should be cleaned up.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("stale discovery file was not removed")
	}
}

func TestAPIClient_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/api/sessions" {
			t.Errorf("path = %q, want /agent/api/sessions", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sessions":[]}`))
	}))
	defer srv.Close()

	c := &apiClient{baseURL: srv.URL + "/agent"}
	body, err := c.get("/api/sessions")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(body) != `{"sessions":[]}` {
		t.Errorf("body = %q", string(body))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/argus/cli/ -run TestDiscover -v`
Expected: FAIL — package does not exist.

**Step 3: Write minimal implementation**

Create `cmd/argus/cli/client.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"
)

// discoveryInfo mirrors agent.DiscoveryInfo without importing internal/.
type discoveryInfo struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
}

// discover reads the discovery file and validates PID liveness.
func discover(path string) (*discoveryInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Argus agent is not running.\nStart it with: argus --port 3000")
		}
		return nil, fmt.Errorf("read discovery file: %w", err)
	}

	var info discoveryInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parse discovery file: %w", err)
	}

	// Check if the PID is still alive.
	proc, err := os.FindProcess(info.PID)
	if err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("Argus agent is not running (stale state detected, cleaning up).\nStart it with: argus --port 3000")
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		os.Remove(path)
		return nil, fmt.Errorf("Argus agent is not running (stale state detected, cleaning up).\nStart it with: argus --port 3000")
	}

	return &info, nil
}

// apiClient makes HTTP requests to the Argus agent API.
type apiClient struct {
	baseURL string // e.g. "http://127.0.0.1:3000/agent"
	http    http.Client
}

// newClient reads the discovery file and returns an API client.
func newClient(discoveryPath string) (*apiClient, error) {
	info, err := discover(discoveryPath)
	if err != nil {
		return nil, err
	}

	c := &apiClient{
		baseURL: "http://" + info.Address + "/agent",
		http: http.Client{
			Timeout: 10 * time.Second,
		},
	}
	return c, nil
}

func (c *apiClient) get(path string) ([]byte, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *apiClient) post(path string, body io.Reader) ([]byte, int, error) {
	resp, err := c.http.Post(c.baseURL+path, "application/json", body)
	if err != nil {
		return nil, 0, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *apiClient) patch(path string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, body)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func (c *apiClient) delete(path string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Cannot reach Argus agent at %s.\nCheck if the agent is running.", c.baseURL)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/argus/cli/ -run "TestDiscover|TestAPIClient" -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/argus/cli/client.go cmd/argus/cli/client_test.go
git commit -m "feat: add CLI discovery client with PID validation"
```

---

### Task 3: Session Resolution — Name/ID Matching

Resolve a user-provided string to a session by matching against name (exact) then ID (prefix).

**Files:**
- Create: `cmd/argus/cli/resolve.go`
- Create: `cmd/argus/cli/resolve_test.go`

**Step 1: Write the failing test**

Create `cmd/argus/cli/resolve_test.go`:

```go
package cli

import "testing"

func TestResolveSession_ExactName(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc", Name: "my-session", TmuxName: "claude-sess_abc"},
		{ID: "sess_def", Name: "other", TmuxName: "claude-sess_def"},
	}
	s, err := resolveSession(sessions, "my-session")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if s.ID != "sess_abc" {
		t.Errorf("id = %q, want %q", s.ID, "sess_abc")
	}
}

func TestResolveSession_IDPrefix(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc123_xyz", Name: "my-session", TmuxName: "claude-sess_abc123_xyz"},
		{ID: "sess_def456_uvw", Name: "other", TmuxName: "claude-sess_def456_uvw"},
	}
	s, err := resolveSession(sessions, "sess_abc")
	if err != nil {
		t.Fatalf("resolveSession: %v", err)
	}
	if s.ID != "sess_abc123_xyz" {
		t.Errorf("id = %q, want %q", s.ID, "sess_abc123_xyz")
	}
}

func TestResolveSession_Ambiguous(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc1", Name: "session-a"},
		{ID: "sess_abc2", Name: "session-b"},
	}
	_, err := resolveSession(sessions, "sess_abc")
	if err == nil {
		t.Fatal("expected error for ambiguous match")
	}
}

func TestResolveSession_NoMatch(t *testing.T) {
	sessions := []sessionInfo{
		{ID: "sess_abc", Name: "my-session"},
	}
	_, err := resolveSession(sessions, "nonexistent")
	if err == nil {
		t.Fatal("expected error for no match")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/argus/cli/ -run TestResolveSession -v`
Expected: FAIL — `resolveSession` and `sessionInfo` undefined.

**Step 3: Write minimal implementation**

Create `cmd/argus/cli/resolve.go`:

```go
package cli

import (
	"fmt"
	"strings"
)

// sessionInfo is a lightweight mirror of the API session response.
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
}

// resolveSession finds a session by exact name match or ID prefix.
func resolveSession(sessions []sessionInfo, query string) (*sessionInfo, error) {
	// 1. Exact name match.
	for i := range sessions {
		if sessions[i].Name == query {
			return &sessions[i], nil
		}
	}

	// 2. ID prefix match.
	var matches []*sessionInfo
	for i := range sessions {
		if strings.HasPrefix(sessions[i].ID, query) {
			matches = append(matches, &sessions[i])
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no session found matching %q", query)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, fmt.Sprintf("  %s (%s)", m.Name, m.ID))
		}
		return nil, fmt.Errorf("ambiguous match %q — multiple sessions match:\n%s", query, strings.Join(names, "\n"))
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/argus/cli/ -run TestResolveSession -v`
Expected: PASS (all four tests).

**Step 5: Commit**

```bash
git add cmd/argus/cli/resolve.go cmd/argus/cli/resolve_test.go
git commit -m "feat: add session resolution by name or ID prefix"
```

---

### Task 4: CLI Entry Point and Dispatch

Wire the `session` subcommand into `main.go` and create the dispatcher.

**Files:**
- Create: `cmd/argus/cli/cli.go`
- Modify: `cmd/argus/main.go`

**Step 1: Write the entry point**

Create `cmd/argus/cli/cli.go`:

```go
package cli

import (
	"fmt"
	"os"
)

// Run is the entry point for `argus session <subcommand>`.
func Run(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "list":
		return runList(args[1:])
	case "create":
		return runCreate(args[1:])
	case "attach":
		return runAttach(args[1:])
	case "delete":
		return runDelete(args[1:])
	case "rename":
		return runRename(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage())
	}
}

func usage() string {
	return `Usage: argus session <command>

Commands:
  list      List all sessions
  create    Create a new session
  attach    Attach to a session's tmux
  delete    Delete a session
  rename    Rename a session`
}

func usageError() error {
	return fmt.Errorf("%s", usage())
}

// die prints an error to stderr and exits.
func die(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

// discoveryFilePath returns the path to the agent discovery file.
// Exits with an error if the home directory cannot be determined.
func discoveryFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		die(fmt.Errorf("cannot determine home directory: %w", err))
	}
	return home + "/.argus/agent.json"
}
```

Create placeholder stubs so the package compiles (these will be filled in subsequent tasks):

Create `cmd/argus/cli/session_list.go`:
```go
package cli

func runList(args []string) error {
	return fmt.Errorf("not implemented")
}
```

Create `cmd/argus/cli/session_create.go`:
```go
package cli

func runCreate(args []string) error {
	return fmt.Errorf("not implemented")
}
```

Create `cmd/argus/cli/session_attach.go`:
```go
package cli

func runAttach(args []string) error {
	return fmt.Errorf("not implemented")
}
```

Create `cmd/argus/cli/session_delete.go`:
```go
package cli

func runDelete(args []string) error {
	return fmt.Errorf("not implemented")
}
```

Create `cmd/argus/cli/session_rename.go`:
```go
package cli

func runRename(args []string) error {
	return fmt.Errorf("not implemented")
}
```

Each stub file needs `import "fmt"` at the top.

**Step 2: Wire into main.go**

Modify `cmd/argus/main.go` — add `"session"` case in the existing switch:

```go
import (
	// ... existing imports ...
	"github.com/bxnlabs/argus/cmd/argus/cli"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "server":
			// ... existing ...
		case "agent":
			// ... existing ...
		case "session":
			if err := cli.Run(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}
	// ... rest unchanged ...
}
```

Also update the unknown-command error message in `runCombined` to include `session`:

```go
fmt.Fprintf(os.Stderr, "argus: unknown command %q\n\nUsage: argus [server|agent|session] [flags]\n", args[0])
```

**Step 3: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles. Running `./bin/argus session` prints usage. Running `./bin/argus session list` prints "not implemented".

**Step 4: Commit**

```bash
git add cmd/argus/cli/ cmd/argus/main.go
git commit -m "feat: add CLI entry point with session subcommand dispatch"
```

---

### Task 5: `session list` Command

**Files:**
- Modify: `cmd/argus/cli/session_list.go`

**Step 1: Implement the list command**

Replace the stub in `cmd/argus/cli/session_list.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"
)

func runList(args []string) error {
	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	body, err := c.get("/api/sessions")
	if err != nil {
		return err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if len(resp.Sessions) == 0 {
		fmt.Println("No sessions.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPROVIDER\tID\tUPDATED")
	for _, s := range resp.Sessions {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.AgentType, s.ID, relativeTime(s.UpdatedAt))
	}
	w.Flush()
	return nil
}

// relativeTime converts a datetime string to a human-readable relative time.
func relativeTime(ts string) string {
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, ts)
		if err == nil {
			break
		}
	}
	if err != nil {
		return ts
	}

	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}
```

**Step 2: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles.

**Step 3: Commit**

```bash
git add cmd/argus/cli/session_list.go
git commit -m "feat: implement session list command"
```

---

### Task 6: `session create` Command

**Files:**
- Modify: `cmd/argus/cli/session_create.go`

**Step 1: Implement the create command**

Replace the stub in `cmd/argus/cli/session_create.go`:

```go
package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func runCreate(args []string) error {
	fs := flag.NewFlagSet("argus session create", flag.ExitOnError)
	name := fs.String("name", "", "Session name (required)")
	provider := fs.String("provider", "claude", "Agent type (claude, codex, gemini, shell)")
	model := fs.String("model", "", "Model override")
	dir := fs.String("dir", ".", "Working directory")
	autoApprove := fs.Bool("auto-approve", false, "Enable auto-approve")
	prompt := fs.String("prompt", "", "Initial prompt to send after creation")
	fs.Parse(args)

	if *name == "" {
		return fmt.Errorf("--name is required\n\nUsage: argus session create --name <name> [flags]")
	}

	// Resolve working directory to absolute path.
	wd, err := filepath.Abs(*dir)
	if err != nil {
		return fmt.Errorf("resolve directory: %w", err)
	}

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	reqBody := map[string]any{
		"name":              *name,
		"agent_type":        *provider,
		"working_directory": wd,
		"auto_approve":      *autoApprove,
	}
	if *model != "" {
		reqBody["model"] = *model
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	body, status, err := c.post("/api/sessions", bytes.NewReader(data))
	if err != nil {
		return err
	}

	if status >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &errResp)
		if errResp.Error != "" {
			return fmt.Errorf("create failed: %s", errResp.Error)
		}
		return fmt.Errorf("create failed (HTTP %d)", status)
	}

	var resp struct {
		Session sessionInfo `json:"session"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	s := resp.Session
	fmt.Fprintf(os.Stdout, "Created session %q (%s)\n  ID:  %s\n  Dir: %s\n", s.Name, s.AgentType, s.ID, s.WorkingDirectory)

	// If --prompt was provided, print a note (sending keys requires tmux attach or API).
	if *prompt != "" {
		fmt.Fprintf(os.Stdout, "\nTo send the initial prompt, attach to the session:\n  argus session attach %s\n", s.Name)
	}

	return nil
}
```

**Step 2: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles.

**Step 3: Commit**

```bash
git add cmd/argus/cli/session_create.go
git commit -m "feat: implement session create command"
```

---

### Task 7: `session attach` Command

**Files:**
- Modify: `cmd/argus/cli/session_attach.go`

**Step 1: Implement the attach command**

Replace the stub in `cmd/argus/cli/session_attach.go`:

```go
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func runAttach(args []string) error {
	fs := flag.NewFlagSet("argus session attach", flag.ExitOnError)
	cc := fs.Bool("cc", false, "Use tmux control mode (-CC)")
	fs.Parse(args)

	if fs.NArg() == 0 {
		return fmt.Errorf("session name or ID required\n\nUsage: argus session attach [--cc] <name-or-id>")
	}
	query := fs.Arg(0)

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	// Fetch all sessions to resolve the query.
	body, err := c.get("/api/sessions")
	if err != nil {
		return err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	session, err := resolveSession(resp.Sessions, query)
	if err != nil {
		return err
	}

	// Call EnsureSession via the GET /api/sessions/{id} endpoint
	// so the agent revives the tmux session if it died.
	_, err = c.get("/api/sessions/" + session.ID)
	if err != nil {
		return fmt.Errorf("ensure session: %w", err)
	}

	// Find tmux binary.
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}

	// Build tmux args.
	var tmuxArgs []string
	if *cc {
		tmuxArgs = []string{"tmux", "-CC", "attach-session", "-t", session.TmuxName}
	} else {
		tmuxArgs = []string{"tmux", "attach-session", "-t", session.TmuxName}
	}

	// Replace the current process with tmux.
	return syscall.Exec(tmux, tmuxArgs, os.Environ())
}
```

**Step 2: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles.

**Step 3: Commit**

```bash
git add cmd/argus/cli/session_attach.go
git commit -m "feat: implement session attach command with --cc flag"
```

---

### Task 8: `session delete` Command

**Files:**
- Modify: `cmd/argus/cli/session_delete.go`

**Step 1: Implement the delete command**

Replace the stub in `cmd/argus/cli/session_delete.go`:

```go
package cli

import (
	"encoding/json"
	"fmt"
)

func runDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("session name or ID required\n\nUsage: argus session delete <name-or-id>")
	}
	query := args[0]

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	// Fetch all sessions to resolve the query.
	body, err := c.get("/api/sessions")
	if err != nil {
		return err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	session, err := resolveSession(resp.Sessions, query)
	if err != nil {
		return err
	}

	_, status, err := c.delete("/api/sessions/" + session.ID)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("delete failed (HTTP %d)", status)
	}

	fmt.Printf("Deleted session %q\n", session.Name)
	return nil
}
```

**Step 2: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles.

**Step 3: Commit**

```bash
git add cmd/argus/cli/session_delete.go
git commit -m "feat: implement session delete command"
```

---

### Task 9: `session rename` Command

**Files:**
- Modify: `cmd/argus/cli/session_rename.go`

**Step 1: Implement the rename command**

Replace the stub in `cmd/argus/cli/session_rename.go`:

```go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func runRename(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("session name/ID and new name required\n\nUsage: argus session rename <name-or-id> <new-name>")
	}
	query := args[0]
	newName := args[1]

	c, err := newClient(discoveryFilePath())
	if err != nil {
		return err
	}

	// Fetch all sessions to resolve the query.
	body, err := c.get("/api/sessions")
	if err != nil {
		return err
	}

	var resp struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	session, err := resolveSession(resp.Sessions, query)
	if err != nil {
		return err
	}

	reqBody, _ := json.Marshal(map[string]string{"name": newName})
	_, status, err := c.patch("/api/sessions/"+session.ID, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("rename failed (HTTP %d)", status)
	}

	fmt.Printf("Renamed session %q → %q\n", session.Name, newName)
	return nil
}
```

**Step 2: Verify build**

Run: `go build ./cmd/argus`
Expected: Compiles.

**Step 3: Commit**

```bash
git add cmd/argus/cli/session_rename.go
git commit -m "feat: implement session rename command"
```

---

### Task 10: Integration Smoke Test

Verify the full flow manually and fix any issues.

**Step 1: Build the binary**

Run: `go build -o bin/argus ./cmd/argus`

**Step 2: Verify CLI help**

Run: `./bin/argus session`
Expected: Prints usage with list, create, attach, delete, rename.

**Step 3: Verify error when agent not running**

Run: `./bin/argus session list`
Expected: `Error: Argus agent is not running.` (if agent is not running)

**Step 4: Run all tests**

Run: `go test ./cmd/argus/cli/ ./internal/agent/ -v`
Expected: All tests pass.

**Step 5: Commit any fixes**

```bash
git add -A
git commit -m "fix: address integration issues from smoke test"
```

(Skip this commit if no fixes were needed.)

---

### Task 11: Final Cleanup

**Step 1: Run full test suite**

Run: `go test ./...`
Expected: All tests pass with no regressions.

**Step 2: Verify build**

Run: `go build ./cmd/argus`
Expected: Clean compile, no warnings.

**Step 3: Final commit if needed**

```bash
git add -A
git commit -m "chore: clean up CLI implementation"
```
