package shared

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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

// homeForSocketLen returns an ARGUS_HOME whose tmux socket path is exactly n
// bytes long, so the boundary tests below don't hand-count separators.
func homeForSocketLen(t *testing.T, n int) string {
	t.Helper()
	suffix := len(filepath.Join("/", "tmux", "server")) // "/tmux/server"
	return "/" + strings.Repeat("a", n-suffix-1)
}

func TestTmuxSocketPathAcceptsPathAtLimit(t *testing.T) {
	home := homeForSocketLen(t, maxTmuxSocketPath)
	t.Setenv("ARGUS_HOME", home)
	got, err := TmuxSocketPath()
	if err != nil {
		t.Fatalf("TmuxSocketPath at the %d-byte limit: %v", maxTmuxSocketPath, err)
	}
	if len(got) != maxTmuxSocketPath {
		t.Fatalf("test bug: built a %d-byte socket path, want %d", len(got), maxTmuxSocketPath)
	}
}

func TestTmuxSocketPathRejectsPathOverLimit(t *testing.T) {
	// One byte over the platform's sockaddr_un capacity. tmux reports this as
	// an opaque "File name too long" that points nowhere near the cause, so
	// TmuxSocketPath rejects it up front with a message naming the limit.
	home := homeForSocketLen(t, maxTmuxSocketPath+1)
	t.Setenv("ARGUS_HOME", home)
	_, err := TmuxSocketPath()
	if err == nil {
		t.Fatalf("TmuxSocketPath with a %d-byte socket path: got nil error, want over-limit error", maxTmuxSocketPath+1)
	}
	for _, want := range []string{
		strconv.Itoa(maxTmuxSocketPath),
		strconv.Itoa(maxTmuxSocketPath + 1),
		"ARGUS_HOME",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestTmuxCommandRejectsPathOverLimit(t *testing.T) {
	// The limit must reach every tmux caller, not just direct TmuxSocketPath users.
	t.Setenv("ARGUS_HOME", homeForSocketLen(t, maxTmuxSocketPath+1))
	if _, err := TmuxCommand("has-session", "-t", "x"); err == nil {
		t.Error("TmuxCommand: got nil error, want over-limit error")
	}
	if _, err := TmuxCommandContext(context.Background(), "list-sessions"); err == nil {
		t.Error("TmuxCommandContext: got nil error, want over-limit error")
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

func TestEnsureTmuxStateDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	got, err := EnsureTmuxStateDir()
	if err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	want := filepath.Join(dir, "tmux")
	if got != want {
		t.Fatalf("EnsureTmuxStateDir() = %q, want %q", got, want)
	}
	info, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat tmux dir: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("tmux dir perm = %o, want 700", perm)
	}
}

func TestSeedTmuxConfig_WritesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
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
}

func TestSeedTmuxConfig_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	tmuxDir, err := EnsureTmuxStateDir()
	if err != nil {
		t.Fatalf("EnsureTmuxStateDir: %v", err)
	}
	// First call creates the config; second hits the EEXIST link path. Neither
	// must leave behind the temp file used for the atomic publish.
	for i := 0; i < 2; i++ {
		if _, err := SeedTmuxConfig(); err != nil {
			t.Fatalf("SeedTmuxConfig (call %d): %v", i+1, err)
		}
	}
	entries, err := os.ReadDir(tmuxDir)
	if err != nil {
		t.Fatalf("read tmux dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "tmux.conf" {
		t.Errorf("tmux dir = %v, want only [tmux.conf] (stray temp file?)", names)
	}
}

func TestSeedTmuxConfig_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ARGUS_HOME", dir)
	if _, err := EnsureTmuxStateDir(); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
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
