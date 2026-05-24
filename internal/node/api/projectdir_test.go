package api

import (
	"path/filepath"
	"testing"

	"github.com/bxnlabs/argus/internal/source"
)

func TestResolveProjectDirHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := resolveProjectDir("/some/repo", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/custom/home", "projects", source.ParentKeyFromPath("/some/repo"))
	if got != want {
		t.Errorf("resolveProjectDir() = %q, want %q", got, want)
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
