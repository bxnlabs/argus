package config_test

import (
	"fmt"
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
		"ARGUS_NODE_PORT", "ARGUS_NODE_BIND_ADDRESS",
		"ARGUS_DATABASE_PATH", "ARGUS_GIT_BRANCH_PREFIX",
		"ARGUS_HOME",
		"ARGUS_TAILSCALE_ENABLED",
		"ARGUS_TAILSCALE_HOSTNAME_PREFIX",
		"ARGUS_TAILSCALE_AUTH_KEY",
		"ARGUS_TAILSCALE_PORT",
		"ARGUS_NOTIFICATIONS_CHANNEL",
		"ARGUS_NOTIFICATIONS_NOTIFY_AFTER_UNREAD_FOR",
		"ARGUS_NOTIFICATIONS_SLACK_BOT_TOKEN",
		"ARGUS_NOTIFICATIONS_SLACK_CHANNEL_ID",
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
	if cfg.Node.Port != 3011 {
		t.Errorf("Node.Port = %d, want 3011", cfg.Node.Port)
	}
	if cfg.Node.BindAddress != "127.0.0.1" {
		t.Errorf("Node.BindAddress = %q, want 127.0.0.1", cfg.Node.BindAddress)
	}
	wantDB := filepath.Join(os.Getenv("HOME"), ".argus", "node.db")
	if cfg.Database.Path != wantDB {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, wantDB)
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

[node]
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
	if cfg.Node.Port != 5000 {
		t.Errorf("Node.Port = %d, want 5000", cfg.Node.Port)
	}
	if cfg.Node.BindAddress != "192.168.1.1" {
		t.Errorf("Node.BindAddress = %q, want 192.168.1.1", cfg.Node.BindAddress)
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
	if cfg.Node.BindAddress != "127.0.0.1" {
		t.Errorf("Node.BindAddress = %q, want 127.0.0.1", cfg.Node.BindAddress)
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

[node]
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
	if cfg.Node.BindAddress != "::1" {
		t.Errorf("Node.BindAddress = %q, want ::1", cfg.Node.BindAddress)
	}
}

func TestTailscaleDefaults(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = true, want false")
	}
	if cfg.Tailscale.HostnamePrefix != "" {
		t.Errorf("Tailscale.HostnamePrefix = %q, want empty", cfg.Tailscale.HostnamePrefix)
	}
	if cfg.Tailscale.AuthKey != "" {
		t.Errorf("Tailscale.AuthKey = %q, want empty", cfg.Tailscale.AuthKey)
	}
	if cfg.Tailscale.Port != 0 {
		t.Errorf("Tailscale.Port = %d, want 0", cfg.Tailscale.Port)
	}
}

func TestTailscaleFromFile(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[tailscale]
enabled = true
hostname_prefix = "my-argus"
auth_key = "tskey-auth-xxx"
port = 41642
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = false, want true")
	}
	if cfg.Tailscale.HostnamePrefix != "my-argus" {
		t.Errorf("Tailscale.HostnamePrefix = %q, want %q", cfg.Tailscale.HostnamePrefix, "my-argus")
	}
	if cfg.Tailscale.AuthKey != "tskey-auth-xxx" {
		t.Errorf("Tailscale.AuthKey = %q, want %q", cfg.Tailscale.AuthKey, "tskey-auth-xxx")
	}
	if cfg.Tailscale.Port != 41642 {
		t.Errorf("Tailscale.Port = %d, want 41642", cfg.Tailscale.Port)
	}
}

func TestTailscaleEnvOverride(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ARGUS_TAILSCALE_ENABLED", "true")
	t.Setenv("ARGUS_TAILSCALE_HOSTNAME_PREFIX", "env-argus")
	t.Setenv("ARGUS_TAILSCALE_AUTH_KEY", "tskey-auth-env")
	t.Setenv("ARGUS_TAILSCALE_PORT", "41642")

	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = false, want true (env override)")
	}
	if cfg.Tailscale.HostnamePrefix != "env-argus" {
		t.Errorf("Tailscale.HostnamePrefix = %q, want %q", cfg.Tailscale.HostnamePrefix, "env-argus")
	}
	if cfg.Tailscale.AuthKey != "tskey-auth-env" {
		t.Errorf("Tailscale.AuthKey = %q, want %q", cfg.Tailscale.AuthKey, "tskey-auth-env")
	}
	if cfg.Tailscale.Port != 41642 {
		t.Errorf("Tailscale.Port = %d, want 41642 (env override)", cfg.Tailscale.Port)
	}
}

func TestTailscaleEnabledWithoutHostnameOrAuthKey(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[tailscale]
enabled = true
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Tailscale.Enabled {
		t.Errorf("Tailscale.Enabled = false, want true")
	}
	if cfg.Tailscale.HostnamePrefix != "" {
		t.Errorf("Tailscale.HostnamePrefix = %q, want empty", cfg.Tailscale.HostnamePrefix)
	}
	if cfg.Tailscale.AuthKey != "" {
		t.Errorf("Tailscale.AuthKey = %q, want empty", cfg.Tailscale.AuthKey)
	}
}

