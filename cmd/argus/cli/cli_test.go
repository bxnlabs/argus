package cli

import (
	"path/filepath"
	"testing"
)

func TestDiscoveryFilePathHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := discoveryFilePath()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/custom/home", "node.json")
	if got != want {
		t.Errorf("discoveryFilePath() = %q, want %q", got, want)
	}
}
