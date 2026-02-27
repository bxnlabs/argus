package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
)

// clearArgusEnv unsets all known ARGUS_* environment variables for the
// duration of the test, preventing ambient env from contaminating defaults.
func clearArgusEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ARGUS_SERVER_PORT", "ARGUS_SERVER_BIND_ADDRESS",
		"ARGUS_AGENT_PORT", "ARGUS_AGENT_BIND_ADDRESS",
		"ARGUS_DATABASE_PATH", "ARGUS_GIT_BRANCH_PREFIX",
	} {
		if v, ok := os.LookupEnv(key); ok {
			t.Cleanup(func() { os.Setenv(key, v) })
		}
		os.Unsetenv(key)
	}
}

func TestDefaults(t *testing.T) {
	clearArgusEnv(t)
	// Use auto-discovery with a fake HOME so no real config is found.
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
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

func TestExplicitMissingFileReturnsError(t *testing.T) {
	_, err := config.Load(config.Options{
		ConfigFile: "/does/not/exist.toml",
	})
	if err == nil {
		t.Fatal("expected error for explicitly specified missing config file, got nil")
	}
}

func TestAutoDiscoveryMissingFileUsesDefaults(t *testing.T) {
	clearArgusEnv(t)
	// Auto-discovery (no explicit ConfigFile) should tolerate missing files.
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error for auto-discovery missing file: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "127.0.0.1" {
		t.Errorf("Server.BindAddress = %q, want 127.0.0.1", cfg.Server.BindAddress)
	}
}

func TestEmptyFileUsesDefaults(t *testing.T) {
	clearArgusEnv(t)
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
	clearArgusEnv(t)
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

func TestValidation_EmptyDatabasePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[database]
path = ""
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected error for empty database.path, got nil")
	}
}

func TestDefaultConfigFileSearch(t *testing.T) {
	clearArgusEnv(t)
	// When no ConfigFile is specified, Load searches ~/.argus/
	// Redirect HOME to a temp dir so we don't read the developer's real config.
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error with default search: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("Server.Port = %d, want 3000", cfg.Server.Port)
	}
}

func TestIPv6BindAddress(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[server]
bind_address = "::1"

[agent]
bind_address = "::1"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error for IPv6 bind address: %v", err)
	}
	if cfg.Server.BindAddress != "::1" {
		t.Errorf("Server.BindAddress = %q, want ::1", cfg.Server.BindAddress)
	}
	if cfg.Agent.BindAddress != "::1" {
		t.Errorf("Agent.BindAddress = %q, want ::1", cfg.Agent.BindAddress)
	}
}
