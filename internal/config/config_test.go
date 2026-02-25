package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/config"
)

func TestLoadMissingFile(t *testing.T) {
	cfg, err := config.LoadFrom("/nonexistent/path/config.toml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if cfg.BranchPrefix != "" {
		t.Errorf("expected empty BranchPrefix, got %q", cfg.BranchPrefix)
	}
}

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`branch_prefix = "jeev"`), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BranchPrefix != "jeev" {
		t.Errorf("expected BranchPrefix %q, got %q", "jeev", cfg.BranchPrefix)
	}
}

func TestLoadMalformedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(`not valid toml [[[[`), 0644); err != nil {
		t.Fatal(err)
	}
	// Malformed config should return defaults, not error
	cfg, err := config.LoadFrom(path)
	if err != nil {
		t.Fatalf("expected no error for malformed config, got %v", err)
	}
	if cfg.BranchPrefix != "" {
		t.Errorf("expected empty BranchPrefix for malformed config, got %q", cfg.BranchPrefix)
	}
}