func TestValidation_TailscalePortOutOfRange(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[tailscale]
port = 99999
`)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for tailscale.port = 99999, got nil")
	}
}

func TestValidation_TailscaleHostnamePathTraversal(t *testing.T) {
	clearArgusEnv(t)
	tests := []struct {
		name     string
		hostname string
	}{
		{name: "forward slash", hostname: "../escape"},
		{name: "backslash", hostname: `host\name`},
		{name: "dot dot", hostname: "host..name"},
		{name: "absolute path", hostname: "/etc/evil"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			content := fmt.Sprintf(`
[tailscale]
enabled = true
hostname_prefix = %q
`, tt.hostname)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load(config.Options{ConfigFile: path})
			if err == nil {
				t.Fatalf("expected validation error for hostname_prefix %q, got nil", tt.hostname)
			}
		})
	}
}

func TestValidation_TailscaleHostnameValid(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[tailscale]
enabled = true
hostname_prefix = "my-argus-node"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Tailscale.HostnamePrefix != "my-argus-node" {
		t.Errorf("Tailscale.HostnamePrefix = %q, want %q", cfg.Tailscale.HostnamePrefix, "my-argus-node")
	}
}

func TestNotificationsDefaults(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Notifications.Channel != "" {
		t.Errorf("Notifications.Channel = %q, want empty", cfg.Notifications.Channel)
	}
	if cfg.Notifications.NotifyAfterUnreadFor != "5m" {
		t.Errorf("Notifications.NotifyAfterUnreadFor = %q, want %q", cfg.Notifications.NotifyAfterUnreadFor, "5m")
	}
}

func TestNotificationsSlackFromFile(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"
notify_after_unread_for = "10m"

[notifications.slack]
bot_token = "xoxb-test-token"
channel_id = "C1234567890"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Notifications.Channel != "slack" {
		t.Errorf("Notifications.Channel = %q, want %q", cfg.Notifications.Channel, "slack")
	}
	if cfg.Notifications.NotifyAfterUnreadFor != "10m" {
		t.Errorf("Notifications.NotifyAfterUnreadFor = %q, want %q", cfg.Notifications.NotifyAfterUnreadFor, "10m")
	}
	if cfg.Notifications.Slack.BotToken != "xoxb-test-token" {
		t.Errorf("Slack.BotToken = %q, want %q", cfg.Notifications.Slack.BotToken, "xoxb-test-token")
	}
	if cfg.Notifications.Slack.ChannelID != "C1234567890" {
		t.Errorf("Slack.ChannelID = %q, want %q", cfg.Notifications.Slack.ChannelID, "C1234567890")
	}
}

func TestValidation_SlackMissingBotToken(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"

[notifications.slack]
channel_id = "C1234567890"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for missing bot_token, got nil")
	}
}

func TestValidation_SlackMissingChannelID(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"

[notifications.slack]
bot_token = "xoxb-test-token"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for missing channel_id, got nil")
	}
}

func TestValidation_InvalidNotifyAfterUnreadFor(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "slack"
notify_after_unread_for = "not-a-duration"

[notifications.slack]
bot_token = "xoxb-test-token"
channel_id = "C1234567890"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for invalid duration, got nil")
	}
}

func TestValidation_NotifyAfterUnreadForTooShort(t *testing.T) {
	clearArgusEnv(t)
	for _, dur := range []string{"0s", "500ms", "-5m"} {
		t.Run(dur, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			content := fmt.Sprintf(`
[notifications]
channel = "slack"
notify_after_unread_for = %q

[notifications.slack]
bot_token = "xoxb-test-token"
channel_id = "C1234567890"
`, dur)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load(config.Options{ConfigFile: path})
			if err == nil {
				t.Fatalf("expected validation error for duration %q, got nil", dur)
			}
		})
	}
}

func TestValidation_UnknownNotificationChannel(t *testing.T) {
	clearArgusEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[notifications]
channel = "email"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{ConfigFile: path})
	if err == nil {
		t.Fatal("expected validation error for unknown channel, got nil")
	}
}

func TestNotificationsDisabledByDefault(t *testing.T) {
	clearArgusEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty channel means disabled — no validation of slack fields
	if cfg.Notifications.Channel != "" {
		t.Errorf("Notifications.Channel = %q, want empty (disabled)", cfg.Notifications.Channel)
	}
}

func TestArgusHomeOverridesStateDir(t *testing.T) {
	clearArgusEnv(t)
	argusHome := t.TempDir()
	t.Setenv("ARGUS_HOME", argusHome)

	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantDB := filepath.Join(argusHome, "node.db")
	if cfg.Database.Path != wantDB {
		t.Errorf("Database.Path = %q, want %q", cfg.Database.Path, wantDB)
	}
}

func TestExplicitConfigLoadsWithoutResolvableHome(t *testing.T) {
	clearArgusEnv(t)
	// An explicit --config with an explicit database.path must load even when
	// the home dir can't be resolved (e.g. a daemon running without $HOME).
	t.Setenv("HOME", "")

	// Sanity-check that HOME="" actually disables state-dir resolution on this
	// platform: with no explicit config, auto-discovery must fail. If it
	// doesn't, the home dir is resolvable some other way and this test can't
	// exercise the regression, so skip rather than pass vacuously.
	if _, err := config.Load(config.Options{}); err == nil {
		t.Skip("home dir resolvable without HOME; cannot exercise unresolvable-home path")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := []byte(`
[database]
path = "/tmp/explicit.db"
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(config.Options{ConfigFile: path})
	if err != nil {
		t.Fatalf("unexpected error loading explicit config without HOME: %v", err)
	}
	if cfg.Database.Path != "/tmp/explicit.db" {
		t.Errorf("Database.Path = %q, want /tmp/explicit.db", cfg.Database.Path)
	}
}
