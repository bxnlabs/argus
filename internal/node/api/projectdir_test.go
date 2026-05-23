package api

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveProjectDirHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := resolveProjectDir("/some/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := filepath.Join("/custom/home", "projects")
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("resolveProjectDir() = %q, want prefix %q", got, wantPrefix)
	}
}

func TestResolveProjectDirOverrideShortCircuits(t *testing.T) {
	got, err := resolveProjectDir("/some/repo", "/explicit/override")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/explicit/override" {
		t.Errorf("resolveProjectDir() = %q, want /explicit/override", got)
	}
}
