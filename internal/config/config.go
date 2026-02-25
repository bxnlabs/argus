package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds user preferences loaded from ~/.argus/config.toml.
type Config struct {
	BranchPrefix string `toml:"branch_prefix"`
}

// Load loads the config from the default location (~/.argus/config.toml).
// Missing or malformed files are silently ignored; defaults are returned.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("config: could not determine home directory: %w", err)
	}
	return LoadFrom(filepath.Join(home, ".argus", "config.toml"))
}

// LoadFrom loads the config from the given path.
// Missing or malformed files are silently ignored; defaults are returned.
func LoadFrom(path string) (*Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, nil
	}
	if _, err := toml.Decode(string(data), cfg); err != nil {
		// Malformed config: warn to stderr, use defaults.
		return cfg, nil
	}
	return cfg, nil
}

func defaultConfig() *Config {
	return &Config{}
}
