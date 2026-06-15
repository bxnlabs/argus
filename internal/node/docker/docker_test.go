package docker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileComposeFile(t *testing.T) {
	state := t.TempDir()
	// Non-docker profile: hooks dir only.
	mkdir(t, filepath.Join(state, "profiles", "plain", "hooks"))
	if _, ok := ProfileComposeFile(state, "plain"); ok {
		t.Error("plain profile should not be dockerized")
	}
	// Docker profile: docker-compose.yml present.
	mkdir(t, filepath.Join(state, "profiles", "work", "hooks"))
	writeFile(t, filepath.Join(state, "profiles", "work", "docker-compose.yml"), "services: {}")
	file, ok := ProfileComposeFile(state, "work")
	if !ok {
		t.Fatal("work profile should be dockerized")
	}
	if file != filepath.Join(state, "profiles", "work", "docker-compose.yml") {
		t.Errorf("unexpected compose file: %s", file)
	}
	// compose.yaml is also recognized.
	mkdir(t, filepath.Join(state, "profiles", "alt", "hooks"))
	writeFile(t, filepath.Join(state, "profiles", "alt", "compose.yaml"), "services: {}")
	if _, ok := ProfileComposeFile(state, "alt"); !ok {
		t.Error("compose.yaml should be recognized")
	}
}

func TestProjectName(t *testing.T) {
	if got := ProjectName("work"); got != "argus-work" {
		t.Errorf("ProjectName = %q, want argus-work", got)
	}
}

func TestEnv(t *testing.T) {
	env := Env("/home/jeev", "/home/jeev/.argus")
	want := map[string]bool{
		"ARGUS_HOST_HOME=/home/jeev":        true,
		"ARGUS_STATE_DIR=/home/jeev/.argus": true,
	}
	for _, e := range env {
		delete(want, e)
	}
	if len(want) != 0 {
		t.Errorf("missing env entries: %v (got %v)", want, env)
	}
	hasUID, hasGID := false, false
	for _, e := range env {
		if len(e) > 9 && e[:9] == "ARGUS_UID" {
			hasUID = true
		}
		if len(e) > 9 && e[:9] == "ARGUS_GID" {
			hasGID = true
		}
	}
	if !hasUID || !hasGID {
		t.Errorf("expected ARGUS_UID and ARGUS_GID in env: %v", env)
	}
}

func TestPathVisible(t *testing.T) {
	home := "/home/jeev"
	state := "/data/argus"
	cases := []struct {
		path string
		want bool
	}{
		{"/home/jeev/repo/wt", true},
		{"/home/jeev", true},
		{"/data/argus/worktrees/x", true},
		{"/var/tmp/elsewhere", false},
		{"/home/jeevil/sneaky", false}, // prefix-but-not-subdir
	}
	for _, c := range cases {
		if got := PathVisible(c.path, home, state); got != c.want {
			t.Errorf("PathVisible(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
