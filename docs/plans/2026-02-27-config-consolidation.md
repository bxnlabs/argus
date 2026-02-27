# Config Consolidation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Consolidate all configuration into `~/.argus/config.toml` using Viper, support environment variable overrides with `ARGUS_` prefix, remove `--port`/`--db` CLI flags, and default bind addresses to `127.0.0.1`.

**Architecture:** Replace `BurntSushi/toml` with `spf13/viper` in `internal/config`. The config struct expands to include `[server]`, `[agent]`, `[database]`, and `[git]` sections. Viper handles TOML parsing, env var binding (`ARGUS_` prefix), and defaults. The CLI (`cmd/argus/main.go`) drops `--port`/`--db` flags, adds a persistent `--config` flag, and initializes Viper via `cobra.OnInitialize`. All subcommands read from the global config struct instead of local flag variables.

**Tech Stack:** `spf13/viper` with `pelletier/go-toml/v2` for TOML, existing `spf13/cobra` CLI, Go stdlib `net` for validation.

---

### Task 1: Add Viper dependency, remove BurntSushi/toml

**Files:**
- Modify: `go.mod`

**Step 1: Add Viper and remove BurntSushi/toml**

```bash
go get github.com/spf13/viper@latest
go mod tidy
```

Expected: `go.mod` gains `github.com/spf13/viper` (and transitive deps including `github.com/pelletier/go-toml/v2`). `github.com/BurntSushi/toml` remains until code references are removed in Task 2.

**Step 2: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: add spf13/viper for config management"
```

---

### Task 2: Rewrite internal/config with Viper

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing tests**

Replace `internal/config/config_test.go` with:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
)

func TestDefaults(t *testing.T) {
	cfg, err := config.Load(config.Options{
		ConfigFile: "/nonexistent/config.toml",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Server.BindAddress = %q, want 127.0.0.1", cfg.Server.BindAddress)
	}
	if cfg.Agent.Port != 3011 {
		t.Errorf("Agent.Port = %d, want 3011", cfg.Agent.Port)
	}
	if cfg.Agent.BindAddress != "127.0.0.1" {
		t.Errorf("Agent.BindAddress = %q, want 127.0.0.1", cfg.Agent.BindAddress)
	}
	if cfg.Database.Path != "~/.argus/agent.db" {
		t.Errorf("Database.Path = %q, want ~/.argus/agent.db", cfg.Database.Path)
	}
	if cfg.Git.BranchPrefix != "" {
		t.Errorf("Git.BranchPrefix = %q, want empty", cfg.Git.BranchPrefix)
	}
}

func TestLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[server]
port = 4000
bind_address = "0.0.0.0"

[agent]
port = 5000
bind_address = "192.168.1.1"

[database]
path = "/tmp/test.db"

[git]
branch_prefix = "jeev"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 4000 {
		t.Errorf("Server.Port = %d, want 4000", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "0.0.0.0" {
		t.Errorf("Server.BindAddress = %q, want 0.0.0.0", cfg.Server.BindAddress)
	}
	if cfg.Agent.Port != 5000 {
		t.Errorf("Agent.Port = %d, want 5000", cfg.Agent.Port)
	}
	if cfg.Agent.BindAddress != "192.168.1.1" {
		t.Errorf("Agent.BindAddress = %q, want 192.168.1.1", cfg.Agent.BindAddress)
	}
	if cfg.Database.Path != "/tmp/test.db" {
		t.Errorf("Database.Path = %q, want /tmp/test.db", cfg.Database.Path)
	}
	if cfg.Git.BranchPrefix != "jeev" {
		t.Errorf("Git.BranchPrefix = %q, want jeev", cfg.Git.BranchPrefix)
	}
}

func TestEnvVarOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[server]
port = 4000
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("ARGUS_SERVER_PORT", "9999")
	t.Setenv("ARGUS_GIT_BRANCH_PREFIX", "env-prefix")

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("Server.Port = %d, want 9999 (env override)", cfg.Server.Port)
	}
	if cfg.Git.BranchPrefix != "env-prefix" {
		t.Errorf("Git.BranchPrefix = %q, want env-prefix", cfg.Git.BranchPrefix)
	}
}

func TestMissingFileUsesDefaults(t *testing.T) {
	cfg, err := config.Load(config.Options{
		ConfigFile: "/does/not/exist.toml",
	})
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Server.BindAddress = %q, want 127.0.0.1", cfg.Server.BindAddress)
	}
}

func TestEmptyFileUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Server.BindAddress = %q, want 127.0.0.1", cfg.Server.BindAddress)
	}
	if cfg.Agent.BindAddress != "127.0.0.1" {
		t.Errorf("Agent.BindAddress = %q, want 127.0.0.1", cfg.Agent.BindAddress)
	}
}

func TestPartialConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[git]
branch_prefix = "jeev"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Git.BranchPrefix != "jeev" {
		t.Errorf("Git.BranchPrefix = %q, want jeev", cfg.Git.BranchPrefix)
	}
	// Unset fields should have defaults
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Server.BindAddress = %q, want 127.0.0.1", cfg.Server.BindAddress)
	}
}

func TestMalformedConfigReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("not valid toml [[[["), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected error for malformed config, got nil")
	}
}

func TestValidation_InvalidPort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[server]
port = 99999
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected error for invalid port, got nil")
	}
}

func TestValidation_InvalidBindAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[server]
bind_address = "not-an-ip"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected error for invalid bind_address, got nil")
	}
}

func TestDefaultConfigFileSearch(t *testing.T) {
	// When no ConfigFile is specified, Load searches ~/.argus/
	// We test this by NOT setting ConfigFile — it should not error
	// even if the default file doesn't exist.
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error with default search: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/ -v
```

