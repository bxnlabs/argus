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
