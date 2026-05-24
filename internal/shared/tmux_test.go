package shared

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxSocketPathHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath: %v", err)
	}
	want := filepath.Join("/custom/home", "tmux", "server")
	if got != want {
		t.Errorf("TmuxSocketPath() = %q, want %q", got, want)
	}
}

func TestTmuxConfigPathHonorsArgusHome(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	got, err := TmuxConfigPath()
	if err != nil {
		t.Fatalf("TmuxConfigPath: %v", err)
	}
	want := filepath.Join("/custom/home", "tmux", "tmux.conf")
	if got != want {
		t.Errorf("TmuxConfigPath() = %q, want %q", got, want)
	}
}

func TestTmuxCommandThreadsSocket(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	cmd, err := TmuxCommand("has-session", "-t", "x")
	if err != nil {
		t.Fatalf("TmuxCommand: %v", err)
	}
	sock := filepath.Join("/custom/home", "tmux", "server")
	want := []string{"tmux", "-S", sock, "has-session", "-t", "x"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestTmuxCommandContextThreadsSocket(t *testing.T) {
	t.Setenv("ARGUS_HOME", "/custom/home")
	cmd, err := TmuxCommandContext(context.Background(), "list-sessions")
	if err != nil {
		t.Fatalf("TmuxCommandContext: %v", err)
	}
	sock := filepath.Join("/custom/home", "tmux", "server")
	want := []string{"tmux", "-S", sock, "list-sessions"}
	if len(cmd.Args) != len(want) {
		t.Fatalf("cmd.Args = %v, want %v", cmd.Args, want)
	}
	for i := range want {
		if cmd.Args[i] != want[i] {
			t.Errorf("cmd.Args[%d] = %q, want %q", i, cmd.Args[i], want[i])
		}
	}
}

func TestSeedTmuxConfig_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	got, err := SeedTmuxConfig()
	if err != nil {
		t.Fatalf("SeedTmuxConfig: %v", err)
	}
	want := filepath.Join(dir, "tmux", "tmux.conf")
	if got != want {
		t.Fatalf("SeedTmuxConfig() = %q, want %q", got, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	for _, directive := range []string{
		`default-terminal "tmux-256color"`,
		"terminal-overrides",
		"mouse on",
		"status-position bottom",
	} {
		if !strings.Contains(string(data), directive) {
			t.Errorf("config missing %q\ngot:\n%s", directive, data)
		}
	}
	info, err := os.Stat(filepath.Join(dir, "tmux"))
	if err != nil {
		t.Fatalf("stat tmux dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("tmux dir perm = %o, want 700", perm)
	}
}

func TestSeedTmuxConfig_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	confPath := filepath.Join(dir, "tmux", "tmux.conf")
	custom := "# my custom config\nset -g mouse off\n"
	if err := os.WriteFile(confPath, []byte(custom), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedTmuxConfig(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("SeedTmuxConfig overwrote user config\ngot:\n%s\nwant:\n%s", data, custom)
	}
}