Expected: compilation errors — `config.Options`, `config.Load(Options{})` don't exist yet.

**Step 3: Write the implementation**

Replace `internal/config/config.go` with:

```go
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all Argus configuration.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Agent    AgentConfig    `mapstructure:"agent"`
	Database DatabaseConfig `mapstructure:"database"`
	Git      GitConfig      `mapstructure:"git"`
}

type ServerConfig struct {
	Port        int    `mapstructure:"port"`
	BindAddress string `mapstructure:"bind_address"`
}

type AgentConfig struct {
	Port        int    `mapstructure:"port"`
	BindAddress string `mapstructure:"bind_address"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

type GitConfig struct {
	BranchPrefix string `mapstructure:"branch_prefix"`
}

// Options controls how config is loaded.
type Options struct {
	// ConfigFile overrides the default config file path.
	// When empty, searches ~/.argus/ for config.toml.
	ConfigFile string
}

// Load reads configuration from the TOML file and environment variables.
// Priority (highest to lowest): env vars > config file > defaults.
// A missing config file is not an error; defaults are used.
// A malformed config file is an error.
func Load(opts Options) (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("server.port", 3000)
	v.SetDefault("server.bind_address", "127.0.0.1")
	v.SetDefault("agent.port", 3011)
	v.SetDefault("agent.bind_address", "127.0.0.1")
	v.SetDefault("database.path", "~/.argus/agent.db")
	v.SetDefault("git.branch_prefix", "")

	// Environment variables: ARGUS_SERVER_PORT, etc.
	v.SetEnvPrefix("ARGUS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file
	if opts.ConfigFile != "" {
		v.SetConfigFile(opts.ConfigFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("config: determine home directory: %w", err)
		}
		v.AddConfigPath(filepath.Join(home, ".argus"))
		v.SetConfigName("config")
		v.SetConfigType("toml")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Missing file is fine — use defaults.
		} else if os.IsNotExist(err) {
			// Explicit path that doesn't exist — use defaults.
		} else {
			return nil, fmt.Errorf("config: read config file: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	return &cfg, nil
}

func validate(cfg *Config) error {
	if err := validatePort("server.port", cfg.Server.Port); err != nil {
		return err
	}
	if err := validatePort("agent.port", cfg.Agent.Port); err != nil {
		return err
	}
	if err := validateIP("server.bind_address", cfg.Server.BindAddress); err != nil {
		return err
	}
	if err := validateIP("agent.bind_address", cfg.Agent.BindAddress); err != nil {
		return err
	}
	if cfg.Database.Path == "" {
		return fmt.Errorf("database.path must not be empty")
	}
	return nil
}

func validatePort(key string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%s must be 1-65535, got %d", key, port)
	}
	return nil
}

func validateIP(key, addr string) error {
	if net.ParseIP(addr) == nil {
		return fmt.Errorf("%s must be a valid IP address, got %q", key, addr)
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/ -v
```

Expected: all tests pass.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: rewrite config package with Viper, structured sections, and validation"
```

---

### Task 3: Update worktree manager to use new Config struct

**Files:**
- Modify: `internal/worktree/manager.go`
- Modify: `internal/worktree/manager_test.go`

**Step 1: Write the failing tests**

The existing tests in `internal/worktree/manager_test.go` construct `config.Config{BranchPrefix: "jeev"}`. These will fail to compile because the field is now at `config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}}`. Update every `&config.Config{BranchPrefix: ...}` to the new struct shape.

In `internal/worktree/manager_test.go`, replace all occurrences of:
```go
&config.Config{BranchPrefix: "jeev"}
```
with:
```go
&config.Config{Git: config.GitConfig{BranchPrefix: "jeev"}}
```

And replace all occurrences of:
```go
&config.Config{}
```
with:
```go
&config.Config{}
```
(This one stays the same — zero value still works.)

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/worktree/ -v
```

Expected: compilation error — `config.Config` no longer has `BranchPrefix` at the top level.

**Step 3: Update the manager**

In `internal/worktree/manager.go`, update the `branchName` method. Change:

```go
func (m *Manager) branchName(slug string) string {
	if m.cfg.BranchPrefix != "" {
		return m.cfg.BranchPrefix + "/" + slug
	}
	return slug
}
```

to:

```go
func (m *Manager) branchName(slug string) string {
	if m.cfg.Git.BranchPrefix != "" {
		return m.cfg.Git.BranchPrefix + "/" + slug
	}
	return slug
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/worktree/ -v
```

Expected: all tests pass.

**Step 5: Commit**

```bash
git add internal/worktree/manager.go internal/worktree/manager_test.go
git commit -m "refactor: update worktree manager for new config struct layout"
```

---

### Task 4: Update agent setup to accept full Config

**Files:**
- Modify: `internal/agent/setup.go`

**Step 1: Update agent.Setup to use config.Config**

In `internal/agent/setup.go`:

1. Remove the `Config` struct (the agent-specific one).
2. Change `Setup` to accept `*config.Config` from the `internal/config` package.
3. Use `cfg.Database.Path` instead of `cfg.DBPath`.
4. Remove the `config.Load()` call inside Setup — config is now loaded once at CLI init.
5. Pass `cfg` directly to `worktree.NewManager`.

Replace the file content with:

```go
package agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bxnlabs/argus/internal/agent/api"
	"github.com/bxnlabs/argus/internal/agent/db"
	"github.com/bxnlabs/argus/internal/agent/session"
	"github.com/bxnlabs/argus/internal/agent/status"
	"github.com/bxnlabs/argus/internal/config"
	ghsvc "github.com/bxnlabs/argus/internal/github"
	"github.com/bxnlabs/argus/internal/shared"
	"github.com/bxnlabs/argus/internal/worktree"
)

// Setup initializes the agent: opens the database, runs migrations, and
// returns an HTTP handler with all agent API routes. The returned cleanup
// function closes the database and should be called on shutdown.
func Setup(cfg *config.Config) (http.Handler, func(), error) {
	database, err := db.Open(cfg.Database.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("open db: %w", err)
	}

	if err := database.RunMigrations(); err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("migrations: %w", err)
	}

	// Determine state dir from DB path (~/.argus)
	expandedDBPath, expandErr := shared.ExpandPath(cfg.Database.Path)
	if expandErr != nil {
		expandedDBPath = cfg.Database.Path // fall back to literal path
	}
	stateDir := filepath.Dir(expandedDBPath)
	if stateDir == "." {
		home, err := os.UserHomeDir()
		if err != nil {
			database.Close()
			return nil, nil, fmt.Errorf("home dir: %w", err)
		}
		stateDir = filepath.Join(home, ".argus")
	}

	wtMgr := worktree.NewManager(stateDir, cfg)

	mgr := session.NewManager(database, wtMgr)
	detector := status.NewDetector()

	repoIndexer := ghsvc.NewRepoIndexer(stateDir)
	repoIndexer.Start(context.Background())

	handler := api.NewRouter(api.Deps{
		SessionManager: mgr,
		StatusDetector: detector,
		RepoIndexer:    repoIndexer,
	})

	cleanup := func() {
		repoIndexer.Close()
		database.Close()
	}
	return handler, cleanup, nil
}
```

**Step 2: Verify compilation**

```bash
go build ./internal/agent/
```

Expected: may fail if `cmd/argus/main.go` still references `agent.Config{}`. That's fine — we fix that in Task 5.

**Step 3: Commit**

```bash
git add internal/agent/setup.go
git commit -m "refactor: agent.Setup accepts config.Config instead of agent.Config"
```

---

### Task 5: Rewrite CLI with --config flag and Viper init

**Files:**
- Modify: `cmd/argus/main.go`

**Step 1: Rewrite main.go**

Replace the entire file with:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bxnlabs/argus/cmd/argus/cli"
	"github.com/bxnlabs/argus/internal/agent"
	"github.com/bxnlabs/argus/internal/config"
	"github.com/bxnlabs/argus/internal/web"
	"github.com/spf13/cobra"
)

var cfg *config.Config

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var configFile string

	rootCmd := &cobra.Command{
		Use:   "argus",
		Short: "Argus — agent session manager",
		Long:  "Argus runs a combined web server and agent API, or individual components.",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = config.Load(config.Options{ConfigFile: configFile})
			if err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCombined()
		},
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "path to config file (default ~/.argus/config.toml)")

	rootCmd.AddCommand(
		newServerCmd(),
		newAgentCmd(),
		cli.NewSessionCmd(),
	)

	return rootCmd
}

func newServerCmd() *cobra.Command {
	var webDir string

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start only the SPA frontend server",
		RunE: func(cmd *cobra.Command, args []string) error {
			mux := http.NewServeMux()
			mux.Handle("/", web.NewSPAHandler(webDir))
			addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
			return serve(addr, mux, "argus server", nil)
		},
	}

	cmd.Flags().StringVar(&webDir, "web", "", "Override embedded SPA with local directory")

	return cmd
}

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start only the agent API",
		RunE: func(cmd *cobra.Command, args []string) error {
			agentHandler, cleanup, err := agent.Setup(cfg)
			if err != nil {
				return err
			}
			defer cleanup()

			mux := http.NewServeMux()
			mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))

			addr := fmt.Sprintf("%s:%d", cfg.Agent.BindAddress, cfg.Agent.Port)
			return serve(addr, mux, "argus agent", func(a string) {
				writeDiscovery(a)
			})
		},
	}

	return cmd
}

func writeDiscovery(addr string) {
	// Normalize wildcard bind addresses (e.g. [::]:3000, 0.0.0.0:3000)
	// to loopback so the CLI's loopback validation accepts them.
	if host, port, err := net.SplitHostPort(addr); err == nil {
		if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
			addr = "127.0.0.1:" + port
		}
	}

	dp, err := agent.DefaultDiscoveryPath()
	if err != nil {
		log.Printf("warning: cannot determine discovery path: %v", err)
		return
	}
	if err := agent.WriteDiscoveryFile(dp, addr); err != nil {
		log.Printf("warning: cannot write discovery file: %v", err)
	}
}

func removeDiscovery() {
	dp, err := agent.DefaultDiscoveryPath()
	if err != nil {
		return
	}
	agent.RemoveDiscoveryFile(dp)
}

// runCombined starts the agent and SPA on a single port.
func runCombined() error {
	agentHandler, cleanup, err := agent.Setup(cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	mux := http.NewServeMux()
	mux.Handle("/agent/", http.StripPrefix("/agent", agentHandler))
	mux.Handle("/", web.NewSPAHandler(""))

	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	return serve(addr, mux, "argus", func(a string) {
		writeDiscovery(a)
	})
}

// serve starts an HTTP server with graceful shutdown.
func serve(addr string, handler http.Handler, name string, onListening func(addr string)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	srv := &http.Server{
		Handler: handler,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(done)

	serveErr := make(chan error, 1)
	go func() {
		log.Printf("%s listening on %s", name, ln.Addr().String())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	if onListening != nil {
		onListening(ln.Addr().String())
		defer removeDiscovery()
	}

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-done:
	}
	log.Println("shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	return nil
}
```

**Step 2: Verify full build**

```bash
go build ./cmd/argus/
```

Expected: clean compilation.

**Step 3: Commit**

```bash
git add cmd/argus/main.go
git commit -m "feat: replace CLI flags with config file, add --config flag and Viper init"
```

---

### Task 6: Remove BurntSushi/toml dependency

**Files:**
- Modify: `go.mod`, `go.sum`

**Step 1: Remove the dependency**

```bash
go mod tidy
```

Expected: `github.com/BurntSushi/toml` removed from `go.mod` (no more code imports it).

**Step 2: Verify build and tests**

```bash
go build ./... && go test ./...
```

Expected: clean build, all tests pass.

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "deps: remove BurntSushi/toml, now using Viper for TOML parsing"
```

---

### Task 7: End-to-end verification

**Files:** None (verification only)

**Step 1: Run the full test suite**

```bash
go test ./... -v
```

Expected: all tests pass.

**Step 2: Test binary with no config file**

```bash
go build -o /tmp/argus ./cmd/argus/ && /tmp/argus --help
```

Expected: help output shows `--config` flag, no `--port` or `--db` flags on root command.

**Step 3: Test with a custom config file**

Create a temp config:

```bash
cat > /tmp/argus-test-config.toml << 'EOF'
[server]
port = 4444
bind_address = "127.0.0.1"

[database]
path = "/tmp/argus-test.db"

[git]
branch_prefix = "test"
EOF
```

Run:

```bash
/tmp/argus --config /tmp/argus-test-config.toml &
sleep 1
curl -s http://127.0.0.1:4444/ | head -c 100
kill %1
```

Expected: server starts on port 4444, responds to requests.

**Step 4: Test env var override**

```bash
ARGUS_SERVER_PORT=5555 /tmp/argus --config /tmp/argus-test-config.toml &
sleep 1
curl -s http://127.0.0.1:5555/ | head -c 100
kill %1
```

Expected: server starts on port 5555 (env var overrides config file).

**Step 5: Clean up**

```bash
rm /tmp/argus /tmp/argus-test-config.toml /tmp/argus-test.db 2>/dev/null
```
