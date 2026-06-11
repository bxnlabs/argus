package config

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/bxnlabs/argus/internal/shared"
	"github.com/spf13/viper"
)

// Config holds all Argus configuration.
type Config struct {
	Port        int    `mapstructure:"port"`
	BindAddress string `mapstructure:"bind_address"`
	// AllowedHosts are extra Host header values accepted by the node API's
	// anti-DNS-rebinding gate, beyond the ones derived automatically (loopback,
	// the bind address, and the Tailscale identity). Only needed when binding to
	// 0.0.0.0 and reaching the node by a name that can't be derived (e.g. an
	// /etc/hosts alias or internal DNS name). Empty by default.
	AllowedHosts  []string            `mapstructure:"allowed_hosts"`
	Git           GitConfig           `mapstructure:"git"`
	Tailscale     TailscaleConfig     `mapstructure:"tailscale"`
	Notifications NotificationsConfig `mapstructure:"notifications"`
}

type GitConfig struct {
	BranchPrefix string `mapstructure:"branch_prefix"`
}

type TailscaleConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	HostnamePrefix string `mapstructure:"hostname_prefix"`
	AuthKey        string `mapstructure:"auth_key"`
	Port           int    `mapstructure:"port"`
	DiscoveryTag   string `mapstructure:"discovery_tag"`
}

type NotificationsConfig struct {
	Channel              string            `mapstructure:"channel"`
	NotifyAfterUnreadFor string            `mapstructure:"notify_after_unread_for"`
	Slack                SlackNotifyConfig `mapstructure:"slack"`
}

type SlackNotifyConfig struct {
	BotToken  string `mapstructure:"bot_token"`
	ChannelID string `mapstructure:"channel_id"`
}

// Options controls how config is loaded.
type Options struct {
	// ConfigFile overrides the default config file path.
	// When empty, searches ~/.argus/ for config.toml.
	ConfigFile string
}

// Load reads configuration from the TOML file and environment variables.
// Priority (highest to lowest): env vars > config file > defaults.
// When ConfigFile is set explicitly, a missing file is an error.
// When auto-discovering (~/.argus/config.toml), a missing file is fine; defaults are used.
// A malformed config file is always an error.
func Load(opts Options) (*Config, error) {
	v := viper.New()

	// The state dir is only needed to auto-discover ~/.argus/config.toml. An
	// explicit --config must still load when the home dir is unavailable, so
	// defer treating this as fatal until it's actually used below.
	stateDir, stateDirErr := shared.StateDir()

	// Defaults
	v.SetDefault("port", 3000)
	v.SetDefault("bind_address", "127.0.0.1")
	v.SetDefault("git.branch_prefix", "")
	v.SetDefault("tailscale.enabled", false)
	v.SetDefault("tailscale.hostname_prefix", "")
	v.SetDefault("tailscale.auth_key", "")
	v.SetDefault("tailscale.port", 0)
	v.SetDefault("tailscale.discovery_tag", "tag:argus-node")
	v.SetDefault("notifications.channel", "")
	v.SetDefault("notifications.notify_after_unread_for", "5m")

	// Environment variables: ARGUS_PORT, etc.
	v.SetEnvPrefix("ARGUS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Config file
	if opts.ConfigFile != "" {
		v.SetConfigFile(opts.ConfigFile)
	} else {
		if stateDirErr != nil {
			return nil, fmt.Errorf("config: determine state dir: %w", stateDirErr)
		}
		// Secure the state root before reading config from it: ~/.argus/config.toml
		// may hold secrets (Tailscale auth key, Slack token), so a root left at
		// 0755 by an older version must be tightened first. This is the earliest
		// point every config-loading command (server, node, combined, migrate)
		// passes through. Scoped to auto-discovery so an explicit --config still
		// loads without a resolvable home (see TestExplicitConfigLoadsWithoutResolvableHome).
		if err := shared.EnsureSecureDir(stateDir); err != nil {
			return nil, fmt.Errorf("config: secure state dir: %w", err)
		}
		v.AddConfigPath(stateDir)
		v.SetConfigName("config")
		v.SetConfigType("toml")
	}

	if err := v.ReadInConfig(); err != nil {
		notFound := false
		if _, ok := err.(viper.ConfigFileNotFoundError); ok && opts.ConfigFile == "" {
			notFound = true
		} else if os.IsNotExist(err) && opts.ConfigFile == "" {
			notFound = true
		}
		if !notFound {
			return nil, fmt.Errorf("config: read config file: %w", err)
		}
		// Auto-discovery found no file — use defaults.
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
	if err := validatePort("port", cfg.Port); err != nil {
		return err
	}
	if err := validateIP("bind_address", cfg.BindAddress); err != nil {
		return err
	}
	if cfg.Tailscale.Port < 0 || cfg.Tailscale.Port > 65535 {
		return fmt.Errorf("tailscale.port must be 0-65535, got %d", cfg.Tailscale.Port)
	}
	if cfg.Tailscale.Enabled && cfg.Tailscale.HostnamePrefix != "" {
		if strings.ContainsAny(cfg.Tailscale.HostnamePrefix, "/\\") || strings.Contains(cfg.Tailscale.HostnamePrefix, "..") {
			return fmt.Errorf("tailscale.hostname_prefix must not contain path separators or '..'")
		}
	}
	if cfg.Notifications.Channel != "" {
		dur, err := time.ParseDuration(cfg.Notifications.NotifyAfterUnreadFor)
		if err != nil {
			return fmt.Errorf("notifications.notify_after_unread_for must be a valid duration (e.g. \"5m\"), got %q", cfg.Notifications.NotifyAfterUnreadFor)
		}
		if dur < time.Second {
			return fmt.Errorf("notifications.notify_after_unread_for must be at least 1s, got %q", cfg.Notifications.NotifyAfterUnreadFor)
		}
		switch cfg.Notifications.Channel {
		case "slack":
			if cfg.Notifications.Slack.BotToken == "" {
				return fmt.Errorf("notifications.slack.bot_token is required when notifications.channel = \"slack\"")
			}
			if cfg.Notifications.Slack.ChannelID == "" {
				return fmt.Errorf("notifications.slack.channel_id is required when notifications.channel = \"slack\"")
			}
		default:
			return fmt.Errorf("notifications.channel must be \"slack\" or empty, got %q", cfg.Notifications.Channel)
		}
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
